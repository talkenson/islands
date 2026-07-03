package game

const (
	DefaultWorldDayLengthSeconds uint64 = 480
	DefaultWorldSecondsPerTick   uint64 = 1
)

type DayPhase string

const (
	PhaseLateNight DayPhase = "late_night"
	PhaseDawn      DayPhase = "dawn"
	PhaseMorning   DayPhase = "morning"
	PhaseDay       DayPhase = "day"
	PhaseAfternoon DayPhase = "afternoon"
	PhaseDusk      DayPhase = "dusk"
	PhaseEvening   DayPhase = "evening"
	PhaseNight     DayPhase = "night"
)

var dayPhases = [...]DayPhase{
	PhaseLateNight,
	PhaseDawn,
	PhaseMorning,
	PhaseDay,
	PhaseAfternoon,
	PhaseDusk,
	PhaseEvening,
	PhaseNight,
}

type ClockConfig struct {
	DayLengthSeconds          uint64
	SecondsPerTick            uint64
	WorldSecondsPerRealSecond float64
}

func (c ClockConfig) Normalize() ClockConfig {
	if c.DayLengthSeconds == 0 {
		c.DayLengthSeconds = DefaultWorldDayLengthSeconds
	}
	if c.SecondsPerTick == 0 {
		c.SecondsPerTick = DefaultWorldSecondsPerTick
	}
	if c.WorldSecondsPerRealSecond <= 0 {
		c.WorldSecondsPerRealSecond = 1
	}
	return c
}

type WorldTime struct {
	WorldTime                 uint64   `json:"world_time"`
	Day                       uint64   `json:"day"`
	Phase                     DayPhase `json:"phase"`
	PhaseProgress             float64  `json:"phase_progress"`
	DayProgress               float64  `json:"day_progress"`
	DayLengthSeconds          uint64   `json:"day_length_seconds"`
	WorldSecondsPerRealSecond float64  `json:"world_seconds_per_real_second"`
}

func BuildWorldTime(worldTime uint64, cfg ClockConfig) WorldTime {
	cfg = cfg.Normalize()
	dayLength := cfg.DayLengthSeconds

	dayOffset := worldTime % dayLength
	phaseCount := uint64(len(dayPhases))
	phaseIndex := int(dayOffset * phaseCount / dayLength)
	if phaseIndex >= len(dayPhases) {
		phaseIndex = len(dayPhases) - 1
	}
	phaseStart := uint64(phaseIndex) * dayLength / phaseCount
	phaseEnd := uint64(phaseIndex+1) * dayLength / phaseCount
	if phaseEnd <= phaseStart {
		phaseEnd = phaseStart + 1
	}
	phaseOffset := dayOffset - phaseStart
	phaseLength := phaseEnd - phaseStart

	return WorldTime{
		WorldTime:                 worldTime,
		Day:                       worldTime/dayLength + 1,
		Phase:                     dayPhases[phaseIndex],
		PhaseProgress:             float64(phaseOffset) / float64(phaseLength),
		DayProgress:               float64(dayOffset) / float64(dayLength),
		DayLengthSeconds:          dayLength,
		WorldSecondsPerRealSecond: cfg.WorldSecondsPerRealSecond,
	}
}
