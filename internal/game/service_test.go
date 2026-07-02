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
	service.SeedDemoWorld(1)

	visible := hub.Subscribe(1, 1, map[world.ChunkCoord]struct{}{{X: 0, Y: 0}: {}})
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
