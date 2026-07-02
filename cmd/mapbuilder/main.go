package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"islands/internal/mapgen"
)

func main() {
	start := time.Now()
	config := mapgen.DefaultConfig()
	var outDir string
	var pixelSize int
	var printTimings bool
	var exportMap bool

	flag.StringVar(&config.Seed, "seed", config.Seed, "generation seed")
	flag.IntVar(&config.Width, "width", config.Width, "map width in cells")
	flag.IntVar(&config.Height, "height", config.Height, "map height in cells")
	flag.IntVar(&config.Workers, "workers", config.Workers, "parallel map generation workers; 0 uses up to 8 logical CPUs")
	flag.IntVar(&config.ContinentCount, "continents", config.ContinentCount, "continent count")
	flag.IntVar(&config.RiverCount, "rivers", config.RiverCount, "target river count")
	flag.IntVar(&config.MinRiverLength, "min-river-length", config.MinRiverLength, "minimum accepted river length")
	flag.IntVar(&pixelSize, "pixel-size", 3, "rendered PNG pixel size")
	flag.StringVar(&outDir, "out", "artifacts/generated", "output directory")
	flag.BoolVar(&exportMap, "export-map", false, "write generated binary map data")
	flag.BoolVar(&printTimings, "timings", false, "print map generation stage timings")
	flag.Parse()
	if flag.NArg() > 0 {
		fatal(fmt.Errorf("unexpected argument %q; put flags before positional arguments or remove it", flag.Arg(0)))
	}

	generateStart := time.Now()
	m, report, err := mapgen.GenerateWithReport(config)
	if err != nil {
		fatal(err)
	}
	generateDuration := time.Since(generateStart)

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fatal(err)
	}

	binPath := filepath.Join(outDir, "world.islmap")
	var saveDuration time.Duration
	if exportMap {
		saveStart := time.Now()
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
		saveDuration = time.Since(saveStart)
	}

	renderStart := time.Now()
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
	renderDuration := time.Since(renderStart)

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
	if exportMap {
		fmt.Printf("saved map: %s\n", binPath)
	}
	fmt.Printf("saved render: %s\n", pngPath)
	if printTimings {
		fmt.Println("generation timings:")
		for _, stage := range report.Stages {
			fmt.Printf("  %-18s %s\n", stage.Name+":", stage.Duration.Round(time.Millisecond))
		}
		fmt.Println("builder timings:")
		fmt.Printf("  %-18s %s\n", "generate total:", generateDuration.Round(time.Millisecond))
		if exportMap {
			fmt.Printf("  %-18s %s\n", "save map:", saveDuration.Round(time.Millisecond))
		} else {
			fmt.Printf("  %-18s %s\n", "save map:", "skipped")
		}
		fmt.Printf("  %-18s %s\n", "render png:", renderDuration.Round(time.Millisecond))
		fmt.Printf("  %-18s %s\n", "total:", time.Since(start).Round(time.Millisecond))
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
