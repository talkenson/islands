package realtime

import (
	"testing"
	"time"

	"islands/internal/world"
)

func TestHubPublishesOnlyIntersectingChunkEvents(t *testing.T) {
	hub := NewHub()
	near := hub.Subscribe(1, 1, map[world.ChunkCoord]struct{}{{X: 0, Y: 0}: {}})
	defer hub.Unsubscribe(near.ID)
	far := hub.Subscribe(2, 1, map[world.ChunkCoord]struct{}{{X: 5, Y: 5}: {}})
	defer hub.Unsubscribe(far.ID)

	hub.Publish(Event{
		ID:            1,
		Type:          "chunk_snapshot",
		WorldID:       1,
		ChangedChunks: []world.ChunkCoord{{X: 0, Y: 0}},
	})

	select {
	case event := <-near.Events:
		if event.ID != 1 {
			t.Fatalf("event id: got %d, want 1", event.ID)
		}
	case <-time.After(time.Second):
		t.Fatalf("near client did not receive event")
	}

	select {
	case event := <-far.Events:
		t.Fatalf("far client received event %+v", event)
	case <-time.After(20 * time.Millisecond):
	}
}
