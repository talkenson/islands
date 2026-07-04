package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"islands/internal/game"
)

func TestLoadServerConfigProperties(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.conf")
	data := []byte("addr=:9090\nworld-day-length=12m\nforest-growth-per-day=1\nstorage-batch-interval=250ms\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write properties: %v", err)
	}

	cfg, loaded, err := loadServerConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !loaded {
		t.Fatalf("properties file was not loaded")
	}
	if cfg.Addr != ":9090" {
		t.Fatalf("addr: got %q", cfg.Addr)
	}
	if cfg.WorldDayLength != 12*time.Minute {
		t.Fatalf("day length: got %s", cfg.WorldDayLength)
	}
	if cfg.ForestGrowthPerDay != 1 {
		t.Fatalf("forest growth per day: got %d", cfg.ForestGrowthPerDay)
	}
	if cfg.StorageBatchInterval != 250*time.Millisecond {
		t.Fatalf("storage batch interval: got %s", cfg.StorageBatchInterval)
	}
}

func TestServerConfigPathSupportsConfigAndShorthand(t *testing.T) {
	if got := serverConfigPath([]string{"-config", "custom.conf"}); got != "custom.conf" {
		t.Fatalf("-config path: got %q", got)
	}
	if got := serverConfigPath([]string{"-config=custom.conf"}); got != "custom.conf" {
		t.Fatalf("-config= path: got %q", got)
	}
	if got := serverConfigPath([]string{"-c", "short.conf"}); got != "short.conf" {
		t.Fatalf("-c path: got %q", got)
	}
	if got := serverConfigPath([]string{"-c=short.conf"}); got != "short.conf" {
		t.Fatalf("-c= path: got %q", got)
	}
}

func TestGrowthSlotFollowsInGameDay(t *testing.T) {
	cfg := game.ClockConfig{DayLengthSeconds: 480, SecondsPerTick: 1}

	if got := growthSlot(game.BuildWorldTime(0, cfg), 2); got != 0 {
		t.Fatalf("start slot: got %d, want 0", got)
	}
	if got := growthSlot(game.BuildWorldTime(239, cfg), 2); got != 0 {
		t.Fatalf("before half-day slot: got %d, want 0", got)
	}
	if got := growthSlot(game.BuildWorldTime(240, cfg), 2); got != 1 {
		t.Fatalf("half-day slot: got %d, want 1", got)
	}
	if got := growthSlot(game.BuildWorldTime(480, cfg), 2); got != 2 {
		t.Fatalf("next-day slot: got %d, want 2", got)
	}
}
