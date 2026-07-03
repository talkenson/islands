package game

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"islands/internal/actor"
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
	for _, key := range []string{"base", "cover", "stock"} {
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
		t.Fatal(err)
	}

	var patch InventoryPatch
	found := false
	for i := 0; i < 3; i++ {
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
	if len(patch.Inventory) != 1 || patch.Inventory[0].ItemID != ItemWood || patch.Inventory[0].Amount != 1 {
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
	if got := loaded.Chunks[coord].Stock[index]; got != 2 {
		t.Fatalf("reloaded stock: got %d, want 2", got)
	}
	if len(loaded.Players.Stacks) != 1 || loaded.Players.Stacks[0].ItemID != ItemWood || loaded.Players.Stacks[0].Amount != 1 {
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

	if _, err := service.ApplyAction(context.Background(), 1, 1, ActionRequest{ActionType: "move", X: DemoActorStartX + 1, Y: DemoActorStartY}); err != nil {
		t.Fatalf("move: %v", err)
	}

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
