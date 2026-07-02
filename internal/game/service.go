package game

import (
	"context"
	"errors"
	"sync"

	"islands/internal/actor"
	"islands/internal/realtime"
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
	chunks       map[uint64]map[world.ChunkCoord]*world.Chunk
	loadedWorlds map[uint64]bool
	renderSeeds  map[uint64]string
	tick         uint64
	nextID       uint64
	hub          *realtime.Hub
	config       realtime.Config
}

func NewService(hub *realtime.Hub, config realtime.Config) *Service {
	if hub == nil {
		hub = realtime.NewHub()
	}
	return &Service{
		actors:       make(map[actor.ID]*actor.Actor),
		chunks:       make(map[uint64]map[world.ChunkCoord]*world.Chunk),
		loadedWorlds: make(map[uint64]bool),
		renderSeeds:  make(map[uint64]string),
		hub:          hub,
		config:       config.Normalize(),
	}
}

func (s *Service) SeedDemoWorld(worldID uint64) actor.Actor {
	s.mu.Lock()
	defer s.mu.Unlock()

	act := seedDemoActorLocked(s.actors, worldID)
	coord, _ := world.ToChunkCoord(act.X, act.Y)
	s.ensureChunkLocked(worldID, coord)
	s.renderSeeds[worldID] = "demo"
	return act
}

func (s *Service) SeedDemoActor(worldID uint64) actor.Actor {
	s.mu.Lock()
	defer s.mu.Unlock()
	return seedDemoActorLocked(s.actors, worldID)
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
		copy(copied.Stock, ch.Stock)
		copy(copied.Meta, ch.Meta)
		loaded[coord] = copied
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.chunks[worldID] = loaded
	s.loadedWorlds[worldID] = true
	return nil
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

func (s *Service) VisibleChunksForActor(ctx context.Context, worldID, actorID uint64) (map[world.ChunkCoord]struct{}, error) {
	act, err := s.Actor(ctx, worldID, actorID)
	if err != nil {
		return nil, err
	}
	center, _ := world.ToChunkCoord(act.X, act.Y)
	return realtime.VisibleChunks(center, s.config.VisibleChunkRadius), nil
}

type ChunkSnapshot struct {
	CX          int32    `json:"cx"`
	CY          int32    `json:"cy"`
	Base        []uint16 `json:"base"`
	Water       []uint8  `json:"water"`
	Cover       []uint16 `json:"cover"`
	Stock       []uint16 `json:"stock"`
	Meta        []uint8  `json:"meta"`
	UpdatedTick uint64   `json:"updated_tick"`
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

type ActionRequest struct {
	ActionType     string `json:"action_type"`
	ClientActionID string `json:"client_action_id,omitempty"`
	X              int32  `json:"x,omitempty"`
	Y              int32  `json:"y,omitempty"`
}

type ActionResult struct {
	Accepted       bool        `json:"accepted"`
	ClientActionID string      `json:"client_action_id,omitempty"`
	Actor          actor.Actor `json:"actor"`
	EventID        uint64      `json:"event_id"`
}

func (s *Service) ApplyAction(ctx context.Context, worldID, actorID uint64, req ActionRequest) (ActionResult, error) {
	s.mu.Lock()
	act, ok := s.actors[actor.ID(actorID)]
	if !ok || act.WorldID != worldID {
		s.mu.Unlock()
		return ActionResult{}, ErrForbidden
	}

	switch req.ActionType {
	case "move":
		oldCenter, _ := world.ToChunkCoord(act.X, act.Y)
		act.X = req.X
		act.Y = req.Y
		s.tick++
		eventID := s.nextEventIDLocked()
		center, _ := world.ToChunkCoord(act.X, act.Y)
		interest := realtime.VisibleChunks(center, s.config.VisibleChunkRadius)
		oldInterest := realtime.VisibleChunks(oldCenter, s.config.VisibleChunkRadius)
		newChunks := interestDifference(interest, oldInterest)
		changed := interestList(interest)
		snapshots := s.chunkSnapshotsLocked(worldID, newChunks)
		result := ActionResult{Accepted: true, ClientActionID: req.ClientActionID, Actor: *act, EventID: eventID}
		s.mu.Unlock()
		s.hub.SetActorInterest(worldID, actorID, interest)
		s.hub.Publish(realtime.Event{ID: eventID, Type: "entity_patch", WorldID: worldID, ChangedChunks: changed, Data: result})
		for _, snapshot := range snapshots {
			snapshotID := s.nextEventID()
			s.hub.Publish(realtime.Event{
				ID:            snapshotID,
				Type:          "chunk_snapshot",
				WorldID:       worldID,
				ChangedChunks: []world.ChunkCoord{{X: snapshot.CX, Y: snapshot.CY}},
				Data:          snapshot,
			})
		}
		return result, nil
	case "harvest":
		coord, index := world.ToChunkCoord(act.X, act.Y)
		ch := s.chunkLocked(worldID, coord)
		if ch == nil {
			if s.loadedWorlds[worldID] {
				s.mu.Unlock()
				return ActionResult{}, ErrNotVisible
			}
			ch = s.ensureChunkLocked(worldID, coord)
		}
		if ch.Stock[index] == 0 {
			s.mu.Unlock()
			return ActionResult{}, ErrConflict
		}
		ch.Stock[index]--
		ch.Dirty = true
		s.tick++
		eventID := s.nextEventIDLocked()
		snapshot := snapshotChunk(ch, s.tick)
		result := ActionResult{Accepted: true, ClientActionID: req.ClientActionID, Actor: *act, EventID: eventID}
		s.mu.Unlock()
		s.hub.Publish(realtime.Event{
			ID:            eventID,
			Type:          "chunk_snapshot",
			WorldID:       worldID,
			ChangedChunks: []world.ChunkCoord{coord},
			Data:          snapshot,
		})
		return result, nil
	default:
		s.mu.Unlock()
		return ActionResult{}, ErrInvalidAction
	}
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

func seedDemoActorLocked(actors map[actor.ID]*actor.Actor, worldID uint64) actor.Actor {
	act := actor.Actor{ID: 1, WorldID: worldID, X: DemoActorStartX, Y: DemoActorStartY, PocketInventoryID: 1}
	actors[act.ID] = &act
	return act
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

func snapshotChunk(ch *world.Chunk, tick uint64) ChunkSnapshot {
	return ChunkSnapshot{
		CX:          ch.X,
		CY:          ch.Y,
		Base:        append([]uint16(nil), ch.Base...),
		Water:       append([]uint8(nil), ch.Water...),
		Cover:       append([]uint16(nil), ch.Cover...),
		Stock:       append([]uint16(nil), ch.Stock...),
		Meta:        append([]uint8(nil), ch.Meta...),
		UpdatedTick: tick,
	}
}

func interestList(interest map[world.ChunkCoord]struct{}) []world.ChunkCoord {
	out := make([]world.ChunkCoord, 0, len(interest))
	for coord := range interest {
		out = append(out, coord)
	}
	return out
}

func interestDifference(next, previous map[world.ChunkCoord]struct{}) map[world.ChunkCoord]struct{} {
	out := make(map[world.ChunkCoord]struct{})
	for coord := range next {
		if _, ok := previous[coord]; !ok {
			out[coord] = struct{}{}
		}
	}
	return out
}
