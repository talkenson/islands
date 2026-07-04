package game

import (
	"context"

	"islands/internal/realtime"
	"islands/internal/storage"
	"islands/internal/world"
)

const (
	TreeStageSapling uint8 = 1
	TreeStageYoung   uint8 = 2
	TreeStageMature  uint8 = 3
	TreeStageOld     uint8 = 4
)

func (s *Service) AdvanceForestGrowth(ctx context.Context, worldID uint64) (int, error) {
	s.mu.Lock()
	active := s.activeChunksLocked(worldID)
	if len(active) == 0 {
		s.mu.Unlock()
		return 0, nil
	}

	previousTick := s.tick
	nextTick := s.tick + 1
	changed := make(map[world.ChunkCoord]*world.Chunk)
	previous := make(map[world.ChunkCoord]*world.Chunk)

	for coord := range active {
		ch := s.chunkLocked(worldID, coord)
		if ch == nil {
			continue
		}
		for i := 0; i < world.ChunkCells; i++ {
			index := uint16(i)
			if s.growTreeCellLocked(worldID, coord, index, nextTick, changed, previous) {
				continue
			}
			s.spreadTreeLocked(worldID, active, coord, index, nextTick, changed, previous)
		}
	}

	if len(changed) == 0 {
		s.tick = nextTick
		s.mu.Unlock()
		return 0, nil
	}

	s.tick = nextTick
	store := s.store
	if store != nil {
		dirty := make([]storage.DirtyChunk, 0, len(changed))
		for _, ch := range changed {
			dirty = append(dirty, storage.DirtyChunk{Chunk: ch, Tick: s.tick})
		}
		if err := saveForestDirtyChunks(ctx, store, dirty); err != nil {
			for coord, ch := range previous {
				s.chunks[worldID][coord] = ch
			}
			s.tick = previousTick
			s.mu.Unlock()
			return 0, err
		}
	}

	events := make([]realtime.Event, 0, len(changed))
	for coord, ch := range changed {
		eventID := s.nextEventIDLocked()
		events = append(events, realtime.Event{
			ID:            eventID,
			Type:          "chunk_snapshot",
			WorldID:       worldID,
			ChangedChunks: []world.ChunkCoord{coord},
			Data:          snapshotChunk(ch, s.tick),
		})
	}
	s.mu.Unlock()

	for _, event := range events {
		s.hub.Publish(event)
	}
	return len(events), nil
}

func (s *Service) activeChunksLocked(worldID uint64) map[world.ChunkCoord]struct{} {
	active := make(map[world.ChunkCoord]struct{})
	for _, act := range s.actors {
		if act.WorldID != worldID {
			continue
		}
		center, _ := world.ToChunkCoord(act.X, act.Y)
		for coord := range realtime.VisibleChunks(center, s.config.VisibleChunkRadius) {
			if s.chunkLocked(worldID, coord) != nil {
				active[coord] = struct{}{}
			}
		}
	}
	return active
}

func (s *Service) growTreeCellLocked(worldID uint64, coord world.ChunkCoord, index uint16, tick uint64, changed, previous map[world.ChunkCoord]*world.Chunk) bool {
	ch := s.chunkLocked(worldID, coord)
	if ch == nil {
		return false
	}
	cover := ch.CoverCell(index)
	if !isTreeCover(cover.Kind()) {
		return false
	}
	stage := treeStage(cover)
	if stage < TreeStageOld && growthRoll(worldID, coord, index, tick, 31) {
		stage++
		rememberChunk(changed, previous, coord, ch)
		ch.SetCover(index, world.PackCover(cover.Kind(), stage, cover.Flags()))
		ch.SetStock(index, uint16(treeWoodYield(stage)))
		return true
	}
	return false
}

func (s *Service) spreadTreeLocked(worldID uint64, active map[world.ChunkCoord]struct{}, coord world.ChunkCoord, index uint16, tick uint64, changed, previous map[world.ChunkCoord]*world.Chunk) {
	ch := s.chunkLocked(worldID, coord)
	if ch == nil {
		return
	}
	cover := ch.CoverCell(index)
	if !isTreeCover(cover.Kind()) || treeStage(cover) < TreeStageMature {
		return
	}
	if !growthRoll(worldID, coord, index, tick, 127) {
		return
	}

	lx, ly := world.LocalXY(index)
	direction := int(hashGrowth(worldID, coord, index, tick, 17) % 4)
	dx := int32(0)
	dy := int32(0)
	switch direction {
	case 0:
		dx = 1
	case 1:
		dx = -1
	case 2:
		dy = 1
	default:
		dy = -1
	}
	wx := coord.X*world.ChunkSize + int32(lx) + dx
	wy := coord.Y*world.ChunkSize + int32(ly) + dy
	targetCoord, targetIndex := world.ToChunkCoord(wx, wy)
	if _, ok := active[targetCoord]; !ok {
		return
	}
	target := s.chunkLocked(worldID, targetCoord)
	if target == nil || !canPlantTree(target, targetIndex) {
		return
	}
	rememberChunk(changed, previous, targetCoord, target)
	target.SetCover(targetIndex, world.PackCover(seedlingKind(target.BaseCell(targetIndex).Biome(), cover.Kind()), TreeStageSapling, 0))
	target.SetStock(targetIndex, 0)
}

