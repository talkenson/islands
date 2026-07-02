package mapgen

import (
	"time"

	"islands/internal/world"
)

type Map struct {
	Width      int
	Height     int
	Config     Config
	Continents []Continent
	Chunks     map[world.ChunkCoord]*world.Chunk
	Stats      Stats

	heights []float64
}

type Continent struct {
	ID         int
	X, Y       float64
	RX, RY     float64
	Strength   float64
	Wobble     float64
	Lobes      []Lobe
	LakeBasins []LakeBasin
}

type Lobe struct {
	Frequency float64
	Amplitude float64
	Phase     float64
}

type LakeBasin struct {
	X, Y   float64
	RX, RY float64
	Depth  float64
}

type Stats struct {
	Land       int
	Water      int
	Shallow    int
	River      int
	Forest     int
	DryBush    int
	Mountain   int
	Rock       int
	WoodStock  int
	StoneStock int
}

type GenerateReport struct {
	Stages []StageTiming
}

type StageTiming struct {
	Name     string
	Duration time.Duration
}

func (m *Map) setHeight(x, y int, height float64) {
	if len(m.heights) != m.Width*m.Height {
		return
	}
	m.heights[y*m.Width+x] = height
}

func (m *Map) heightAt(x, y int) float64 {
	if len(m.heights) == m.Width*m.Height {
		return m.heights[y*m.Width+x]
	}
	ch, idx := chunkCell(m, x, y)
	return float64(ch.Meta[idx]) / 255
}
