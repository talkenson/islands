package game

import (
	"testing"

	"islands/internal/world"
)

func TestMovementCostUsesTargetSurfaceBeforeCover(t *testing.T) {
	ch := world.NewChunk(0, 0)
	ch.Cover[0] = uint16(world.PackCover(world.CoverPineForest, 4, 0))
	ch.Surface[0] = uint16(world.PackSurface(world.SurfaceTrail, 1, 0))

	got, passable := movementCostMS(ch, 0)

	if !passable {
		t.Fatalf("trail should be passable")
	}
	if got != surfaceMoveCostMS[world.SurfaceTrail] {
		t.Fatalf("cost: got %d, want trail cost %d", got, surfaceMoveCostMS[world.SurfaceTrail])
	}
}

func TestMovementCostFallsBackToCover(t *testing.T) {
	ch := world.NewChunk(0, 0)
	ch.Cover[0] = uint16(world.PackCover(world.CoverBirchForest, 4, 0))

	got, passable := movementCostMS(ch, 0)

	if !passable {
		t.Fatalf("forest should be passable")
	}
	if got <= coverMoveCostMS[world.CoverGrass] {
		t.Fatalf("forest cost: got %d, want more than grass %d", got, coverMoveCostMS[world.CoverGrass])
	}
}

func TestMovementCostFallsBackToSoil(t *testing.T) {
	ch := world.NewChunk(0, 0)
	ch.Base[0] = uint16(world.PackBase(world.BiomeDesert, world.SoilSand, 4, 0))
	ch.Base[1] = uint16(world.PackBase(world.BiomeMarsh, world.SoilMarsh, 2, 0))

	sand, sandPassable := movementCostMS(ch, 0)
	marsh, marshPassable := movementCostMS(ch, 1)

	if !sandPassable || !marshPassable {
		t.Fatalf("sand and marsh should be passable")
	}
	if sand <= coverMoveCostMS[world.CoverGrass] {
		t.Fatalf("sand cost: got %d, want more than grass %d", sand, coverMoveCostMS[world.CoverGrass])
	}
	if marsh <= sand {
		t.Fatalf("marsh cost: got %d, want more than sand %d", marsh, sand)
	}
}

func TestMovementCostBlocksFenceAndClosedGate(t *testing.T) {
	ch := world.NewChunk(0, 0)
	ch.Surface[0] = uint16(world.PackSurface(world.SurfaceFence, 1, 0))
	ch.Surface[1] = uint16(world.PackSurface(world.SurfaceGate, 1, 0))
	ch.Surface[2] = uint16(world.PackSurface(world.SurfaceGate, 1, SurfaceFlagOpen))

	if _, passable := movementCostMS(ch, 0); passable {
		t.Fatalf("fence should block movement")
	}
	if _, passable := movementCostMS(ch, 1); passable {
		t.Fatalf("closed gate should block movement")
	}
	if cost, passable := movementCostMS(ch, 2); !passable || cost == 0 {
		t.Fatalf("open gate should be passable, got cost=%d passable=%v", cost, passable)
	}
}
