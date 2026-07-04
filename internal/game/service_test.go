package game

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"islands/internal/actor"
	"islands/internal/inventory"
	"islands/internal/mapgen"
	"islands/internal/realtime"
	"islands/internal/storage"
	"islands/internal/world"
)

func TestHarvestPublishesOnlyToVisibleSubscribers(t *testing.T) {
	hub := realtime.NewHub()
	service := NewService(hub, realtime.Config{VisibleChunkRadius: 1})
	act := service.SeedDemoWorld(1)
	coord, _ := world.ToChunkCoord(act.X, act.Y)

	visible := hub.Subscribe(1, 1, map[world.ChunkCoord]struct{}{coord: {}})
	defer hub.Unsubscribe(visible.ID)
	hidden := hub.Subscribe(2, 1, map[world.ChunkCoord]struct{}{{X: 4, Y: 4}: {}})
	defer hub.Unsubscribe(hidden.ID)

	result, err := service.ApplyAction(context.Background(), 1, 1, ActionRequest{ActionType: "harvest"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Accepted {
		t.Fatalf("result accepted: got false")
	}

	select {
	case event := <-visible.Events:
		if event.Type != "entity_patch" {
			t.Fatalf("event type: got %q, want entity_patch", event.Type)
		}
	case <-time.After(time.Second):
		t.Fatalf("visible subscriber did not receive harvest update")
	}

	select {
	case event := <-hidden.Events:
		t.Fatalf("hidden subscriber received event %+v", event)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestLoadChunksUsesLoadedMapWithoutCreatingMissingChunks(t *testing.T) {
	service := NewService(realtime.NewHub(), realtime.Config{VisibleChunkRadius: 1})
	chunk := world.NewChunk(0, 0)
	chunk.Stock[0] = 42
	if err := service.LoadChunks(1, map[world.ChunkCoord]*world.Chunk{{X: 0, Y: 0}: chunk}); err != nil {
		t.Fatal(err)
	}

	snapshots := service.ChunkSnapshots(context.Background(), 1, realtime.VisibleChunks(world.ChunkCoord{X: 0, Y: 0}, 1))

	if len(snapshots) != 1 {
		t.Fatalf("snapshot count: got %d, want 1", len(snapshots))
	}
	if snapshots[0].Stock[0] != 42 {
		t.Fatalf("loaded stock: got %d, want 42", snapshots[0].Stock[0])
	}
}

func TestMovePublishesSnapshotsForNewVisibleChunks(t *testing.T) {
	hub := realtime.NewHub()
	service := NewService(hub, realtime.Config{VisibleChunkRadius: 0})
	chunks := make(map[world.ChunkCoord]*world.Chunk)
	for y := int32(-1); y <= 1; y++ {
		for x := int32(-1); x <= 2; x++ {
			chunks[world.ChunkCoord{X: x, Y: y}] = world.NewChunk(x, y)
		}
	}
	if err := service.LoadWorld(1, storage.WorldState{
		Width:  96,
		Height: 96,
		Seed:   "move-test",
		Chunks: chunks,
		Players: storage.PlayerState{Actors: map[actor.ID]*actor.Actor{
			actor.ID(1): &actor.Actor{ID: 1, WorldID: 1, X: world.ChunkSize - 1, Y: 0, PocketInventoryID: 1},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	startCoord := world.ChunkCoord{X: 0, Y: 0}

	client := hub.Subscribe(1, 1, map[world.ChunkCoord]struct{}{startCoord: {}})
	defer hub.Unsubscribe(client.ID)

	result, err := service.ApplyAction(context.Background(), 1, 1, ActionRequest{ActionType: "move", X: world.ChunkSize, Y: 0})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Accepted {
		t.Fatalf("result accepted: got false")
	}

	received := make([]string, 0, 2)
	for len(received) < 2 {
		select {
		case event := <-client.Events:
			received = append(received, event.Type)
		case <-time.After(time.Second):
			t.Fatalf("events: got %v, want entity_patch and chunk_snapshot", received)
		}
	}

	if received[0] != "entity_patch" || received[1] != "chunk_snapshot" {
		t.Fatalf("events: got %v, want [entity_patch chunk_snapshot]", received)
	}
}

func TestMoveEntityPatchIsCompact(t *testing.T) {
	hub := realtime.NewHub()
	service := NewService(hub, realtime.Config{VisibleChunkRadius: 1})
	act := service.SeedDemoWorld(1)
	coord, _ := world.ToChunkCoord(act.X, act.Y)
	client := hub.Subscribe(1, 1, map[world.ChunkCoord]struct{}{coord: {}})
	defer hub.Unsubscribe(client.ID)

	if _, err := service.ApplyAction(context.Background(), 1, 1, ActionRequest{ActionType: "move", X: act.X + 1, Y: act.Y}); err != nil {
		t.Fatal(err)
	}

	select {
	case event := <-client.Events:
		if event.Type != "entity_patch" {
			t.Fatalf("event type: got %q, want entity_patch", event.Type)
		}
		data, err := json.Marshal(event.Data)
		if err != nil {
			t.Fatal(err)
		}
		raw := string(data)
		if strings.Contains(raw, "client_action_id") || strings.Contains(raw, "action_type") || strings.Contains(raw, "event_id") {
			t.Fatalf("entity patch leaked action ack fields: %s", raw)
		}
		if !strings.Contains(raw, `"actor"`) {
			t.Fatalf("entity patch missing actor: %s", raw)
		}
	case <-time.After(time.Second):
		t.Fatalf("client did not receive entity_patch")
	}
}

func TestChunkSnapshotEncodesUint16LayersAsBase64(t *testing.T) {
	ch := world.NewChunk(0, 0)
	ch.Base[0] = 9895
	ch.Cover[0] = 513
	ch.Stock[0] = 11
	snapshot := snapshotChunk(ch, 1)

	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"base", "cover", "surface", "stock"} {
		if _, ok := raw[key].(string); !ok {
			t.Fatalf("%s layer should be base64 string: %s", key, data)
		}
	}
}

func TestMoveRejectsTeleport(t *testing.T) {
	service := NewService(realtime.NewHub(), realtime.Config{VisibleChunkRadius: 1})
	act := service.SeedDemoWorld(1)

	_, err := service.ApplyAction(context.Background(), 1, 1, ActionRequest{
		ActionType: "move",
		X:          act.X + 2,
		Y:          act.Y,
	})

	if err != ErrInvalidAction {
		t.Fatalf("move error: got %v, want %v", err, ErrInvalidAction)
	}
}

func TestActionsDoNotAdvanceWorldTime(t *testing.T) {
	service := NewService(realtime.NewHub(), realtime.Config{VisibleChunkRadius: 1})
	service.SeedDemoWorld(1)
	before := service.WorldTime()

	if _, err := service.ApplyAction(context.Background(), 1, 1, ActionRequest{ActionType: "move", X: DemoActorStartX + 1, Y: DemoActorStartY}); err != nil {
		t.Fatal(err)
	}
	after := service.WorldTime()

	if before.WorldTime != after.WorldTime {
		t.Fatalf("world time after action: got %d, want %d", after.WorldTime, before.WorldTime)
	}
}

func TestMoveActionResultIsCompact(t *testing.T) {
	service := NewService(realtime.NewHub(), realtime.Config{VisibleChunkRadius: 1})
	service.SeedDemoWorld(1)

	result, err := service.ApplyAction(context.Background(), 1, 1, ActionRequest{ActionType: "move", X: DemoActorStartX + 1, Y: DemoActorStartY})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"inventory"`) {
		t.Fatalf("move result should omit inventory: %s", data)
	}
	if strings.Contains(string(data), `"actor"`) {
		t.Fatalf("move result should omit actor: %s", data)
	}
	if strings.Contains(string(data), `"world_time"`) {
		t.Fatalf("move result should omit world_time: %s", data)
	}
	if result.MoveDelayMS == 0 || result.Target == nil {
		t.Fatalf("move result timer fields: %+v", result)
	}
}

func TestMoveStartsPendingMovement(t *testing.T) {
	service := NewService(realtime.NewHub(), realtime.Config{VisibleChunkRadius: 1})
	act := service.SeedDemoWorld(1)

	result, err := service.ApplyAction(context.Background(), 1, 1, ActionRequest{ActionType: "move", X: act.X + 1, Y: act.Y})
	if err != nil {
		t.Fatal(err)
	}
	if result.MoveDelayMS == 0 {
		t.Fatalf("move delay should be returned")
	}
	current, err := service.Actor(context.Background(), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if current.X != act.X || current.Y != act.Y {
		t.Fatalf("actor moved immediately: got %d,%d want %d,%d", current.X, current.Y, act.X, act.Y)
	}
}

func TestPendingMoveBlocksAnotherMove(t *testing.T) {
	service := NewService(realtime.NewHub(), realtime.Config{VisibleChunkRadius: 1})
	act := service.SeedDemoWorld(1)

	if _, err := service.ApplyAction(context.Background(), 1, 1, ActionRequest{ActionType: "move", X: act.X + 1, Y: act.Y}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ApplyAction(context.Background(), 1, 1, ActionRequest{ActionType: "move", X: act.X, Y: act.Y + 1}); !errors.Is(err, ErrConflict) {
		t.Fatalf("second move error: got %v, want %v", err, ErrConflict)
	}
}

func TestPendingMoveBlocksPositionActions(t *testing.T) {
	service := NewService(realtime.NewHub(), realtime.Config{VisibleChunkRadius: 1})
	act := service.SeedDemoWorld(1)

	if _, err := service.ApplyAction(context.Background(), 1, 1, ActionRequest{ActionType: "move", X: act.X + 1, Y: act.Y}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ApplyAction(context.Background(), 1, 1, ActionRequest{ActionType: "harvest"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("harvest during move error: got %v, want %v", err, ErrConflict)
	}
	if _, err := service.ApplyAction(context.Background(), 1, 1, ActionRequest{ActionType: "plant_tree"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("plant during move error: got %v, want %v", err, ErrConflict)
	}
}

func TestPendingMoveCompletionSurvivesPlayerSaveError(t *testing.T) {
	hub := realtime.NewHub()
	service := NewService(hub, realtime.Config{VisibleChunkRadius: 1})
	service.SetStore(failingPlayerStore{err: errors.New("disk unavailable")})
	act := service.SeedDemoWorld(1)
	coord, _ := world.ToChunkCoord(act.X, act.Y)
	targetCoord, targetIndex := world.ToChunkCoord(act.X+1, act.Y)
	service.mu.Lock()
	service.chunkLocked(1, targetCoord).Surface[targetIndex] = uint16(world.PackSurface(world.SurfaceStoneRoad, 1, 0))
	service.mu.Unlock()
	client := hub.Subscribe(1, 1, map[world.ChunkCoord]struct{}{coord: {}, targetCoord: {}})
	defer hub.Unsubscribe(client.ID)

	result, err := service.ApplyAction(context.Background(), 1, 1, ActionRequest{ActionType: "move", X: act.X + 1, Y: act.Y})
	if err != nil {
		t.Fatal(err)
	}
	if result.MoveDelayMS == 0 {
		t.Fatalf("move delay should be returned")
	}

	foundPatch := false
	foundError := false
	deadline := time.After(time.Duration(result.MoveDelayMS+500) * time.Millisecond)
	for !foundPatch || !foundError {
		select {
		case event := <-client.Events:
			if event.Type == "entity_patch" {
				foundPatch = true
			}
			if event.Type == "stream_error" {
				foundError = true
			}
		case <-deadline:
			t.Fatalf("events: entity_patch=%v stream_error=%v", foundPatch, foundError)
		}
	}

	current, err := service.Actor(context.Background(), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if current.X != act.X+1 || current.Y != act.Y {
		t.Fatalf("actor position after save error: got %d,%d want %d,%d", current.X, current.Y, act.X+1, act.Y)
	}
}

func TestShutdownCancelsPendingMove(t *testing.T) {
	service := NewService(realtime.NewHub(), realtime.Config{VisibleChunkRadius: 1})
	act := service.SeedDemoWorld(1)

	result, err := service.ApplyAction(context.Background(), 1, 1, ActionRequest{ActionType: "move", X: act.X + 1, Y: act.Y})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Duration(result.MoveDelayMS+50) * time.Millisecond)

	current, err := service.Actor(context.Background(), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if current.X != act.X || current.Y != act.Y {
		t.Fatalf("actor moved after shutdown: got %d,%d want %d,%d", current.X, current.Y, act.X, act.Y)
	}
}

func TestHarvestActionResultIsCompact(t *testing.T) {
	service := NewService(realtime.NewHub(), realtime.Config{VisibleChunkRadius: 1})
	service.SeedDemoWorld(1)

	result, err := service.ApplyAction(context.Background(), 1, 1, ActionRequest{ActionType: "harvest"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"inventory"`) {
		t.Fatalf("harvest result should omit inventory: %s", data)
	}
	if strings.Contains(string(data), `"actor"`) {
		t.Fatalf("harvest result should omit actor: %s", data)
	}
	if strings.Contains(string(data), `"world_time"`) {
		t.Fatalf("harvest result should omit world_time: %s", data)
	}
}

func TestHarvestPublishesInventoryPatchToActor(t *testing.T) {
	hub := realtime.NewHub()
	service := NewService(hub, realtime.Config{VisibleChunkRadius: 1})
	act := service.SeedDemoWorld(1)
	coord, _ := world.ToChunkCoord(act.X, act.Y)
	owner := hub.Subscribe(1, 1, map[world.ChunkCoord]struct{}{coord: {}})
	defer hub.Unsubscribe(owner.ID)
	other := hub.Subscribe(2, 1, map[world.ChunkCoord]struct{}{coord: {}})
	defer hub.Unsubscribe(other.ID)

	if _, err := service.ApplyAction(context.Background(), 1, 1, ActionRequest{ActionType: "harvest"}); err != nil {
		t.Fatalf("harvest: %v", err)
	}

	var patch InventoryPatch
	found := false
	for i := 0; i < 7; i++ {
		select {
		case event := <-owner.Events:
			if event.Type != "inventory_patch" {
				continue
			}
			data, err := json.Marshal(event.Data)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(data, &patch); err != nil {
				t.Fatal(err)
			}
			found = true
		case <-time.After(time.Second):
			t.Fatalf("owner did not receive inventory_patch")
		}
		if found {
			break
		}
	}
	if !found {
		t.Fatalf("owner did not receive inventory_patch")
	}
	if stackAmount(patch.Inventory, ItemWood) != treeWoodYield(TreeStageMature) {
		t.Fatalf("inventory patch: got %+v", patch.Inventory)
	}

	for i := 0; i < 2; i++ {
		select {
		case event := <-other.Events:
			if event.Type == "inventory_patch" {
				t.Fatalf("other actor received inventory patch: %+v", event)
			}
		case <-time.After(20 * time.Millisecond):
			return
		}
	}
}

func TestPocketInventoryPreservesSlotOrder(t *testing.T) {
	service := NewService(nil, realtime.Config{})
	act := service.SeedDemoActor(1)

	service.mu.Lock()
	if !service.addStackLocked(act.PocketInventoryID, inventory.ItemID(4), 1) {
		t.Fatalf("add first stack")
	}
	if !service.addStackLocked(act.PocketInventoryID, inventory.ItemID(2), 1) {
		t.Fatalf("add second stack")
	}
	snapshot := service.inventorySnapshotLocked(act)
	service.mu.Unlock()

	if len(snapshot) != 2 || snapshot[0].ItemID != 4 || snapshot[1].ItemID != 2 {
		t.Fatalf("snapshot order: got %+v", snapshot)
	}
}

func TestPocketInventorySlotLimitAllowsExistingStack(t *testing.T) {
	service := NewService(nil, realtime.Config{})
	act := service.SeedDemoActor(1)

	service.mu.Lock()
	for i := 0; i < PocketSlotLimit; i++ {
		if !service.addStackLocked(act.PocketInventoryID, inventory.ItemID(100+i), 1) {
			t.Fatalf("add stack %d", i)
		}
	}
	if service.addStackLocked(act.PocketInventoryID, inventory.ItemID(200), 1) {
		t.Fatalf("tenth new stack should not fit")
	}
	if !service.addStackLocked(act.PocketInventoryID, inventory.ItemID(100), 4) {
		t.Fatalf("existing stack should fit")
	}
	snapshot := service.inventorySnapshotLocked(act)
	service.mu.Unlock()

	if len(snapshot) != PocketSlotLimit {
		t.Fatalf("snapshot len: got %d, want %d", len(snapshot), PocketSlotLimit)
	}
	if snapshot[0].ItemID != 100 || snapshot[0].Amount != 5 {
		t.Fatalf("first stack: got %+v", snapshot[0])
	}
}

func TestPocketInventoryOrderSurvivesPlayerStateRoundTrip(t *testing.T) {
	service := NewService(nil, realtime.Config{})
	act := service.SeedDemoActor(1)

	service.mu.Lock()
	if !service.addStackLocked(act.PocketInventoryID, inventory.ItemID(7), 1) {
		t.Fatalf("add first stack")
	}
	if !service.addStackLocked(act.PocketInventoryID, inventory.ItemID(3), 2) {
		t.Fatalf("add second stack")
	}
	state := service.playerStateLocked()
	service.mu.Unlock()

	loaded := NewService(nil, realtime.Config{})
	loaded.mu.Lock()
	loaded.loadPlayersLocked(state)
	snapshot := loaded.inventorySnapshotLocked(act)
	loaded.mu.Unlock()

	if len(snapshot) != 2 || snapshot[0].ItemID != 7 || snapshot[1].ItemID != 3 {
		t.Fatalf("snapshot order after load: got %+v", snapshot)
	}
}

func TestHarvestWithFullPocketDoesNotConsumeStock(t *testing.T) {
	service := NewService(nil, realtime.Config{VisibleChunkRadius: 1})
	act := service.SeedDemoWorld(1)

	service.mu.Lock()
	for i := 0; i < PocketSlotLimit; i++ {
		if !service.addStackLocked(act.PocketInventoryID, inventory.ItemID(100+i), 1) {
			t.Fatalf("add stack %d", i)
		}
	}
	coord, index := world.ToChunkCoord(act.X, act.Y)
	ch := service.chunkLocked(1, coord)
	previousStock := ch.Stock[index]
	previousCover := ch.Cover[index]
	service.mu.Unlock()

	if _, err := service.ApplyAction(context.Background(), 1, uint64(act.ID), ActionRequest{ActionType: "harvest"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("harvest err: got %v, want %v", err, ErrConflict)
	}

	service.mu.Lock()
	defer service.mu.Unlock()
	if ch.Stock[index] != previousStock {
		t.Fatalf("stock: got %d, want %d", ch.Stock[index], previousStock)
	}
	if ch.Cover[index] != previousCover {
		t.Fatalf("cover changed: got %d, want %d", ch.Cover[index], previousCover)
	}
	if len(service.inventorySnapshotLocked(act)) != PocketSlotLimit {
		t.Fatalf("inventory changed: got %+v", service.inventorySnapshotLocked(act))
	}
}

func TestPlantTreeCreatesSapling(t *testing.T) {
	service := NewService(nil, realtime.Config{VisibleChunkRadius: 1})
	act := service.SeedDemoActor(1)
	coord, index := world.ToChunkCoord(act.X, act.Y)
	ch := world.NewChunk(coord.X, coord.Y)
	ch.Base[index] = uint16(world.PackBase(world.BiomeMeadow, world.SoilGrass, 8, 0))
	ch.Cover[index] = uint16(world.PackCover(world.CoverGrass, 1, 0))
	if err := service.LoadChunks(1, map[world.ChunkCoord]*world.Chunk{coord: ch}); err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	if !service.addStackLocked(act.PocketInventoryID, ItemTreeSapling, 1) {
		t.Fatalf("add tree sapling")
	}
	service.mu.Unlock()

	if _, err := service.ApplyAction(context.Background(), 1, uint64(act.ID), ActionRequest{ActionType: "plant_tree"}); err != nil {
		t.Fatalf("plant tree: %v", err)
	}

	service.mu.Lock()
	defer service.mu.Unlock()
	cover := service.chunkLocked(1, coord).CoverCell(index)
	if cover.Kind() != world.CoverBirchForest || treeStage(cover) != TreeStageSapling {
		t.Fatalf("cover after plant: got kind=%d stage=%d", cover.Kind(), treeStage(cover))
	}
	if got := stackAmount(service.inventorySnapshotLocked(act), ItemTreeSapling); got != 0 {
		t.Fatalf("saplings after plant: got %d, want 0", got)
	}
}

func TestHarvestTreeStageYields(t *testing.T) {
	for _, tt := range []struct {
		name  string
		stage uint8
		wood  uint32
	}{
		{name: "sapling", stage: TreeStageSapling, wood: 0},
		{name: "young", stage: TreeStageYoung, wood: 1},
		{name: "mature", stage: TreeStageMature, wood: 7},
		{name: "old", stage: TreeStageOld, wood: 11},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(nil, realtime.Config{VisibleChunkRadius: 1})
			act := service.SeedDemoActor(1)
			coord, index := world.ToChunkCoord(act.X, act.Y)
			ch := world.NewChunk(coord.X, coord.Y)
			ch.Base[index] = uint16(world.PackBase(world.BiomeBirchForest, world.SoilGrass, 8, 0))
			ch.Cover[index] = uint16(world.PackCover(world.CoverBirchForest, tt.stage, 0))
			ch.Stock[index] = uint16(treeWoodYield(tt.stage))
			if err := service.LoadChunks(1, map[world.ChunkCoord]*world.Chunk{coord: ch}); err != nil {
				t.Fatal(err)
			}

			if _, err := service.ApplyAction(context.Background(), 1, uint64(act.ID), ActionRequest{ActionType: "harvest"}); err != nil {
				t.Fatalf("harvest: %v", err)
			}

			service.mu.Lock()
			defer service.mu.Unlock()
			if got := service.chunkLocked(1, coord).Stock[index]; got != 0 {
				t.Fatalf("stock after fell: got %d, want 0", got)
			}
			if got := service.chunkLocked(1, coord).CoverCell(index).Kind(); got != world.CoverGrass {
				t.Fatalf("cover after fell: got %d, want grass", got)
			}
			inventory := service.inventorySnapshotLocked(act)
			if stackAmount(inventory, ItemWood) != tt.wood {
				t.Fatalf("wood after fell: got inventory %+v, want %d wood", inventory, tt.wood)
			}
			saplings := stackAmount(inventory, ItemTreeSapling)
			if tt.stage < TreeStageMature && saplings != 0 {
				t.Fatalf("saplings after young/sapling fell: got %d, want 0", saplings)
			}
			if saplings > 3 {
				t.Fatalf("saplings after mature/old fell: got %d, want <= 3", saplings)
			}
		})
	}
}

func TestForestGrowthAdvancesActiveSapling(t *testing.T) {
	service := NewService(nil, realtime.Config{VisibleChunkRadius: 0})
	act := service.SeedDemoActor(1)
	coord, index := world.ToChunkCoord(act.X, act.Y)
	ch := world.NewChunk(coord.X, coord.Y)
	ch.Base[index] = uint16(world.PackBase(world.BiomeBirchForest, world.SoilGrass, 8, 0))
	ch.Cover[index] = uint16(world.PackCover(world.CoverBirchForest, TreeStageSapling, 0))
	if err := service.LoadChunks(1, map[world.ChunkCoord]*world.Chunk{coord: ch}); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 256; i++ {
		if _, err := service.AdvanceForestGrowth(context.Background(), 1); err != nil {
			t.Fatalf("growth tick %d: %v", i, err)
		}
		service.mu.Lock()
		stage := treeStage(service.chunkLocked(1, coord).CoverCell(index))
		stock := service.chunkLocked(1, coord).Stock[index]
		service.mu.Unlock()
		if stage >= TreeStageYoung && stock == uint16(treeWoodYield(TreeStageYoung)) {
			return
		}
	}
	t.Fatalf("sapling did not advance after growth ticks")
}

func TestAdvanceWorldTimePublishesOnPhaseChange(t *testing.T) {
	hub := realtime.NewHub()
	service := NewService(hub, realtime.Config{VisibleChunkRadius: 1})
	service.SetClockConfig(ClockConfig{DayLengthSeconds: 480, SecondsPerTick: 1})
	act := service.SeedDemoWorld(1)
	coord, _ := world.ToChunkCoord(act.X, act.Y)
	client := hub.Subscribe(1, 1, map[world.ChunkCoord]struct{}{coord: {}})
	defer hub.Unsubscribe(client.ID)

	if _, changed := service.AdvanceWorldTime(1, 59); changed {
		t.Fatalf("phase changed too early")
	}
	select {
	case event := <-client.Events:
		t.Fatalf("unexpected event before phase change: %+v", event)
	case <-time.After(20 * time.Millisecond):
	}

	next, changed := service.AdvanceWorldTime(1, 1)
	if !changed {
		t.Fatalf("phase did not change")
	}
	if next.Phase != PhaseDawn {
		t.Fatalf("phase: got %s, want %s", next.Phase, PhaseDawn)
	}
	select {
	case event := <-client.Events:
		if event.Type != "world_time" {
			t.Fatalf("event type: got %q, want world_time", event.Type)
		}
	case <-time.After(time.Second):
		t.Fatalf("world_time event was not published")
	}
}

func TestHarvestPersistsThroughFileStoreRestart(t *testing.T) {
	dir := t.TempDir()
	mapPath := filepath.Join(dir, "world.islmap")
	coord, index := world.ToChunkCoord(DemoActorStartX, DemoActorStartY)
	writeGameTestMap(t, mapPath, coord, index, 3)

	store := storage.NewFileStore(mapPath, "")
	state, err := store.LoadWorld(context.Background())
	if err != nil {
		t.Fatalf("load world: %v", err)
	}
	service := NewService(realtime.NewHub(), realtime.Config{VisibleChunkRadius: 1})
	service.SetStore(store)
	if err := service.LoadWorld(1, state); err != nil {
		t.Fatalf("load service world: %v", err)
	}
	service.SeedDemoActor(1)

	if _, err := service.ApplyAction(context.Background(), 1, 1, ActionRequest{ActionType: "harvest"}); err != nil {
		t.Fatalf("harvest: %v", err)
	}

	restarted := storage.NewFileStore(mapPath, "")
	loaded, err := restarted.LoadWorld(context.Background())
	if err != nil {
		t.Fatalf("reload world: %v", err)
	}
	if got := loaded.Chunks[coord].Stock[index]; got != 0 {
		t.Fatalf("reloaded stock: got %d, want 0", got)
	}
	if got := loaded.Chunks[coord].CoverCell(index).Kind(); got != world.CoverGrass {
		t.Fatalf("reloaded cover: got %d, want grass", got)
	}
	if stackAmountFromStacks(loaded.Players.Stacks, ItemWood) != treeWoodYield(TreeStageMature) {
		t.Fatalf("reloaded inventory: got %+v", loaded.Players.Stacks)
	}
}

func TestMovePersistsActorThroughFileStoreRestart(t *testing.T) {
	dir := t.TempDir()
	mapPath := filepath.Join(dir, "world.islmap")
	coord, index := world.ToChunkCoord(DemoActorStartX, DemoActorStartY)
	writeGameTestMap(t, mapPath, coord, index, 3)

	store := storage.NewFileStore(mapPath, "")
	state, err := store.LoadWorld(context.Background())
	if err != nil {
		t.Fatalf("load world: %v", err)
	}
	service := NewService(realtime.NewHub(), realtime.Config{VisibleChunkRadius: 1})
	service.SetStore(store)
	if err := service.LoadWorld(1, state); err != nil {
		t.Fatalf("load service world: %v", err)
	}
	service.SeedDemoActor(1)
	targetCoord, targetIndex := world.ToChunkCoord(DemoActorStartX+1, DemoActorStartY)
	service.mu.Lock()
	service.chunkLocked(1, targetCoord).Surface[targetIndex] = uint16(world.PackSurface(world.SurfaceStoneRoad, 1, 0))
	service.mu.Unlock()

	result, err := service.ApplyAction(context.Background(), 1, 1, ActionRequest{ActionType: "move", X: DemoActorStartX + 1, Y: DemoActorStartY})
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	time.Sleep(time.Duration(result.MoveDelayMS+50) * time.Millisecond)

	restarted := storage.NewFileStore(mapPath, "")
	loaded, err := restarted.LoadWorld(context.Background())
	if err != nil {
		t.Fatalf("reload world: %v", err)
	}
	act := loaded.Players.Actors[1]
	if act == nil {
		t.Fatalf("actor was not persisted")
	}
	if act.X != DemoActorStartX+1 || act.Y != DemoActorStartY {
		t.Fatalf("actor position: got %d,%d", act.X, act.Y)
	}
}

func writeGameTestMap(t *testing.T, path string, coord world.ChunkCoord, index uint16, stock uint16) {
	t.Helper()
	ch := world.NewChunk(coord.X, coord.Y)
	ch.Base[index] = uint16(world.PackBase(world.BiomeBirchForest, world.SoilGrass, 8, 0))
	ch.Cover[index] = uint16(world.PackCover(world.CoverBirchForest, TreeStageMature, 0))
	ch.Stock[index] = stock
	m := &mapgen.Map{
		Width:  2048,
		Height: 2048,
		Config: mapgen.Config{Seed: "game-test"},
		Chunks: map[world.ChunkCoord]*world.Chunk{
			coord: ch,
		},
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create map: %v", err)
	}
	if err := mapgen.SaveBinary(file, m); err != nil {
		_ = file.Close()
		t.Fatalf("save map: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close map: %v", err)
	}
}

func stackAmount(items []InventoryItem, itemID inventory.ItemID) uint32 {
	for _, item := range items {
		if item.ItemID == itemID {
			return item.Amount
		}
	}
	return 0
}

func stackAmountFromStacks(stacks []inventory.Stack, itemID inventory.ItemID) uint32 {
	for _, stack := range stacks {
		if stack.ItemID == itemID {
			return stack.Amount
		}
	}
	return 0
}

type failingPlayerStore struct {
	err error
}

func (s failingPlayerStore) LoadWorld(ctx context.Context) (storage.WorldState, error) {
	return storage.WorldState{}, s.err
}

func (s failingPlayerStore) SaveDirtyChunk(ctx context.Context, ch *world.Chunk, tick uint64) error {
	return nil
}

func (s failingPlayerStore) SavePlayerState(ctx context.Context, state storage.PlayerState, tick uint64) error {
	return s.err
}

func (s failingPlayerStore) Flush(ctx context.Context) error {
	return nil
}

func (s failingPlayerStore) Compact(ctx context.Context, chunks map[world.ChunkCoord]*world.Chunk, tick uint64) error {
	return nil
}
