package game

import "islands/internal/world"

const (
	MovementBlockedCostMS uint64 = 0
	SurfaceFlagOpen       uint8  = 1 << 0
)

var surfaceMoveCostMS = map[world.SurfaceKind]uint64{
	world.SurfaceTrail:     300,
	world.SurfaceDirtRoad:  220,
	world.SurfaceStoneRoad: 160,
	world.SurfaceBridge:    240,
	world.SurfacePier:      260,
}

var coverMoveCostMS = map[world.CoverKind]uint64{
	world.CoverGrass:       800,
	world.CoverBush:        1100,
	world.CoverDryBush:     1000,
	world.CoverBirchForest: 1500,
	world.CoverPineForest:  1600,
	world.CoverMixedForest: 1550,
	world.CoverReeds:       1800,
	world.CoverField:       900,
}

var soilMoveCostMS = map[world.Soil]uint64{
	world.SoilSilt:      950,
	world.SoilSand:      1000,
	world.SoilBare:      850,
	world.SoilGrass:     800,
	world.SoilFertile:   800,
	world.SoilExhausted: 900,
	world.SoilRocky:     1200,
	world.SoilMarsh:     2400,
}

const defaultMoveCostMS uint64 = 800

func movementCostMS(ch *world.Chunk, index uint16) (uint64, bool) {
	surface := ch.SurfaceCell(index)
	if surface.Kind() != world.SurfaceNone {
		return surfaceMovementCostMS(surface)
	}

	cover := ch.CoverCell(index)
	if cover.Kind() != world.CoverNone {
		if cost, ok := coverMoveCostMS[cover.Kind()]; ok {
			return cost, true
		}
	}

	base := ch.BaseCell(index)
	if cost, ok := soilMoveCostMS[base.Soil()]; ok {
		return cost, true
	}
	return defaultMoveCostMS, true
}

func surfaceMovementCostMS(surface world.SurfaceCell) (uint64, bool) {
	switch surface.Kind() {
	case world.SurfaceFence:
		return MovementBlockedCostMS, false
	case world.SurfaceGate:
		if surface.Flags()&SurfaceFlagOpen == 0 {
			return MovementBlockedCostMS, false
		}
		return 350, true
	default:
		cost, ok := surfaceMoveCostMS[surface.Kind()]
		return cost, ok
	}
}