func rememberChunk(changed, previous map[world.ChunkCoord]*world.Chunk, coord world.ChunkCoord, ch *world.Chunk) {
	if _, ok := changed[coord]; ok {
		return
	}
	previous[coord] = copyChunk(ch)
	changed[coord] = ch
}

func copyChunk(ch *world.Chunk) *world.Chunk {
	next := world.NewChunk(ch.X, ch.Y)
	copy(next.Base, ch.Base)
	copy(next.Water, ch.Water)
	copy(next.Cover, ch.Cover)
	copy(next.Stock, ch.Stock)
	copy(next.Meta, ch.Meta)
	copy(next.Temperature, ch.Temperature)
	next.Dirty = ch.Dirty
	return next
}

func isTreeCover(kind world.CoverKind) bool {
	return kind == world.CoverBirchForest || kind == world.CoverPineForest || kind == world.CoverMixedForest
}

func treeStage(cover world.CoverCell) uint8 {
	level := cover.Level()
	if level < TreeStageSapling {
		return TreeStageSapling
	}
	if level > TreeStageOld {
		return TreeStageOld
	}
	return level
}

func treeWoodYield(stage uint8) uint32 {
	switch stage {
	case TreeStageSapling:
		return 0
	case TreeStageYoung:
		return 1
	case TreeStageMature:
		return 7
	default:
		return 11
	}
}

func treeSaplingYield(worldID uint64, coord world.ChunkCoord, index uint16, tick uint64) uint32 {
	roll := hashGrowth(worldID, coord, index, tick, 91) % 100
	switch {
	case roll < 45:
		return 0
	case roll < 80:
		return 1
	case roll < 95:
		return 2
	default:
		return 3
	}
}

func growthRoll(worldID uint64, coord world.ChunkCoord, index uint16, tick uint64, mask uint64) bool {
	return hashGrowth(worldID, coord, index, tick, 0)&mask == 0
}

func hashGrowth(worldID uint64, coord world.ChunkCoord, index uint16, tick uint64, salt uint64) uint64 {
	x := worldID*0x9e3779b185ebca87 ^ tick*0xc2b2ae3d27d4eb4f ^ salt
	x ^= uint64(uint32(coord.X)) * 0x165667b19e3779f9
	x ^= uint64(uint32(coord.Y)) * 0x85ebca77c2b2ae63
	x ^= uint64(index) * 0x27d4eb2f165667c5
	x ^= x >> 33
	x *= 0xff51afd7ed558ccd
	x ^= x >> 33
	x *= 0xc4ceb9fe1a85ec53
	x ^= x >> 33
	return x
}

func canPlantTree(ch *world.Chunk, index uint16) bool {
	if ch.WaterCell(index).Kind() != world.WaterNone {
		return false
	}
	switch ch.BaseCell(index).Soil() {
	case world.SoilWater, world.SoilSand, world.SoilRocky, world.SoilMarsh:
		return false
	}
	switch ch.CoverCell(index).Kind() {
	case world.CoverNone, world.CoverGrass, world.CoverBush:
		return true
	default:
		return false
	}
}

func seedlingKind(biome world.Biome, fallback world.CoverKind) world.CoverKind {
	switch biome {
	case world.BiomeTaiga:
		return world.CoverPineForest
	case world.BiomeTemperateForest:
		return world.CoverMixedForest
	case world.BiomeBirchForest, world.BiomeMeadow, world.BiomeRiverValley:
		return world.CoverBirchForest
	default:
		if isTreeCover(fallback) {
			return fallback
		}
		return world.CoverBirchForest
	}
}

func saveForestDirtyChunks(ctx context.Context, store storage.Store, chunks []storage.DirtyChunk) error {
	if batch, ok := store.(storage.DirtyChunkBatchWriter); ok {
		return batch.SaveDirtyChunks(ctx, chunks)
	}
	for _, dirty := range chunks {
		if err := store.SaveDirtyChunk(ctx, dirty.Chunk, dirty.Tick); err != nil {
			return err
		}
	}
	return nil
}
