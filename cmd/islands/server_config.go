package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"islands/internal/game"
)

const defaultServerConfigPath = "server.conf"

type serverConfig struct {
	Addr                  string
	AuthSecret            string
	VisibleChunkRadius    int
	WorldMap              string
	WorldJournal          string
	WorldPlayers          string
	CompactWorldInterval  time.Duration
	StorageBatchInterval  time.Duration
	StorageBatchMaxChunks int
	WorldDayLength        time.Duration
	WorldTimeTickInterval time.Duration
	WorldSecondsPerTick   uint64
	ForestGrowthPerDay    int
	ConfigPath            string
}

func defaultServerConfig() serverConfig {
	return serverConfig{
		Addr:                  ":8080",
		AuthSecret:            "dev-secret-change-me",
		VisibleChunkRadius:    2,
		StorageBatchInterval:  time.Second,
		StorageBatchMaxChunks: 128,
		WorldDayLength:        8 * time.Minute,
		WorldTimeTickInterval: time.Second,
		WorldSecondsPerTick:   game.DefaultWorldSecondsPerTick,
		ForestGrowthPerDay:    2,
		ConfigPath:            defaultServerConfigPath,
	}
}

func serverConfigPath(args []string) string {
	for i, arg := range args {
		if (arg == "-config" || arg == "-c") && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(arg, "-config=") {
			return strings.TrimPrefix(arg, "-config=")
		}
		if strings.HasPrefix(arg, "-c=") {
			return strings.TrimPrefix(arg, "-c=")
		}
	}
	return defaultServerConfigPath
}

func loadServerConfig(path string) (serverConfig, bool, error) {
	cfg := defaultServerConfig()
	cfg.ConfigPath = path
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, false, nil
		}
		return cfg, false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return cfg, true, fmt.Errorf("%s:%d: expected key=value", path, lineNo)
		}
		if err := cfg.set(strings.TrimSpace(key), strings.TrimSpace(value)); err != nil {
			return cfg, true, fmt.Errorf("%s:%d: %w", path, lineNo, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return cfg, true, err
	}
	return cfg, true, nil
}

func (c *serverConfig) set(key, value string) error {
	switch key {
	case "addr":
		c.Addr = value
	case "auth-secret":
		c.AuthSecret = value
	case "visible-chunk-radius":
		v, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		c.VisibleChunkRadius = v
	case "world-map":
		c.WorldMap = value
	case "world-journal":
		c.WorldJournal = value
	case "world-players":
		c.WorldPlayers = value
	case "compact-world-interval":
		v, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		c.CompactWorldInterval = v
	case "storage-batch-interval":
		v, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		c.StorageBatchInterval = v
	case "storage-batch-max-chunks":
		v, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		c.StorageBatchMaxChunks = v
	case "world-day-length":
		v, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		c.WorldDayLength = v
	case "world-time-tick-interval":
		v, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		c.WorldTimeTickInterval = v
	case "world-seconds-per-tick":
		v, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return err
		}
		c.WorldSecondsPerTick = v
	case "forest-growth-per-day":
		v, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		c.ForestGrowthPerDay = v
	default:
		return fmt.Errorf("unknown property %q", key)
	}
	return nil
}
