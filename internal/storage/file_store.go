package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"islands/internal/actor"
	"islands/internal/codec"
	"islands/internal/inventory"
	"islands/internal/mapgen"
	"islands/internal/world"
)

type FileStore struct {
	mu          sync.Mutex
	mapPath     string
	journalPath string
	playersPath string

	width  int
	height int
	seed   string
}

func NewFileStore(mapPath, journalPath string, playersPath ...string) *FileStore {
	if journalPath == "" && mapPath != "" {
		journalPath = defaultJournalPath(mapPath)
	}
	resolvedPlayersPath := ""
	if len(playersPath) > 0 {
		resolvedPlayersPath = playersPath[0]
	}
	if resolvedPlayersPath == "" && mapPath != "" {
		resolvedPlayersPath = defaultPlayersPath(mapPath)
	}
	return &FileStore{mapPath: mapPath, journalPath: journalPath, playersPath: resolvedPlayersPath}
}

func DefaultJournalPath(mapPath string) string {
	return defaultJournalPath(mapPath)
}

func DefaultPlayersPath(mapPath string) string {
	return defaultPlayersPath(mapPath)
}

func (s *FileStore) LoadWorld(ctx context.Context) (WorldState, error) {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()

	m, err := s.loadBaseMapLocked()
	if err != nil {
		return WorldState{}, err
	}

	state := WorldState{
		Width:  m.Width,
		Height: m.Height,
		Seed:   m.Config.Seed,
		Chunks: m.Chunks,
	}

	tick, err := s.applyJournalLocked(state.Chunks)
	if err != nil {
		return WorldState{}, err
	}
	state.Tick = tick
	players, playersTick, err := s.loadPlayerStateLocked()
	if err != nil {
		return WorldState{}, err
	}
	state.Players = players
	if playersTick > state.Tick {
		state.Tick = playersTick
	}
	s.width = state.Width
	s.height = state.Height
	s.seed = state.Seed
	return state, nil
}

func (s *FileStore) SaveDirtyChunk(ctx context.Context, ch *world.Chunk, tick uint64) error {
	return s.SaveDirtyChunks(ctx, []DirtyChunk{{Chunk: ch, Tick: tick}})
}

func (s *FileStore) SaveDirtyChunks(ctx context.Context, chunks []DirtyChunk) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(chunks) == 0 {
		return nil
	}
	if s.journalPath == "" {
		return fmt.Errorf("journal path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(s.journalPath), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(s.journalPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	for _, dirty := range chunks {
		if dirty.Chunk == nil {
			return fmt.Errorf("dirty chunk is nil")
		}
		if err := writeJournalRecord(file, dirty.Tick, dirty.Chunk); err != nil {
			return err
		}
	}
	return file.Sync()
}

func (s *FileStore) SavePlayerState(ctx context.Context, state PlayerState, tick uint64) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.playersPath == "" {
		return fmt.Errorf("players path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(s.playersPath), 0o755); err != nil {
		return err
	}
	payload := filePlayerState{
		Version:     1,
		Tick:        tick,
		WorldTime:   state.WorldTime,
		Actors:      make([]actor.Actor, 0, len(state.Actors)),
		Inventories: make([]inventory.Inventory, 0, len(state.Inventories)),
		Stacks:      append([]inventory.Stack(nil), state.Stacks...),
	}
	for _, act := range state.Actors {
		if act != nil {
			payload.Actors = append(payload.Actors, *act)
		}
	}
	for _, inv := range state.Inventories {
		if inv != nil {
			payload.Inventories = append(payload.Inventories, *inv)
		}
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmpPath := s.playersPath + ".tmp"
	if err := writeSyncedFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, s.playersPath); err != nil {
		return err
	}
	return syncDir(filepath.Dir(s.playersPath))
}

func (s *FileStore) Flush(ctx context.Context) error {
	_ = ctx
	return nil
}

