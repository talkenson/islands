package storage

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"islands/internal/actor"
	"islands/internal/codec"
	"islands/internal/inventory"
	"islands/internal/mapgen"
	"islands/internal/world"
)

func TestJournalRecordRoundTrip(t *testing.T) {
	ch := world.NewChunk(1, 2)
	ch.Stock[5] = 99

	var buf bytes.Buffer
	if err := writeJournalRecord(&buf, 7, ch); err != nil {
		t.Fatalf("write record: %v", err)
	}

	tick, err := readJournal(bytes.NewReader(buf.Bytes()), func(record journalRecord) error {
		if record.Tick != 7 {
			t.Fatalf("tick: got %d, want 7", record.Tick)
		}
		loaded, err := codec.DecodeChunk(record.Payload)
		if err != nil {
			return err
		}
		if loaded.Stock[5] != 99 {
			t.Fatalf("stock: got %d, want 99", loaded.Stock[5])
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	if tick != 7 {
		t.Fatalf("max tick: got %d, want 7", tick)
	}
}

func TestJournalRejectsChecksumMismatch(t *testing.T) {
	ch := world.NewChunk(1, 2)
	var buf bytes.Buffer
	if err := writeJournalRecord(&buf, 1, ch); err != nil {
		t.Fatalf("write record: %v", err)
	}
	data := buf.Bytes()
	data[len(data)-1] ^= 0xff

	_, err := readJournal(bytes.NewReader(data), func(record journalRecord) error { return nil })
	if !errors.Is(err, ErrJournalChecksum) {
		t.Fatalf("read journal error: got %v, want %v", err, ErrJournalChecksum)
	}
}

func TestJournalRejectsTruncatedTail(t *testing.T) {
	ch := world.NewChunk(1, 2)
	var buf bytes.Buffer
	if err := writeJournalRecord(&buf, 1, ch); err != nil {
		t.Fatalf("write record: %v", err)
	}
	data := buf.Bytes()[:buf.Len()-10]

	_, err := readJournal(bytes.NewReader(data), func(record journalRecord) error { return nil })
	if !errors.Is(err, ErrJournalTruncated) {
		t.Fatalf("read journal error: got %v, want %v", err, ErrJournalTruncated)
	}
}

func TestFileStoreLoadsBaseMapWithoutJournal(t *testing.T) {
	dir := t.TempDir()
	mapPath := filepath.Join(dir, "world.islmap")
	writeTestMap(t, mapPath, 42)

	store := NewFileStore(mapPath, "")
	state, err := store.LoadWorld(nil)
	if err != nil {
		t.Fatalf("load world: %v", err)
	}
	if state.Seed != "test-seed" {
		t.Fatalf("seed: got %q", state.Seed)
	}
	if got := state.Chunks[world.ChunkCoord{}].Stock[0]; got != 42 {
		t.Fatalf("stock: got %d, want 42", got)
	}
}

func TestFileStoreJournalOverridesBaseMap(t *testing.T) {
	dir := t.TempDir()
	mapPath := filepath.Join(dir, "world.islmap")
	writeTestMap(t, mapPath, 42)

	store := NewFileStore(mapPath, "")
	ch := world.NewChunk(0, 0)
	ch.Stock[0] = 7
	if err := store.SaveDirtyChunk(nil, ch, 5); err != nil {
		t.Fatalf("save dirty chunk: %v", err)
	}

	restarted := NewFileStore(mapPath, "")
	state, err := restarted.LoadWorld(nil)
	if err != nil {
		t.Fatalf("load restarted world: %v", err)
	}
	if state.Tick != 5 {
		t.Fatalf("tick: got %d, want 5", state.Tick)
	}
	if got := state.Chunks[world.ChunkCoord{}].Stock[0]; got != 7 {
		t.Fatalf("stock: got %d, want 7", got)
	}
}

func TestFileStoreCompactWritesMapAndClearsJournal(t *testing.T) {
	dir := t.TempDir()
	mapPath := filepath.Join(dir, "world.islmap")
	writeTestMap(t, mapPath, 42)

	store := NewFileStore(mapPath, "")
	state, err := store.LoadWorld(nil)
	if err != nil {
		t.Fatalf("load world: %v", err)
	}
	state.Chunks[world.ChunkCoord{}].Stock[0] = 11
	if err := store.SaveDirtyChunk(nil, state.Chunks[world.ChunkCoord{}], 2); err != nil {
		t.Fatalf("save dirty chunk: %v", err)
	}
	if err := store.Compact(nil, state.Chunks, 2); err != nil {
		t.Fatalf("compact: %v", err)
	}

	journalPath := DefaultJournalPath(mapPath)
	journal, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	if len(journal) != 0 {
		t.Fatalf("journal not cleared: %d bytes", len(journal))
	}

	restarted := NewFileStore(mapPath, "")
	loaded, err := restarted.LoadWorld(nil)
	if err != nil {
		t.Fatalf("load compacted world: %v", err)
	}
	if got := loaded.Chunks[world.ChunkCoord{}].Stock[0]; got != 11 {
		t.Fatalf("stock: got %d, want 11", got)
	}
}

func TestFileStorePersistsPlayerState(t *testing.T) {
	dir := t.TempDir()
	mapPath := filepath.Join(dir, "world.islmap")
	writeTestMap(t, mapPath, 42)
	store := NewFileStore(mapPath, "")

	playerState := PlayerState{
		Actors: map[actor.ID]*actor.Actor{
			1: &actor.Actor{ID: 1, WorldID: 1, X: 12, Y: 34, PocketInventoryID: 1},
		},
		Inventories: map[inventory.ID]*inventory.Inventory{
			1: &inventory.Inventory{ID: 1, WorldID: 1, Kind: inventory.KindPocket, OwnerType: inventory.OwnerActor, OwnerID: 1},
		},
		Stacks: []inventory.Stack{{InventoryID: 1, ItemID: 1, Amount: 3}},
	}
	if err := store.SavePlayerState(nil, playerState, 9); err != nil {
		t.Fatalf("save players: %v", err)
	}

	restarted := NewFileStore(mapPath, "")
	state, err := restarted.LoadWorld(nil)
	if err != nil {
		t.Fatalf("load world: %v", err)
	}
	if state.Tick != 9 {
		t.Fatalf("tick: got %d, want 9", state.Tick)
	}
	act := state.Players.Actors[1]
	if act == nil || act.X != 12 || act.Y != 34 {
		t.Fatalf("actor: got %+v", act)
	}
	if len(state.Players.Stacks) != 1 || state.Players.Stacks[0].Amount != 3 {
		t.Fatalf("stacks: got %+v", state.Players.Stacks)
	}
}

func writeTestMap(t *testing.T, path string, stock uint16) {
	t.Helper()
	ch := world.NewChunk(0, 0)
	ch.Stock[0] = stock
	m := &mapgen.Map{
		Width:  world.ChunkSize,
		Height: world.ChunkSize,
		Config: mapgen.Config{Seed: "test-seed"},
		Chunks: map[world.ChunkCoord]*world.Chunk{
			{}: ch,
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
