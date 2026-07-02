package realtime

import (
	"testing"

	"islands/internal/world"
)

func TestVisibleChunksRadius(t *testing.T) {
	center := world.ChunkCoord{X: 10, Y: -4}

	if got := len(VisibleChunks(center, 1)); got != 9 {
		t.Fatalf("3x3 chunk count: got %d, want 9", got)
	}
	if got := len(VisibleChunks(center, 2)); got != 25 {
		t.Fatalf("5x5 chunk count: got %d, want 25", got)
	}
}

func TestConfigNormalizeAllowsOnlySupportedRadii(t *testing.T) {
	if got := (Config{}).Normalize().VisibleChunkRadius; got != 1 {
		t.Fatalf("default radius: got %d, want 1", got)
	}
	if got := (Config{VisibleChunkRadius: 2}).Normalize().VisibleChunkRadius; got != 2 {
		t.Fatalf("explicit 5x5 radius: got %d, want 2", got)
	}
	if got := (Config{VisibleChunkRadius: 7}).Normalize().VisibleChunkRadius; got != 1 {
		t.Fatalf("unsupported radius: got %d, want 1", got)
	}
}
