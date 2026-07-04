package game

import (
	"context"
	"errors"
	"sync"
	"time"

	"islands/internal/actor"
	"islands/internal/inventory"
	"islands/internal/realtime"
	"islands/internal/storage"
	"islands/internal/world"
)

var (
	ErrForbidden     = errors.New("forbidden")
	ErrInvalidAction = errors.New("invalid_action")
	ErrNotVisible    = errors.New("not_visible")
	ErrConflict      = errors.New("conflict")
)

const (
	DemoActorStartX int32 = 900
	DemoActorStartY int32 = 1900
)

type Service struct {
	mu           sync.Mutex
	actors       map[actor.ID]*actor.Actor
	inventories  map[inventory.ID]*inventory.Inventory
	stacks       map[inventory.ID]*inventory.StackSet
	chunks       map[uint64]map[world.ChunkCoord]*world.Chunk
	loadedWorlds map[uint64]bool
	renderSeeds  map[uint64]string
	pendingMoves map[actor.ID]*pendingMove
	shuttingDown bool
	tick         uint64
	worldTime    uint64
	clockConfig  ClockConfig
	nextID       uint64
	hub          *realtime.Hub
	config       realtime.Config
	store        storage.Store
}

type pendingMove struct {
	WorldID uint64
	ActorID actor.ID
	FromX   int32
	FromY   int32
	TargetX int32
	TargetY int32
	ReadyAt time.Time
	DelayMS uint64
	Timer   *time.Timer
}

func NewService(hub *realtime.Hub, config realtime.Config) *Service {
	if hub == nil {
		hub = realtime.NewHub()
	}
	return &Service{
		actors:       make(map[actor.ID]*actor.Actor),
		inventories:  make(map[inventory.ID]*inventory.Inventory),
		stacks:       make(map[inventory.ID]*inventory.StackSet),
		chunks:       make(map[uint64]map[world.ChunkCoord]*world.Chunk),
		loadedWorlds: make(map[uint64]bool),
		renderSeeds:  make(map[uint64]string),
		pendingMoves: make(map[actor.ID]*pendingMove),
		hub:          hub,
		config:       config.Normalize(),
		clockConfig:  ClockConfig{}.Normalize(),
	}
}

func (s *Service) SetStore(store storage.Store) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store = store
}

func (s *Service) SeedDemoWorld(worldID uint64) actor.Actor {
	s.mu.Lock()
	defer s.mu.Unlock()

	act := s.seedDemoActorLocked(worldID)
	coord, _ := world.ToChunkCoord(act.X, act.Y)
	ch := s.ensureChunkLocked(worldID, coord)
	_, index := world.ToChunkCoord(act.X, act.Y)
	ch.SetBase(index, world.PackBase(world.BiomeBirchForest, world.SoilGrass, 8, 0))
	ch.SetWater(index, world.PackWater(world.WaterNone, 0, false))
	ch.SetCover(index, world.PackCover(world.CoverBirchForest, TreeStageMature, 0))
	ch.SetStock(index, uint16(treeWoodYield(TreeStageMature)))
	s.renderSeeds[worldID] = "demo"
	return act
}

func (s *Service) SeedDemoActor(worldID uint64) actor.Actor {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seedDemoActorLocked(worldID)
}

func (s *Service) LoadWorld(worldID uint64, state storage.WorldState) error {
	if err := s.LoadChunks(worldID, state.Chunks); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadPlayersLocked(state.Players)
	s.renderSeeds[worldID] = state.Seed
	if state.Tick > s.tick {
		s.tick = state.Tick
	}
	s.worldTime = state.Players.WorldTime
	return nil
}

func (s *Service) SetClockConfig(config ClockConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clockConfig = config.Normalize()
}

func (s *Service) WorldTime() WorldTime {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.worldTimeLocked()
}

func (s *Service) AdvanceWorldTime(worldID uint64, seconds uint64) (WorldTime, bool) {
	if seconds == 0 {
		return s.WorldTime(), false
	}

	s.mu.Lock()
	previous := s.worldTimeLocked()
	s.worldTime += seconds
	next := s.worldTimeLocked()
	changed := previous.Phase != next.Phase
	var eventID uint64
	if changed {
		eventID = s.nextEventIDLocked()
	}
	s.mu.Unlock()

	if changed {
		s.hub.Publish(realtime.Event{
			ID:      eventID,
			Type:    "world_time",
			WorldID: worldID,
			Data:    next,
		})
	}
	return next, changed
}

