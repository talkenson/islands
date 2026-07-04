package world

type Biome uint8

const (
	BiomeUnknown Biome = iota
	BiomeTaiga
	BiomeBirchForest
	BiomeTemperateForest
	BiomeRiverValley
	BiomeCoast
	BiomeMarsh
	BiomeSteppe
	BiomeMountain
	BiomeDesert
	BiomeMeadow
)

type Soil uint8

const (
	SoilUnknown Soil = iota
	SoilWater
	SoilSilt
	SoilSand
	SoilBare
	SoilGrass
	SoilFertile
	SoilExhausted
	SoilRocky
	SoilMarsh
)

type WaterKind uint8

const (
	WaterNone WaterKind = iota
	WaterSea
	WaterRiver
	WaterLake
	WaterCanal
	WaterSwamp
)

type CoverKind uint8

const (
	CoverNone CoverKind = iota
	CoverGrass
	CoverBush
	CoverDryBush
	CoverBirchForest
	CoverPineForest
	CoverMixedForest
	CoverReeds
	CoverField
)

type SurfaceKind uint8

const (
	SurfaceNone SurfaceKind = iota
	SurfaceTrail
	SurfaceDirtRoad
	SurfaceStoneRoad
	SurfaceFence
	SurfaceGate
	SurfaceBridge
	SurfacePier
)

type BaseCell uint16
type WaterCell uint8
type CoverCell uint16
type SurfaceCell uint16
