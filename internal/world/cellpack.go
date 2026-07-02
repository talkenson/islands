package world

const (
	baseBiomeBits     = 5
	baseSoilBits      = 4
	baseElevationBits = 5
	baseFlagsBits     = 2

	baseBiomeMask     = (1 << baseBiomeBits) - 1
	baseSoilMask      = (1 << baseSoilBits) - 1
	baseElevationMask = (1 << baseElevationBits) - 1
	baseFlagsMask     = (1 << baseFlagsBits) - 1

	waterKindBits  = 4
	waterLevelBits = 3

	waterKindMask  = (1 << waterKindBits) - 1
	waterLevelMask = (1 << waterLevelBits) - 1

	coverKindBits  = 8
	coverLevelBits = 4
	coverFlagsBits = 4

	coverKindMask  = (1 << coverKindBits) - 1
	coverLevelMask = (1 << coverLevelBits) - 1
	coverFlagsMask = (1 << coverFlagsBits) - 1
)

func PackBase(biome Biome, soil Soil, elevation uint8, flags uint8) BaseCell {
	return BaseCell(
		(uint16(biome) & baseBiomeMask) |
			((uint16(soil) & baseSoilMask) << baseBiomeBits) |
			((uint16(elevation) & baseElevationMask) << (baseBiomeBits + baseSoilBits)) |
			((uint16(flags) & baseFlagsMask) << (baseBiomeBits + baseSoilBits + baseElevationBits)),
	)
}

func (c BaseCell) Biome() Biome {
	return Biome(uint16(c) & baseBiomeMask)
}

func (c BaseCell) Soil() Soil {
	return Soil((uint16(c) >> baseBiomeBits) & baseSoilMask)
}

func (c BaseCell) Elevation() uint8 {
	return uint8((uint16(c) >> (baseBiomeBits + baseSoilBits)) & baseElevationMask)
}

func (c BaseCell) Flags() uint8 {
	return uint8((uint16(c) >> (baseBiomeBits + baseSoilBits + baseElevationBits)) & baseFlagsMask)
}

func PackWater(kind WaterKind, level uint8, tidal bool) WaterCell {
	cell := uint8(kind)&waterKindMask | ((level & waterLevelMask) << waterKindBits)
	if tidal {
		cell |= 1 << 7
	}
	return WaterCell(cell)
}

func (c WaterCell) Kind() WaterKind {
	return WaterKind(uint8(c) & waterKindMask)
}

func (c WaterCell) Level() uint8 {
	return (uint8(c) >> waterKindBits) & waterLevelMask
}

func (c WaterCell) Tidal() bool {
	return uint8(c)&(1<<7) != 0
}

func PackCover(kind CoverKind, level uint8, flags uint8) CoverCell {
	return CoverCell(
		(uint16(kind) & coverKindMask) |
			((uint16(level) & coverLevelMask) << coverKindBits) |
			((uint16(flags) & coverFlagsMask) << (coverKindBits + coverLevelBits)),
	)
}

func (c CoverCell) Kind() CoverKind {
	return CoverKind(uint16(c) & coverKindMask)
}

func (c CoverCell) Level() uint8 {
	return uint8((uint16(c) >> coverKindBits) & coverLevelMask)
}

func (c CoverCell) Flags() uint8 {
	return uint8((uint16(c) >> (coverKindBits + coverLevelBits)) & coverFlagsMask)
}
