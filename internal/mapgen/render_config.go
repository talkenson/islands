package mapgen

import (
	"fmt"
	"image/color"
)

type RenderConfig struct {
	Seed    string              `json:"seed"`
	Palette RenderPaletteConfig `json:"palette"`
}

type RenderPaletteConfig struct {
	Water   WaterPaletteConfig   `json:"water"`
	Terrain TerrainPaletteConfig `json:"terrain"`
	Cover   CoverPaletteConfig   `json:"cover"`
	Biomes  map[string][]string  `json:"biomes"`
}

type WaterPaletteConfig struct {
	Sea          string `json:"sea"`
	Lake         string `json:"lake"`
	TidalSea     string `json:"tidal_sea"`
	TidalLake    string `json:"tidal_lake"`
	River        string `json:"river"`
	RiverVariant string `json:"river_variant"`
}

type TerrainPaletteConfig struct {
	Rock          string `json:"rock"`
	CoastalRock   string `json:"coastal_rock"`
	Sand          string `json:"sand"`
	MountainLight string `json:"mountain_light"`
	MountainDark  string `json:"mountain_dark"`
}

type CoverPaletteConfig struct {
	BirchForest string `json:"birch_forest"`
	PineForest  string `json:"pine_forest"`
	MixedForest string `json:"mixed_forest"`
	DryBush     string `json:"dry_bush"`
	Bush        string `json:"bush"`
}

func DefaultRenderConfig(seed string) RenderConfig {
	if seed == "" {
		seed = "demo"
	}
	return RenderConfig{
		Seed:    makeSeed(seed, "color"),
		Palette: renderPaletteConfig(DefaultRenderPalette),
	}
}

func renderPaletteConfig(p RenderPalette) RenderPaletteConfig {
	biomes := make(map[string][]string, len(p.Biomes))
	for biome, colors := range p.Biomes {
		values := make([]string, 0, len(colors))
		for _, c := range colors {
			values = append(values, colorHex(c))
		}
		biomes[fmt.Sprintf("%d", biome)] = values
	}

	return RenderPaletteConfig{
		Water: WaterPaletteConfig{
			Sea:          colorHex(p.Water.Sea),
			Lake:         colorHex(p.Water.Lake),
			TidalSea:     colorHex(p.Water.TidalSea),
			TidalLake:    colorHex(p.Water.TidalLake),
			River:        colorHex(p.Water.River),
			RiverVariant: colorHex(p.Water.RiverVariant),
		},
		Terrain: TerrainPaletteConfig{
			Rock:          colorHex(p.Terrain.Rock),
			CoastalRock:   colorHex(p.Terrain.CoastalRock),
			Sand:          colorHex(p.Terrain.Sand),
			MountainLight: colorHex(p.Terrain.MountainLight),
			MountainDark:  colorHex(p.Terrain.MountainDark),
		},
		Cover: CoverPaletteConfig{
			BirchForest: colorHex(p.Cover.BirchForest),
			PineForest:  colorHex(p.Cover.PineForest),
			MixedForest: colorHex(p.Cover.MixedForest),
			DryBush:     colorHex(p.Cover.DryBush),
			Bush:        colorHex(p.Cover.Bush),
		},
		Biomes: biomes,
	}
}

func colorHex(c color.RGBA) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}
