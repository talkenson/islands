package world

import "testing"

func TestPackBaseRoundTrip(t *testing.T) {
	cell := PackBase(BiomeTaiga, SoilFertile, 17, 2)

	if got := cell.Biome(); got != BiomeTaiga {
		t.Fatalf("biome: got %d", got)
	}
	if got := cell.Soil(); got != SoilFertile {
		t.Fatalf("soil: got %d", got)
	}
	if got := cell.Elevation(); got != 17 {
		t.Fatalf("elevation: got %d", got)
	}
	if got := cell.Flags(); got != 2 {
		t.Fatalf("flags: got %d", got)
	}
}

func TestPackWaterRoundTrip(t *testing.T) {
	cell := PackWater(WaterSea, 3, true)

	if got := cell.Kind(); got != WaterSea {
		t.Fatalf("kind: got %d", got)
	}
	if got := cell.Level(); got != 3 {
		t.Fatalf("level: got %d", got)
	}
	if !cell.Tidal() {
		t.Fatalf("tidal: got false")
	}
}

func TestPackCoverRoundTrip(t *testing.T) {
	cell := PackCover(CoverPineForest, 4, 9)

	if got := cell.Kind(); got != CoverPineForest {
		t.Fatalf("kind: got %d", got)
	}
	if got := cell.Level(); got != 4 {
		t.Fatalf("level: got %d", got)
	}
	if got := cell.Flags(); got != 9 {
		t.Fatalf("flags: got %d", got)
	}
}

func TestToChunkCoord(t *testing.T) {
	tests := []struct {
		name      string
		x, y      int32
		wantCoord ChunkCoord
		wantIndex uint16
	}{
		{name: "origin", x: 0, y: 0, wantCoord: ChunkCoord{0, 0}, wantIndex: 0},
		{name: "last local cell", x: 31, y: 31, wantCoord: ChunkCoord{0, 0}, wantIndex: 1023},
		{name: "next chunk", x: 32, y: 0, wantCoord: ChunkCoord{1, 0}, wantIndex: 0},
		{name: "negative x", x: -1, y: 0, wantCoord: ChunkCoord{-1, 0}, wantIndex: 31},
		{name: "negative y", x: 0, y: -1, wantCoord: ChunkCoord{0, -1}, wantIndex: 992},
		{name: "negative both", x: -33, y: -33, wantCoord: ChunkCoord{-2, -2}, wantIndex: 1023},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCoord, gotIndex := ToChunkCoord(tt.x, tt.y)
			if gotCoord != tt.wantCoord {
				t.Fatalf("coord: got %+v, want %+v", gotCoord, tt.wantCoord)
			}
			if gotIndex != tt.wantIndex {
				t.Fatalf("index: got %d, want %d", gotIndex, tt.wantIndex)
			}
		})
	}
}
