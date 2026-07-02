package mapgen

import (
	"bytes"
	"testing"
)

func TestSaveLoadBinaryRoundTrip(t *testing.T) {
	config := DefaultConfig()
	config.Width = 64
	config.Height = 64
	config.ContinentCount = 2

	generated, err := Generate(config)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	var buf bytes.Buffer
	if err := SaveBinary(&buf, generated); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := LoadBinary(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if loaded.Width != generated.Width || loaded.Height != generated.Height {
		t.Fatalf("size: got %dx%d, want %dx%d", loaded.Width, loaded.Height, generated.Width, generated.Height)
	}
	if len(loaded.Chunks) != len(generated.Chunks) {
		t.Fatalf("chunks: got %d, want %d", len(loaded.Chunks), len(generated.Chunks))
	}
	if loaded.Stats.Land != generated.Stats.Land || loaded.Stats.Water != generated.Stats.Water {
		t.Fatalf("stats mismatch: got %+v, want %+v", loaded.Stats, generated.Stats)
	}
}
