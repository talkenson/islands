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
	if loaded.Config.Seed != generated.Config.Seed {
		t.Fatalf("seed: got %q, want %q", loaded.Config.Seed, generated.Config.Seed)
	}
	if len(loaded.Chunks) != len(generated.Chunks) {
		t.Fatalf("chunks: got %d, want %d", len(loaded.Chunks), len(generated.Chunks))
	}
	if loaded.Stats.Land != generated.Stats.Land || loaded.Stats.Water != generated.Stats.Water {
		t.Fatalf("stats mismatch: got %+v, want %+v", loaded.Stats, generated.Stats)
	}
	for coord, generatedChunk := range generated.Chunks {
		loadedChunk := loaded.Chunks[coord]
		if loadedChunk == nil {
			t.Fatalf("missing loaded chunk %v", coord)
		}
		if !bytes.Equal(loadedChunk.Temperature, generatedChunk.Temperature) {
			t.Fatalf("temperature mismatch in chunk %v", coord)
		}
		if !bytes.Equal(uint16Bytes(loadedChunk.Surface), uint16Bytes(generatedChunk.Surface)) {
			t.Fatalf("surface mismatch in chunk %v", coord)
		}
		break
	}
}

func uint16Bytes(values []uint16) []byte {
	out := make([]byte, 0, len(values)*2)
	for _, value := range values {
		out = append(out, byte(value), byte(value>>8))
	}
	return out
}

func TestGenerateWorkersIsDeterministic(t *testing.T) {
	config := DefaultConfig()
	config.Width = 96
	config.Height = 96
	config.ContinentCount = 2
	config.RiverCount = 4

	serialConfig := config
	serialConfig.Workers = 1
	serial, err := Generate(serialConfig)
	if err != nil {
		t.Fatalf("serial generate: %v", err)
	}

	parallelConfig := config
	parallelConfig.Workers = 4
	parallel, err := Generate(parallelConfig)
	if err != nil {
		t.Fatalf("parallel generate: %v", err)
	}

	var serialBuf bytes.Buffer
	if err := SaveBinary(&serialBuf, serial); err != nil {
		t.Fatalf("serial save: %v", err)
	}
	var parallelBuf bytes.Buffer
	if err := SaveBinary(&parallelBuf, parallel); err != nil {
		t.Fatalf("parallel save: %v", err)
	}

	if !bytes.Equal(serialBuf.Bytes(), parallelBuf.Bytes()) {
		t.Fatalf("parallel generation changed map output")
	}
}
