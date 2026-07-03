package mapgen

import (
	"testing"

	"islands/internal/world"
)

func TestChooseBiomeUsesHeightMoistureAndTemperature(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name        string
		height      float64
		moisture    float64
		temperature float64
		wantBiome   world.Biome
	}{
		{
			name:        "cold dry is not desert",
			height:      config.LandThreshold + 0.08,
			moisture:    0.2,
			temperature: 0.25,
			wantBiome:   world.BiomeSteppe,
		},
		{
			name:        "high wet is not marsh",
			height:      highlandHeight + 0.02,
			moisture:    0.9,
			temperature: 0.5,
			wantBiome:   world.BiomeTaiga,
		},
		{
			name:        "hot dry lowland is desert",
			height:      config.LandThreshold + 0.08,
			moisture:    0.12,
			temperature: 0.78,
			wantBiome:   world.BiomeDesert,
		},
		{
			name:        "cold moist is taiga",
			height:      config.LandThreshold + 0.08,
			moisture:    0.52,
			temperature: 0.22,
			wantBiome:   world.BiomeTaiga,
		},
		{
			name:        "very high altitude is mountain",
			height:      forcedMountainHeight + 0.01,
			moisture:    0.3,
			temperature: 0.7,
			wantBiome:   world.BiomeMountain,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBiome, _ := chooseBiome(tt.height, tt.moisture, tt.temperature, config)
			if gotBiome != tt.wantBiome {
				t.Fatalf("biome: got %d, want %d", gotBiome, tt.wantBiome)
			}
		})
	}
}

func TestRiverChannelIsDeeperAtCenter(t *testing.T) {
	config := DefaultConfig()
	config.Width = 16
	config.Height = 16
	m := &Map{
		Width:   config.Width,
		Height:  config.Height,
		Config:  config,
		Chunks:  make(map[world.ChunkCoord]*world.Chunk),
		heights: make([]float64, config.Width*config.Height),
	}
	initializeChunks(m, config)

	center := [2]int{8, 8}
	paintRiverChannel(m, center, 5, 0.8)

	centerChunk, centerIndex := chunkCell(m, center[0], center[1])
	edgeChunk, edgeIndex := chunkCell(m, center[0]+2, center[1])
	centerLevel := centerChunk.WaterCell(centerIndex).Level()
	edgeLevel := edgeChunk.WaterCell(edgeIndex).Level()

	if centerLevel <= edgeLevel {
		t.Fatalf("center river depth %d must be greater than edge depth %d", centerLevel, edgeLevel)
	}
	if centerLevel > 7 || edgeLevel < 1 {
		t.Fatalf("river depth out of range: center=%d edge=%d", centerLevel, edgeLevel)
	}
}
