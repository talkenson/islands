package storage

import (
	"context"

	"islands/internal/actor"
	"islands/internal/inventory"
	"islands/internal/world"
)

type WorldState struct {
	Width   int
	Height  int
	Seed    string
	Tick    uint64
	Chunks  map[world.ChunkCoord]*world.Chunk
	Players PlayerState
}

type PlayerState struct {
	Actors      map[actor.ID]*actor.Actor
	Inventories map[inventory.ID]*inventory.Inventory
	Stacks      []inventory.Stack
}

type Store interface {
	LoadWorld(ctx context.Context) (WorldState, error)
	SaveDirtyChunk(ctx context.Context, ch *world.Chunk, tick uint64) error
	SavePlayerState(ctx context.Context, state PlayerState, tick uint64) error
	Flush(ctx context.Context) error
	Compact(ctx context.Context, chunks map[world.ChunkCoord]*world.Chunk, tick uint64) error
}

type DirtyChunk struct {
	Chunk *world.Chunk
	Tick  uint64
}

type DirtyChunkBatchWriter interface {
	SaveDirtyChunks(ctx context.Context, chunks []DirtyChunk) error
}
