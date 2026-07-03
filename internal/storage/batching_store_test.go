package storage

import (
	"os"
	"path/filepath"
	"testing"

	"islands/internal/actor"
	"islands/internal/inventory"
	"islands/internal/world"
)

func TestBatchingStoreFlushPersistsLatestChunkOnly(t *testing.T) {
	dir := t.TempDir()
	mapPath := filepath.Join(dir, "world.islmap")
	writeTestMap(t, mapPath, 42)

	base := NewFileStore(mapPath, "")
	store := NewBatchingStore(base, BatchConfig{MaxDirtyChunks: 10})

	ch := world.NewChunk(0, 0)
	ch.Stock[0] = 7
	if err := store.SaveDirtyChunk(nil, ch, 5); err != nil {
		t.Fatalf("save first chunk: %v", err)
	}
	ch.Stock[0] = 9
	if err := store.SaveDirtyChunk(nil, ch, 6); err != nil {
		t.Fatalf("save second chunk: %v", err)
	}
	if err := store.Flush(nil); err != nil {
		t.Fatalf("flush: %v", err)
	}

	restarted := NewFileStore(mapPath, "")
	state, err := restarted.LoadWorld(nil)
	if err != nil {
		t.Fatalf("load restarted world: %v", err)
	}
	if state.Tick != 6 {
		t.Fatalf("tick: got %d, want 6", state.Tick)
	}
	if got := state.Chunks[world.ChunkCoord{}].Stock[0]; got != 9 {
		t.Fatalf("stock: got %d, want 9", got)
	}

	records := countJournalRecords(t, DefaultJournalPath(mapPath))
	if records != 1 {
		t.Fatalf("journal records: got %d, want 1", records)
	}
}

func TestBatchingStoreFlushPersistsLatestPlayerState(t *testing.T) {
	dir := t.TempDir()
	mapPath := filepath.Join(dir, "world.islmap")
	writeTestMap(t, mapPath, 42)

	base := NewFileStore(mapPath, "")
	store := NewBatchingStore(base, BatchConfig{})

	first := PlayerState{Actors: map[actor.ID]*actor.Actor{
		1: &actor.Actor{ID: 1, WorldID: 1, X: 10, Y: 20, PocketInventoryID: 1},
	}}
	second := PlayerState{
		Actors: map[actor.ID]*actor.Actor{
			1: &actor.Actor{ID: 1, WorldID: 1, X: 30, Y: 40, PocketInventoryID: 1},
		},
		Inventories: map[inventory.ID]*inventory.Inventory{
			1: &inventory.Inventory{ID: 1, WorldID: 1, Kind: inventory.KindPocket, OwnerType: inventory.OwnerActor, OwnerID: 1},
		},
		Stacks: []inventory.Stack{{InventoryID: 1, ItemID: 1, Amount: 4}},
	}
	if err := store.SavePlayerState(nil, first, 2); err != nil {
		t.Fatalf("save first players: %v", err)
	}
	if err := store.SavePlayerState(nil, second, 3); err != nil {
		t.Fatalf("save second players: %v", err)
	}
	if err := store.Flush(nil); err != nil {
		t.Fatalf("flush: %v", err)
	}

	restarted := NewFileStore(mapPath, "")
	state, err := restarted.LoadWorld(nil)
	if err != nil {
		t.Fatalf("load restarted world: %v", err)
	}
	if state.Tick != 3 {
		t.Fatalf("tick: got %d, want 3", state.Tick)
	}
	act := state.Players.Actors[1]
	if act == nil || act.X != 30 || act.Y != 40 {
		t.Fatalf("actor: got %+v", act)
	}
	if len(state.Players.Stacks) != 1 || state.Players.Stacks[0].Amount != 4 {
		t.Fatalf("stacks: got %+v", state.Players.Stacks)
	}
}

func countJournalRecords(t *testing.T, path string) int {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open journal: %v", err)
	}
	defer file.Close()

	count := 0
	_, err = readJournal(file, func(record journalRecord) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	return count
}
