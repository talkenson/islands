package game

import (
	"context"
	"testing"
	"time"

	"islands/internal/realtime"
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
