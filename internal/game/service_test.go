package game

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

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
		if event.Type != "chunk_snapshot" {
			t.Fatalf("event type: got %q, want chunk_snapshot", event.Type)
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
	act := service.SeedDemoWorld(1)
	startCoord, _ := world.ToChunkCoord(act.X, act.Y)

	client := hub.Subscribe(1, 1, map[world.ChunkCoord]struct{}{startCoord: {}})
	defer hub.Unsubscribe(client.ID)

	targetX := (startCoord.X + 1) * world.ChunkSize
	result, err := service.ApplyAction(context.Background(), 1, 1, ActionRequest{ActionType: "move", X: targetX, Y: act.Y})
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

	if _, err := service.ApplyAction(context.Background(), 1, 1, ActionRequest{ActionType: "move", X: DemoActorStartX + 5, Y: DemoActorStartY - 2}); err != nil {
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
	if act.X != DemoActorStartX+5 || act.Y != DemoActorStartY-2 {
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
