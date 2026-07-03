package mapgen

import (
	"image/color"

	"islands/internal/world"
)

type RenderPalette struct {
	Water   WaterPalette
	Terrain TerrainPalette
	Cover   CoverPalette
	Biomes  map[world.Biome][]color.RGBA
}

type WaterPalette struct {
	Sea          color.RGBA
	Lake         color.RGBA
	TidalSea     color.RGBA
	TidalLake    color.RGBA
	River        color.RGBA
	RiverVariant color.RGBA
}

type TerrainPalette struct {
	Rock          color.RGBA
	CoastalRock   color.RGBA
	Sand          color.RGBA
	MountainLight color.RGBA
	MountainDark  color.RGBA
}

type CoverPalette struct {
	BirchForest color.RGBA
	PineForest  color.RGBA
	MixedForest color.RGBA
	DryBush     color.RGBA
	Bush        color.RGBA
}

var DefaultRenderPalette = RenderPalette{
	Water: WaterPalette{
		Sea:          hexColor("#204063"),
		Lake:         hexColor("#204063"),
		TidalSea:     hexColor("#255b94"),
		TidalLake:    hexColor("#338ab9"),
		River:        hexColor("#3a92b8"),
		RiverVariant: hexColor("#2883ad"),
	},
	Terrain: TerrainPalette{
		Rock:          hexColor("#777b74"),
		CoastalRock:   hexColor("#8b8879"),
		Sand:          hexColor("#d6bd75"),
		MountainLight: hexColor("#a6aaa2"),
		MountainDark:  hexColor("#6e736d"),
	},
	Cover: CoverPalette{
		BirchForest: hexColor("#2f6b35"),
		PineForest:  hexColor("#234f2d"),
		MixedForest: hexColor("#2d4d3f"),
		DryBush:     hexColor("#756f35"),
		Bush:        hexColor("#756f35"),
	},
	Biomes: map[world.Biome][]color.RGBA{
		world.BiomeTaiga: {
			hexColor("#42624b"),
			hexColor("#385640"),
			hexColor("#4f7055"),
			hexColor("#314939"),
		},
		world.BiomeBirchForest: {
			hexColor("#6fa85f"),
			hexColor("#7bb96b"),
			hexColor("#5f984f"),
			hexColor("#83ad64"),
		},
		world.BiomeTemperateForest: {
			hexColor("#597f52"),
			hexColor("#638c5b"),
			hexColor("#4e704a"),
			hexColor("#729762"),
		},
		world.BiomeRiverValley: {
			hexColor("#5f984f"),
			hexColor("#6fa85f"),
			hexColor("#83ad64"),
			hexColor("#7bb96b"),
		},
		world.BiomeCoast: {
			hexColor("#cdbb7b"),
			hexColor("#d6bd75"),
			hexColor("#b9aa78"),
			hexColor("#a9a083"),
		},
		world.BiomeMarsh: {
			hexColor("#4f735f"),
			hexColor("#5a806a"),
			hexColor("#3f604f"),
			hexColor("#63766a"),
		},
		world.BiomeSteppe: {
			hexColor("#8f9f52"),
			hexColor("#a6b866"),
			hexColor("#b8c878"),
			hexColor("#7f8f48"),
		},
		world.BiomeDesert: {
			hexColor("#d7c27a"),
			hexColor("#c9ad62"),
			hexColor("#e0cf91"),
			hexColor("#b99a55"),
		},
		world.BiomeMeadow: {
			hexColor("#88aa5d"),
			hexColor("#9bbb66"),
			hexColor("#79a15a"),
			hexColor("#a8c874"),
		},
	},
}

func biomeColors(palette RenderPalette, biome world.Biome) []color.RGBA {
	if colors, ok := palette.Biomes[biome]; ok && len(colors) > 0 {
		return colors
	}
	return palette.Biomes[world.BiomeTemperateForest]
}