func (s *Service) LoadChunks(worldID uint64, chunks map[world.ChunkCoord]*world.Chunk) error {
	if len(chunks) == 0 {
		return ErrConflict
	}

	loaded := make(map[world.ChunkCoord]*world.Chunk, len(chunks))
	for coord, ch := range chunks {
		if ch == nil {
			return ErrConflict
		}
		if err := ch.Validate(); err != nil {
			return err
		}
		copied := world.NewChunk(ch.X, ch.Y)
		copy(copied.Base, ch.Base)
		copy(copied.Water, ch.Water)
		copy(copied.Cover, ch.Cover)
		copy(copied.Surface, ch.Surface)
		copy(copied.Stock, ch.Stock)
		copy(copied.Meta, ch.Meta)
		copy(copied.Temperature, ch.Temperature)
		loaded[coord] = copied
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.chunks[worldID] = loaded
	s.loadedWorlds[worldID] = true
	return nil
}

func (s *Service) WorldChunks(worldID uint64) map[world.ChunkCoord]*world.Chunk {
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyChunks(s.chunks[worldID])
}

func (s *Service) CompactWorld(ctx context.Context, worldID uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store == nil {
		return nil
	}
	return s.store.Compact(ctx, copyChunks(s.chunks[worldID]), s.tick)
}

func (s *Service) SetWorldRenderSeed(worldID uint64, seed string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.renderSeeds[worldID] = seed
}

func (s *Service) WorldRenderSeed(worldID uint64) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if seed := s.renderSeeds[worldID]; seed != "" {
		return seed
	}
	return "demo"
}

func (s *Service) Actor(ctx context.Context, worldID, actorID uint64) (actor.Actor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	act, ok := s.actors[actor.ID(actorID)]
	if !ok || act.WorldID != worldID {
		return actor.Actor{}, ErrForbidden
	}
	return *act, nil
}

func (s *Service) Inventory(ctx context.Context, worldID, actorID uint64) ([]InventoryItem, error) {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	act, ok := s.actors[actor.ID(actorID)]
	if !ok || act.WorldID != worldID {
		return nil, ErrForbidden
	}
	return s.inventorySnapshotLocked(*act), nil
}

func (s *Service) VisibleChunksForActor(ctx context.Context, worldID, actorID uint64) (map[world.ChunkCoord]struct{}, error) {
	act, err := s.Actor(ctx, worldID, actorID)
	if err != nil {
		return nil, err
	}
	center, _ := world.ToChunkCoord(act.X, act.Y)
	return realtime.VisibleChunks(center, s.config.VisibleChunkRadius), nil
}

type ChunkSnapshot struct {
	CX          int32       `json:"cx"`
	CY          int32       `json:"cy"`
	Base        Uint16Layer `json:"base"`
	Water       []uint8     `json:"water"`
	Cover       Uint16Layer `json:"cover"`
	Surface     Uint16Layer `json:"surface"`
	Stock       Uint16Layer `json:"stock"`
	Meta        []uint8     `json:"meta"`
	Temperature []uint8     `json:"temperature"`
	UpdatedTick uint64      `json:"updated_tick"`
}

func (s *Service) ChunkSnapshots(ctx context.Context, worldID uint64, coords map[world.ChunkCoord]struct{}) []ChunkSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.chunkSnapshotsLocked(worldID, coords)
}

func (s *Service) chunkSnapshotsLocked(worldID uint64, coords map[world.ChunkCoord]struct{}) []ChunkSnapshot {
	snapshots := make([]ChunkSnapshot, 0, len(coords))
	for coord := range coords {
		ch := s.chunkLocked(worldID, coord)
		if ch == nil {
			if s.loadedWorlds[worldID] {
				continue
			}
			ch = s.ensureChunkLocked(worldID, coord)
		}
		snapshots = append(snapshots, snapshotChunk(ch, s.tick))
	}
	return snapshots
}

func (s *Service) ensureChunkLocked(worldID uint64, coord world.ChunkCoord) *world.Chunk {
	byWorld := s.chunks[worldID]
	if byWorld == nil {
		byWorld = make(map[world.ChunkCoord]*world.Chunk)
		s.chunks[worldID] = byWorld
	}
	ch := byWorld[coord]
	if ch == nil {
		ch = world.NewChunk(coord.X, coord.Y)
		for i := range ch.Stock {
			ch.Stock[i] = 3
		}
		byWorld[coord] = ch
	}
	return ch
}

func (s *Service) chunkLocked(worldID uint64, coord world.ChunkCoord) *world.Chunk {
	byWorld := s.chunks[worldID]
	if byWorld == nil {
		return nil
	}
	return byWorld[coord]
}

func (s *Service) seedDemoActorLocked(worldID uint64) actor.Actor {
	if existing, ok := s.actors[1]; ok && existing.WorldID == worldID {
		if existing.PocketInventoryID == 0 {
			existing.PocketInventoryID = 1
		}
		s.ensurePocketInventoryLocked(*existing)
		return *existing
	}
	act := actor.Actor{ID: 1, WorldID: worldID, X: DemoActorStartX, Y: DemoActorStartY, PocketInventoryID: 1}
	s.actors[act.ID] = &act
	s.ensurePocketInventoryLocked(act)
	return act
}

func (s *Service) worldTimeLocked() WorldTime {
	return BuildWorldTime(s.worldTime, s.clockConfig)
}

func (s *Service) nextEventIDLocked() uint64 {
	s.nextID++
	return s.nextID
}

func (s *Service) nextEventID() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nextEventIDLocked()
}
