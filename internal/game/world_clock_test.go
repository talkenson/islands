package game

import "testing"

func TestBuildWorldTimeEightPhases(t *testing.T) {
	cfg := ClockConfig{DayLengthSeconds: 480, SecondsPerTick: 1}
	tests := []struct {
		worldTime     uint64
		day           uint64
		phase         DayPhase
		phaseProgress float64
	}{
		{worldTime: 0, day: 1, phase: PhaseLateNight, phaseProgress: 0},
		{worldTime: 60, day: 1, phase: PhaseDawn, phaseProgress: 0},
		{worldTime: 120, day: 1, phase: PhaseMorning, phaseProgress: 0},
		{worldTime: 180, day: 1, phase: PhaseDay, phaseProgress: 0},
		{worldTime: 240, day: 1, phase: PhaseAfternoon, phaseProgress: 0},
		{worldTime: 300, day: 1, phase: PhaseDusk, phaseProgress: 0},
		{worldTime: 360, day: 1, phase: PhaseEvening, phaseProgress: 0},
		{worldTime: 420, day: 1, phase: PhaseNight, phaseProgress: 0},
		{worldTime: 480, day: 2, phase: PhaseLateNight, phaseProgress: 0},
		{worldTime: 510, day: 2, phase: PhaseLateNight, phaseProgress: 0.5},
	}

	for _, test := range tests {
		got := BuildWorldTime(test.worldTime, cfg)
		if got.Day != test.day || got.Phase != test.phase || got.PhaseProgress != test.phaseProgress {
			t.Fatalf("world time %d: got day=%d phase=%s progress=%v, want day=%d phase=%s progress=%v", test.worldTime, got.Day, got.Phase, got.PhaseProgress, test.day, test.phase, test.phaseProgress)
		}
	}
}

func TestClockConfigNormalize(t *testing.T) {
	got := ClockConfig{}.Normalize()
	if got.DayLengthSeconds != DefaultWorldDayLengthSeconds {
		t.Fatalf("day length: got %d, want %d", got.DayLengthSeconds, DefaultWorldDayLengthSeconds)
	}
	if got.SecondsPerTick != DefaultWorldSecondsPerTick {
		t.Fatalf("seconds per tick: got %d, want %d", got.SecondsPerTick, DefaultWorldSecondsPerTick)
	}
}
