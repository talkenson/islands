package game

import "islands/internal/world"

func snapshotChunk(ch *world.Chunk, tick uint64) ChunkSnapshot {
	return ChunkSnapshot{
		CX:          ch.X,
		CY:          ch.Y,
		Base:        Uint16Layer(append([]uint16(nil), ch.Base...)),
		Water:       append([]uint8(nil), ch.Water...),
		Cover:       Uint16Layer(append([]uint16(nil), ch.Cover...)),
		Surface:     Uint16Layer(append([]uint16(nil), ch.Surface...)),
		Stock:       Uint16Layer(append([]uint16(nil), ch.Stock...)),
		Meta:        append([]uint8(nil), ch.Meta...),
		Temperature: append([]uint8(nil), ch.Temperature...),
		UpdatedTick: tick,
	}
}

func interestList(interest map[world.ChunkCoord]struct{}) []world.ChunkCoord {
	out := make([]world.ChunkCoord, 0, len(interest))
	for coord := range interest {
		out = append(out, coord)
	}
	return out
}

func interestDifference(next, previous map[world.ChunkCoord]struct{}) map[world.ChunkCoord]struct{} {
	out := make(map[world.ChunkCoord]struct{})
	for coord := range next {
		if _, ok := previous[coord]; !ok {
			out[coord] = struct{}{}
		}
	}
	return out
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
		copy(next.Surface, ch.Surface)
		copy(next.Stock, ch.Stock)
		copy(next.Meta, ch.Meta)
		copy(next.Temperature, ch.Temperature)
		next.Dirty = ch.Dirty
		copied[coord] = next
	}
	return copied
}
