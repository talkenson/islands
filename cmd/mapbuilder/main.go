package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"islands/internal/mapgen"
)

func main() {
	config := mapgen.DefaultConfig()
	var outDir string
	var pixelSize int

	flag.StringVar(&config.Seed, "seed", config.Seed, "generation seed")
	flag.IntVar(&config.Width, "width", config.Width, "map width in cells")
	flag.IntVar(&config.Height, "height", config.Height, "map height in cells")
	flag.IntVar(&config.ContinentCount, "continents", config.ContinentCount, "continent count")
	flag.IntVar(&config.RiverCount, "rivers", config.RiverCount, "target river count")
	flag.IntVar(&config.MinRiverLength, "min-river-length", config.MinRiverLength, "minimum accepted river length")
	flag.IntVar(&pixelSize, "pixel-size", 3, "rendered PNG pixel size")
	flag.StringVar(&outDir, "out", "artifacts/generated", "output directory")
	flag.Parse()

	m, err := mapgen.Generate(config)
	if err != nil {
		fatal(err)
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fatal(err)
	}

	binPath := filepath.Join(outDir, "world.islmap")
	binFile, err := os.Create(binPath)
	if err != nil {
		fatal(err)
	}
	if err := mapgen.SaveBinary(binFile, m); err != nil {
		_ = binFile.Close()
		fatal(err)
	}
	if err := binFile.Close(); err != nil {
		fatal(err)
	}

	pngPath := filepath.Join(outDir, "world.png")
	pngFile, err := os.Create(pngPath)
	if err != nil {
		fatal(err)
	}
	if err := mapgen.RenderPNG(pngFile, m, pixelSize); err != nil {
		_ = pngFile.Close()
		fatal(err)
	}
	if err := pngFile.Close(); err != nil {
		fatal(err)
	}

	fmt.Printf("generated %dx%d world with %d chunks\n", m.Width, m.Height, len(m.Chunks))
	fmt.Printf("land=%d water=%d shallow=%d river=%d forest=%d dry_bush=%d rock=%d mountain=%d wood=%d stone=%d\n",
		m.Stats.Land,
		m.Stats.Water,
		m.Stats.Shallow,
		m.Stats.River,
		m.Stats.Forest,
		m.Stats.DryBush,
		m.Stats.Rock,
		m.Stats.Mountain,
		m.Stats.WoodStock,
		m.Stats.StoneStock,
	)
	fmt.Printf("saved map: %s\n", binPath)
	fmt.Printf("saved render: %s\n", pngPath)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