func (s *FileStore) Compact(ctx context.Context, chunks map[world.ChunkCoord]*world.Chunk, tick uint64) error {
	_ = ctx
	_ = tick
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.mapPath == "" {
		return fmt.Errorf("map path is empty")
	}
	if len(chunks) == 0 {
		return fmt.Errorf("compact requires chunks")
	}
	if s.width == 0 || s.height == 0 {
		m, err := s.loadBaseMapLocked()
		if err != nil {
			return err
		}
		s.width = m.Width
		s.height = m.Height
		s.seed = m.Config.Seed
	}

	tmpPath := s.mapPath + ".tmp"
	file, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	m := &mapgen.Map{
		Width:  s.width,
		Height: s.height,
		Config: mapgen.Config{Seed: s.seed},
		Chunks: copyChunks(chunks),
	}
	if err := mapgen.SaveBinary(file, m); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, s.mapPath); err != nil {
		return err
	}
	if s.journalPath != "" {
		if err := os.WriteFile(s.journalPath, nil, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (s *FileStore) loadBaseMapLocked() (*mapgen.Map, error) {
	if s.mapPath == "" {
		return nil, fmt.Errorf("map path is empty")
	}
	file, err := os.Open(s.mapPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return mapgen.LoadBinary(file)
}

func (s *FileStore) applyJournalLocked(chunks map[world.ChunkCoord]*world.Chunk) (uint64, error) {
	if s.journalPath == "" {
		return 0, nil
	}
	file, err := os.Open(s.journalPath)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer file.Close()

	return readJournal(file, func(record journalRecord) error {
		ch, err := codec.DecodeChunk(record.Payload)
		if err != nil {
			return err
		}
		if ch.X != record.Coord.X || ch.Y != record.Coord.Y {
			return fmt.Errorf("journal coord mismatch: header=%d,%d payload=%d,%d", record.Coord.X, record.Coord.Y, ch.X, ch.Y)
		}
		ch.Dirty = false
		chunks[record.Coord] = ch
		return nil
	})
}

type filePlayerState struct {
	Version     uint16                `json:"version"`
	Tick        uint64                `json:"tick"`
	WorldTime   uint64                `json:"world_time,omitempty"`
	Actors      []actor.Actor         `json:"actors"`
	Inventories []inventory.Inventory `json:"inventories"`
	Stacks      []inventory.Stack     `json:"stacks"`
}

func (s *FileStore) loadPlayerStateLocked() (PlayerState, uint64, error) {
	state := PlayerState{
		Actors:      make(map[actor.ID]*actor.Actor),
		Inventories: make(map[inventory.ID]*inventory.Inventory),
	}
	if s.playersPath == "" {
		return state, 0, nil
	}
	data, err := os.ReadFile(s.playersPath)
	if errors.Is(err, os.ErrNotExist) {
		return state, 0, nil
	}
	if err != nil {
		return PlayerState{}, 0, err
	}
	var payload filePlayerState
	if err := json.Unmarshal(data, &payload); err != nil {
		return PlayerState{}, 0, err
	}
	if payload.Version != 1 {
		return PlayerState{}, 0, fmt.Errorf("unsupported players version %d", payload.Version)
	}
	for _, act := range payload.Actors {
		copied := act
		state.Actors[act.ID] = &copied
	}
	for _, inv := range payload.Inventories {
		copied := inv
		state.Inventories[inv.ID] = &copied
	}
	state.Stacks = append([]inventory.Stack(nil), payload.Stacks...)
	state.WorldTime = payload.WorldTime
	return state, payload.Tick, nil
}

func defaultJournalPath(mapPath string) string {
	ext := filepath.Ext(mapPath)
	if ext == "" {
		return mapPath + ".journal"
	}
	return mapPath[:len(mapPath)-len(ext)] + ".journal"
}

func defaultPlayersPath(mapPath string) string {
	ext := filepath.Ext(mapPath)
	if ext == "" {
		return mapPath + ".players.json"
	}
	return mapPath[:len(mapPath)-len(ext)] + ".players.json"
}

func syncDir(dir string) error {
	file, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}

func writeSyncedFile(path string, data []byte, perm os.FileMode) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func copyChunks(chunks map[world.ChunkCoord]*world.Chunk) map[world.ChunkCoord]*world.Chunk {
	copied := make(map[world.ChunkCoord]*world.Chunk, len(chunks))
	for coord, ch := range chunks {
		if ch == nil {
			continue
		}
		next := world.NewChunk(ch.X, ch.Y)
		copy(next.Base, ch.Base)
		copy(next.Water, ch.Water)
		copy(next.Cover, ch.Cover)
		copy(next.Stock, ch.Stock)
		copy(next.Meta, ch.Meta)
		copy(next.Temperature, ch.Temperature)
		copied[coord] = next
	}
	return copied
}
