package storage

import (
	"context"
	"sync"
	"time"

	"islands/internal/actor"
	"islands/internal/inventory"
	"islands/internal/world"
)

type BatchConfig struct {
	FlushInterval  time.Duration
	MaxDirtyChunks int
}

func (c BatchConfig) Normalize() BatchConfig {
	if c.MaxDirtyChunks <= 0 {
		c.MaxDirtyChunks = 128
	}
	return c
}

type BatchingStore struct {
	base Store
	cfg  BatchConfig

	mu         sync.Mutex
	flushMu    sync.Mutex
	dirty      map[world.ChunkCoord]pendingChunk
	players    PlayerState
	hasPlayer  bool
	playerTick uint64
	lastErr    error
	wake       chan struct{}
}

type pendingChunk struct {
	ch   *world.Chunk
	tick uint64
}

func NewBatchingStore(base Store, cfg BatchConfig) *BatchingStore {
	cfg = cfg.Normalize()
	store := &BatchingStore{
		base:  base,
		cfg:   cfg,
		dirty: make(map[world.ChunkCoord]pendingChunk),
		wake:  make(chan struct{}, 1),
	}
	if cfg.FlushInterval > 0 {
		go store.run()
	}
	return store
}

func (s *BatchingStore) LoadWorld(ctx context.Context) (WorldState, error) {
	return s.base.LoadWorld(ctx)
}

func (s *BatchingStore) SaveDirtyChunk(ctx context.Context, ch *world.Chunk, tick uint64) error {
	_ = ctx
	if ch == nil {
		return nil
	}
	copied := copyChunk(ch)
	coord := world.ChunkCoord{X: copied.X, Y: copied.Y}

	s.mu.Lock()
	s.dirty[coord] = pendingChunk{ch: copied, tick: tick}
	shouldWake := len(s.dirty) >= s.cfg.MaxDirtyChunks
	s.mu.Unlock()

	if shouldWake {
		s.signalWake()
	}
	return nil
}

func (s *BatchingStore) SavePlayerState(ctx context.Context, state PlayerState, tick uint64) error {
	_ = ctx
	s.mu.Lock()
	s.players = copyPlayerState(state)
	s.playerTick = tick
	s.hasPlayer = true
	s.mu.Unlock()
	return nil
}

func (s *BatchingStore) Flush(ctx context.Context) error {
	return s.flushPending(ctx)
}

func (s *BatchingStore) Compact(ctx context.Context, chunks map[world.ChunkCoord]*world.Chunk, tick uint64) error {
	if err := s.Flush(ctx); err != nil {
		return err
	}
	return s.base.Compact(ctx, chunks, tick)
}

func (s *BatchingStore) run() {
	ticker := time.NewTicker(s.cfg.FlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = s.Flush(context.Background())
		case <-s.wake:
			_ = s.Flush(context.Background())
		}
	}
}

func (s *BatchingStore) flushPending(ctx context.Context) error {
	s.flushMu.Lock()
	defer s.flushMu.Unlock()

	chunks, players, hasPlayer, playerTick := s.takePending()
	if len(chunks) == 0 && !hasPlayer {
		s.mu.Lock()
		err := s.lastErr
		s.mu.Unlock()
		return err
	}

	if len(chunks) > 0 {
		if err := saveDirtyChunkBatch(ctx, s.base, chunks); err != nil {
			s.requeueChunks(chunks, err)
			if hasPlayer {
				s.requeuePlayer(players, playerTick, err)
			}
			return err
		}
	}
	if hasPlayer {
		if err := s.base.SavePlayerState(ctx, players, playerTick); err != nil {
			s.requeuePlayer(players, playerTick, err)
			return err
		}
	}

	s.mu.Lock()
	s.lastErr = nil
	s.mu.Unlock()
	return nil
}

func (s *BatchingStore) takePending() ([]DirtyChunk, PlayerState, bool, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	chunks := make([]DirtyChunk, 0, len(s.dirty))
	for _, pending := range s.dirty {
		chunks = append(chunks, DirtyChunk{Chunk: pending.ch, Tick: pending.tick})
	}
	s.dirty = make(map[world.ChunkCoord]pendingChunk)

	players := s.players
	hasPlayer := s.hasPlayer
	playerTick := s.playerTick
	s.players = PlayerState{}
	s.hasPlayer = false
	s.playerTick = 0

	return chunks, players, hasPlayer, playerTick
}

func (s *BatchingStore) requeueChunks(chunks []DirtyChunk, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastErr = err
	for _, dirty := range chunks {
		if dirty.Chunk == nil {
			continue
		}
		coord := world.ChunkCoord{X: dirty.Chunk.X, Y: dirty.Chunk.Y}
		if _, exists := s.dirty[coord]; !exists {
			s.dirty[coord] = pendingChunk{ch: dirty.Chunk, tick: dirty.Tick}
		}
	}
}

func (s *BatchingStore) requeuePlayer(state PlayerState, tick uint64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastErr = err
	if !s.hasPlayer {
		s.players = state
		s.playerTick = tick
		s.hasPlayer = true
	}
}

func (s *BatchingStore) signalWake() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func saveDirtyChunkBatch(ctx context.Context, store Store, chunks []DirtyChunk) error {
	if batch, ok := store.(DirtyChunkBatchWriter); ok {
		return batch.SaveDirtyChunks(ctx, chunks)
	}
	for _, dirty := range chunks {
		if dirty.Chunk == nil {
			continue
		}
		if err := store.SaveDirtyChunk(ctx, dirty.Chunk, dirty.Tick); err != nil {
			return err
		}
	}
	return nil
}

func copyPlayerState(state PlayerState) PlayerState {
	copied := PlayerState{
		Actors:      make(map[actor.ID]*actor.Actor, len(state.Actors)),
		Inventories: make(map[inventory.ID]*inventory.Inventory, len(state.Inventories)),
		Stacks:      append([]inventory.Stack(nil), state.Stacks...),
		WorldTime:   state.WorldTime,
	}
	for id, act := range state.Actors {
		if act == nil {
			continue
		}
		next := *act
		copied.Actors[id] = &next
	}
	for id, inv := range state.Inventories {
		if inv == nil {
			continue
		}
		next := *inv
		copied.Inventories[id] = &next
	}
	return copied
}

func copyChunk(ch *world.Chunk) *world.Chunk {
	if ch == nil {
		return nil
	}
	next := world.NewChunk(ch.X, ch.Y)
	copy(next.Base, ch.Base)
	copy(next.Water, ch.Water)
	copy(next.Cover, ch.Cover)
	copy(next.Stock, ch.Stock)
	copy(next.Meta, ch.Meta)
	copy(next.Temperature, ch.Temperature)
	next.Dirty = ch.Dirty
	return next
}
