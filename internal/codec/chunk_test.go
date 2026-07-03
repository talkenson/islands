package codec

import (
	"testing"

	"islands/internal/world"
)

func TestChunkRoundTrip(t *testing.T) {
	ch := world.NewChunk(2, -3)
	ch.Base[0] = 12
	ch.Water[1] = 3
	ch.Cover[2] = 45
	ch.Stock[3] = 67
	ch.Meta[4] = 89

	payload, err := EncodeChunk(ch)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	loaded, err := DecodeChunk(payload)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if loaded.X != ch.X || loaded.Y != ch.Y {
		t.Fatalf("coord: got %d,%d want %d,%d", loaded.X, loaded.Y, ch.X, ch.Y)
	}
	if loaded.Base[0] != 12 || loaded.Water[1] != 3 || loaded.Cover[2] != 45 || loaded.Stock[3] != 67 || loaded.Meta[4] != 89 {
		t.Fatalf("round trip changed chunk arrays")
	}
}

func TestDecodeChunkRejectsBadPayload(t *testing.T) {
	if _, err := DecodeChunk([]byte("short")); err == nil {
		t.Fatalf("decode short payload succeeded")
	}

	ch := world.NewChunk(0, 0)
	payload, err := EncodeChunk(ch)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	payload[0] = 'X'
	if _, err := DecodeChunk(payload); err == nil {
		t.Fatalf("decode bad magic succeeded")
	}
}
