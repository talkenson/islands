package realtime

import "islands/internal/world"

type Config struct {
	VisibleChunkRadius int32
}

func (c Config) Normalize() Config {
	if c.VisibleChunkRadius != 2 {
		c.VisibleChunkRadius = 1
	}
	return c
}

func VisibleChunks(center world.ChunkCoord, radius int32) map[world.ChunkCoord]struct{} {
	if radius < 0 {
		radius = 0
	}
	chunks := make(map[world.ChunkCoord]struct{}, (radius*2+1)*(radius*2+1))
	for y := center.Y - radius; y <= center.Y+radius; y++ {
		for x := center.X - radius; x <= center.X+radius; x++ {
			chunks[world.ChunkCoord{X: x, Y: y}] = struct{}{}
		}
	}
	return chunks
}

func Intersects(a map[world.ChunkCoord]struct{}, b []world.ChunkCoord) bool {
	if len(b) == 0 {
		return true
	}
	for _, coord := range b {
		if _, ok := a[coord]; ok {
			return true
		}
	}
	return false
}
