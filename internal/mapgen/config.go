package mapgen

type Config struct {
	Seed string

	Width   int
	Height  int
	Workers int

	ContinentCount              int
	OceanMargin                 float64
	ContinentSpacing            float64
	ContinentPlacementAttempts  int
	ContinentSeparationStrength float64
	ContinentRoughness          float64
	CoastDetailScale            float64
	LandThreshold               float64

	HeightScale        float64
	MoistureScale      float64
	TemperatureScale   float64
	ForestClusterScale float64

	LakeScale                 float64
	LakeThreshold             float64
	LakeDepth                 float64
	LakeMinContinentInfluence float64
	LakeBasinsPerContinentMin int
	LakeBasinsPerContinentMax int
	LakeBasinMinRadius        float64
	LakeBasinMaxRadius        float64

	ShallowWaterMinWidth    int
	ShallowWaterMaxWidthMin int
	ShallowWaterMaxWidthMax int
	ShallowWaterScale       float64

	RiverCount           int
	MinRiverLength       int
	RiverMeanderScale    float64
	RiverMeanderStrength float64
	RiverMinWidth        int
	RiverMaxWidth        int

	RockScale         float64
	RockThreshold     float64
	RockyBeachChance  float64
	MountainHeight    float64
	MountainScale     float64
	MountainThreshold float64
}

func DefaultConfig() Config {
	return Config{
		Seed: "talkenson",

		Width:   4096,
		Height:  4096,
		Workers: 16,

		ContinentCount:              5,
		OceanMargin:                 0.12,
		ContinentSpacing:            0.1,
		ContinentPlacementAttempts:  90,
		ContinentSeparationStrength: 0.34,
		ContinentRoughness:          0.22,
		CoastDetailScale:            0.035,
		LandThreshold:               0.42,

		HeightScale:        0.01,
		MoistureScale:      0.015,
		TemperatureScale:   0.01,
		ForestClusterScale: 0.04,

		LakeScale:                 0.014,
		LakeThreshold:             0.5,
		LakeDepth:                 0.68,
		LakeMinContinentInfluence: 0.46,
		LakeBasinsPerContinentMin: 1,
		LakeBasinsPerContinentMax: 3,
		LakeBasinMinRadius:        0.045,
		LakeBasinMaxRadius:        0.09,

		ShallowWaterMinWidth:    0,
		ShallowWaterMaxWidthMin: 2,
		ShallowWaterMaxWidthMax: 5,
		ShallowWaterScale:       0.045,

		RiverCount:           200,
		MinRiverLength:       60,
		RiverMeanderScale:    0.055,
		RiverMeanderStrength: 0.18,
		RiverMinWidth:        1,
		RiverMaxWidth:        6,

		RockScale:         0.035,
		RockThreshold:     0.8,
		RockyBeachChance:  0.38,
		MountainHeight:    0.72,
		MountainScale:     0.018,
		MountainThreshold: 0.57,
	}
}
