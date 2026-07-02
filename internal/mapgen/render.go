package mapgen

import (
	"image"
	"image/color"
	"image/png"
	"io"
	"math"

	"islands/internal/world"
)

func RenderPNG(w io.Writer, m *Map, pixelSize int) error {
	return RenderPNGWithPalette(w, m, pixelSize, DefaultRenderPalette)
}

func RenderPNGWithPalette(w io.Writer, m *Map, pixelSize int, palette RenderPalette) error {
	if pixelSize < 1 {
		pixelSize = 1
	}
	colorNoise := newValueNoise(makeSeed(m.Config.Seed, "color"))
	img := image.NewRGBA(image.Rect(0, 0, m.Width*pixelSize, m.Height*pixelSize))

	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			c := cellColor(m, x, y, colorNoise, palette)
			fillPixel(img, x*pixelSize, y*pixelSize, pixelSize, c)
		}
	}

	return png.Encode(w, img)
}

func cellColor(m *Map, x, y int, colorNoise *valueNoise, palette RenderPalette) color.RGBA {
	ch, idx := chunkCell(m, x, y)
	base := ch.BaseCell(idx)
	water := ch.WaterCell(idx)
	cover := ch.CoverCell(idx)
	height := m.heightAt(x, y)

	if water.Kind() != world.WaterNone {
		if water.Kind() == world.WaterRiver {
			baseColor := palette.Water.River
			if colorNoise.noise2D(float64(x)*0.2, float64(y)*0.2) > 0.55 {
				baseColor = palette.Water.RiverVariant
			}
			return shade(baseColor, int(math.Round(float64(water.Level()-1)*4)))
		}
		if water.Tidal() {
			baseColor := palette.Water.TidalSea
			if water.Kind() == world.WaterLake {
				baseColor = palette.Water.TidalLake
			}
			return shade(baseColor, int(math.Round(float64(water.Level())*-1.5+height*12)))
		}
		if water.Kind() == world.WaterLake {
			return shade(palette.Water.Lake, int(math.Round(height*18)))
		}
		return shade(palette.Water.Sea, int(math.Round(height*22)))
	}

	if cover.Flags()&coverFlagMountain != 0 {
		n := colorNoise.octaveNoise2D(float64(x)*0.13+11, float64(y)*0.13+19, 3, 0.5)
		baseColor := palette.Terrain.MountainLight
		if n <= 0.58 {
			baseColor = palette.Terrain.MountainDark
		}
		return shade(baseColor, int(math.Round((n-0.5)*22)))
	}
	if cover.Flags()&coverFlagRock != 0 || base.Soil() == world.SoilRocky {
		n := colorNoise.octaveNoise2D(float64(x)*0.13+11, float64(y)*0.13+19, 3, 0.5)
		if hasWaterNeighbor(m, x, y) {
			return shade(palette.Terrain.CoastalRock, int(math.Round((n-0.5)*18)))
		}
		return shade(palette.Terrain.Rock, int(math.Round((n-0.5)*20)))
	}

	switch cover.Kind() {
	case world.CoverBirchForest:
		return shade(palette.Cover.BirchForest, int(cover.Level())*2+int(math.Round(coverDensity(ch.Stock[idx])*12)))
	case world.CoverPineForest:
		return shade(palette.Cover.PineForest, int(cover.Level())*2+int(math.Round(coverDensity(ch.Stock[idx])*12)))
	case world.CoverMixedForest:
		return shade(palette.Cover.MixedForest, int(cover.Level())*2+int(math.Round(coverDensity(ch.Stock[idx])*12)))
	case world.CoverDryBush:
		return shade(palette.Cover.DryBush, int(math.Round(coverDensity(ch.Stock[idx])*10)))
	case world.CoverBush:
		return shade(palette.Cover.Bush, int(cover.Level()))
	}

	colors := biomeColors(palette, base.Biome())
	n := colorNoise.octaveNoise2D(float64(x)*0.09, float64(y)*0.09, 3, 0.5)
	colorIndex := min(len(colors)-1, int(math.Floor(n*float64(len(colors)))))
	return shade(colors[colorIndex], int(math.Round((n-0.5)*18)))
}

func coverDensity(stock uint16) float64 {
	return clamp((float64(stock)-6)/18, 0.28, 1)
}

func hexColor(hex string) color.RGBA {
	if len(hex) != 7 || hex[0] != '#' {
		return color.RGBA{A: 255}
	}
	return color.RGBA{
		R: hexByte(hex[1], hex[2]),
		G: hexByte(hex[3], hex[4]),
		B: hexByte(hex[5], hex[6]),
		A: 255,
	}
}

func hexByte(a, b byte) uint8 {
	return hexNibble(a)<<4 | hexNibble(b)
}

func hexNibble(b byte) uint8 {
	switch {
	case b >= '0' && b <= '9':
		return b - '0'
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10
	default:
		return 0
	}
}

func shade(c color.RGBA, amount int) color.RGBA {
	return color.RGBA{
		R: clampByte(int(c.R) + amount),
		G: clampByte(int(c.G) + amount),
		B: clampByte(int(c.B) + amount),
		A: c.A,
	}
}

func clampByte(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func fillPixel(img *image.RGBA, x, y, size int, c color.RGBA) {
	for yy := y; yy < y+size; yy++ {
		for xx := x; xx < x+size; xx++ {
			img.SetRGBA(xx, yy, c)
		}
	}
}
