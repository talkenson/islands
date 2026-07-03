package game

import (
	"context"
	"errors"
	"sync"

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

	ItemWood inventory.ItemID = 1
)

type Service struct {
	mu           sync.Mutex
	actors       map[actor.ID]*actor.Actor
	inventories  map[inventory.ID]*inventory.Inventory
	stacks       map[inventory.ID]map[inventory.ItemID]*inventory.Stack
	chunks       map[uint64]map[world.ChunkCoord]*world.Chunk
	loadedWorlds map[uint64]bool
	renderSeeds  map[uint64]string
	tick         uint64
	nextID       uint64
	hub          *realtime.Hub
	config       realtime.Config
	store        storage.Store
}

func NewService(hub *realtime.Hub, config realtime.Config) *Service {
	if hub == nil {
		hub = realtime.NewHub()
	}
	return &Service{
		actors:       make(map[actor.ID]*actor.Actor),
		inventories:  make(map[inventory.ID]*inventory.Inventory),
		stacks:       make(map[inventory.ID]map[inventory.ItemID]*inventory.Stack),
		chunks:       make(map[uint64]map[world.ChunkCoord]*world.Chunk),
		loadedWorlds: make(map[uint64]bool),
		renderSeeds:  make(map[uint64]string),
		hub:          hub,
		config:       config.Normalize(),
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
	s.ensureChunkLocked(worldID, coord)
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
	return nil
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

type InventoryItem struct {
	ItemID  inventory.ItemID `json:"item_id"`
	Name    string           `json:"name"`
	Amount  uint32           `json:"amount"`
	Quality uint8            `json:"quality"`
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
	CX          int32    `json:"cx"`
	CY          int32    `json:"cy"`
	Base        []uint16 `json:"base"`
	Water       []uint8  `json:"water"`
	Cover       []uint16 `json:"cover"`
	Stock       []uint16 `json:"stock"`
	Meta        []uint8  `json:"meta"`
	Temperature []uint8  `json:"temperature"`
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
	Accepted       bool            `json:"accepted"`
	ClientActionID string          `json:"client_action_id,omitempty"`
	ActionType     string          `json:"action_type,omitempty"`
	Actor          actor.Actor     `json:"actor"`
	Inventory      []InventoryItem `json:"inventory"`
	EventID        uint64          `json:"event_id"`
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
		if !validMove(*act, req.X, req.Y) {
			s.mu.Unlock()
			return ActionResult{}, ErrInvalidAction
		}
		targetCoord, _ := world.ToChunkCoord(req.X, req.Y)
		if s.loadedWorlds[worldID] && s.chunkLocked(worldID, targetCoord) == nil {
			s.mu.Unlock()
			return ActionResult{}, ErrNotVisible
		}
		oldCenter, _ := world.ToChunkCoord(act.X, act.Y)
		previousX := act.X
		previousY := act.Y
		previousTick := s.tick
		act.X = req.X
		act.Y = req.Y
		s.tick++
		store := s.store
		if store != nil {
			if err := store.SavePlayerState(ctx, s.playerStateLocked(), s.tick); err != nil {
				act.X = previousX
				act.Y = previousY
				s.tick = previousTick
				s.mu.Unlock()
				return ActionResult{}, err
			}
		}
		eventID := s.nextEventIDLocked()
		center, _ := world.ToChunkCoord(act.X, act.Y)
		interest := realtime.VisibleChunks(center, s.config.VisibleChunkRadius)
		oldInterest := realtime.VisibleChunks(oldCenter, s.config.VisibleChunkRadius)
		newChunks := interestDifference(interest, oldInterest)
		changed := interestList(interest)
		snapshots := s.chunkSnapshotsLocked(worldID, newChunks)
		result := ActionResult{Accepted: true, ClientActionID: req.ClientActionID, ActionType: req.ActionType, Actor: *act, Inventory: s.inventorySnapshotLocked(*act), EventID: eventID}
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
		previousStock := ch.Stock[index]
		previousDirty := ch.Dirty
		previousTick := s.tick
		previousStack, hadStack := s.stackLocked(act.PocketInventoryID, ItemWood)
		ch.Stock[index]--
		s.addStackLocked(act.PocketInventoryID, ItemWood, 1)
		ch.Dirty = true
		s.tick++
		store := s.store
		if store != nil {
			if err := store.SaveDirtyChunk(ctx, ch, s.tick); err != nil {
				ch.Stock[index] = previousStock
				ch.Dirty = previousDirty
				s.restoreStackLocked(act.PocketInventoryID, ItemWood, previousStack, hadStack)
				s.tick = previousTick
				s.mu.Unlock()
				return ActionResult{}, err
			}
			if err := store.SavePlayerState(ctx, s.playerStateLocked(), s.tick); err != nil {
				ch.Stock[index] = previousStock
				ch.Dirty = previousDirty
				s.restoreStackLocked(act.PocketInventoryID, ItemWood, previousStack, hadStack)
				s.tick = previousTick
				s.mu.Unlock()
				return ActionResult{}, err
			}
		}
		eventID := s.nextEventIDLocked()
		snapshot := snapshotChunk(ch, s.tick)
		result := ActionResult{Accepted: true, ClientActionID: req.ClientActionID, ActionType: req.ActionType, Actor: *act, Inventory: s.inventorySnapshotLocked(*act), EventID: eventID}
		s.mu.Unlock()
		s.hub.Publish(realtime.Event{
			ID:            eventID,
			Type:          "entity_patch",
			WorldID:       worldID,
			ChangedChunks: []world.ChunkCoord{coord},
			Data:          result,
		})
		snapshotID := s.nextEventID()
		s.hub.Publish(realtime.Event{
			ID:            snapshotID,
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

func validMove(act actor.Actor, x, y int32) bool {
	dx := int64(x) - int64(act.X)
	dy := int64(y) - int64(act.Y)
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	return dx+dy == 1
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

func (s *Service) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	store := s.store
	state := s.playerStateLocked()
	tick := s.tick
	s.mu.Unlock()
	if store == nil {
		return nil
	}
	if err := store.SavePlayerState(ctx, state, tick); err != nil {
		return err
	}
	return store.Flush(ctx)
}

func (s *Service) loadPlayersLocked(state storage.PlayerState) {
	if len(state.Actors) > 0 {
		s.actors = make(map[actor.ID]*actor.Actor, len(state.Actors))
		for id, act := range state.Actors {
			if act == nil {
				continue
			}
			copied := *act
			s.actors[id] = &copied
		}
	}
	if len(state.Inventories) > 0 {
		s.inventories = make(map[inventory.ID]*inventory.Inventory, len(state.Inventories))
		for id, inv := range state.Inventories {
			if inv == nil {
				continue
			}
			copied := *inv
			s.inventories[id] = &copied
		}
	}
	if len(state.Stacks) > 0 {
		s.stacks = make(map[inventory.ID]map[inventory.ItemID]*inventory.Stack)
		for _, stack := range state.Stacks {
			copied := stack
			byItem := s.stacks[stack.InventoryID]
			if byItem == nil {
				byItem = make(map[inventory.ItemID]*inventory.Stack)
				s.stacks[stack.InventoryID] = byItem
			}
			byItem[stack.ItemID] = &copied
		}
	}
	for _, act := range s.actors {
		if act.PocketInventoryID == 0 {
			act.PocketInventoryID = uint64(act.ID)
		}
		s.ensurePocketInventoryLocked(*act)
	}
}

func (s *Service) playerStateLocked() storage.PlayerState {
	state := storage.PlayerState{
		Actors:      make(map[actor.ID]*actor.Actor, len(s.actors)),
		Inventories: make(map[inventory.ID]*inventory.Inventory, len(s.inventories)),
	}
	for id, act := range s.actors {
		if act == nil {
			continue
		}
		copied := *act
		state.Actors[id] = &copied
	}
	for id, inv := range s.inventories {
		if inv == nil {
			continue
		}
		copied := *inv
		state.Inventories[id] = &copied
	}
	for _, byItem := range s.stacks {
		for _, stack := range byItem {
			if stack != nil && stack.Amount > 0 {
				state.Stacks = append(state.Stacks, *stack)
			}
		}
	}
	return state
}

func (s *Service) ensurePocketInventoryLocked(act actor.Actor) {
	invID := inventory.ID(act.PocketInventoryID)
	if invID == 0 {
		return
	}
	if _, ok := s.inventories[invID]; ok {
		return
	}
	s.inventories[invID] = &inventory.Inventory{
		ID:        invID,
		WorldID:   act.WorldID,
		Kind:      inventory.KindPocket,
		OwnerType: inventory.OwnerActor,
		OwnerID:   uint64(act.ID),
		MaxWeight: 100,
		MaxVolume: 100,
	}
}

func (s *Service) addStackLocked(invID uint64, itemID inventory.ItemID, amount uint32) {
	id := inventory.ID(invID)
	byItem := s.stacks[id]
	if byItem == nil {
		byItem = make(map[inventory.ItemID]*inventory.Stack)
		s.stacks[id] = byItem
	}
	stack := byItem[itemID]
	if stack == nil {
		stack = &inventory.Stack{InventoryID: id, ItemID: itemID}
		byItem[itemID] = stack
	}
	stack.Amount += amount
}

func (s *Service) stackLocked(invID uint64, itemID inventory.ItemID) (inventory.Stack, bool) {
	byItem := s.stacks[inventory.ID(invID)]
	if byItem == nil || byItem[itemID] == nil {
		return inventory.Stack{}, false
	}
	return *byItem[itemID], true
}

func (s *Service) restoreStackLocked(invID uint64, itemID inventory.ItemID, previous inventory.Stack, hadStack bool) {
	id := inventory.ID(invID)
	byItem := s.stacks[id]
	if !hadStack {
		if byItem != nil {
			delete(byItem, itemID)
			if len(byItem) == 0 {
				delete(s.stacks, id)
			}
		}
		return
	}
	if byItem == nil {
		byItem = make(map[inventory.ItemID]*inventory.Stack)
		s.stacks[id] = byItem
	}
	copied := previous
	byItem[itemID] = &copied
}

func (s *Service) inventorySnapshotLocked(act actor.Actor) []InventoryItem {
	byItem := s.stacks[inventory.ID(act.PocketInventoryID)]
	if len(byItem) == 0 {
		return nil
	}
	out := make([]InventoryItem, 0, len(byItem))
	for _, stack := range byItem {
		if stack == nil || stack.Amount == 0 {
			continue
		}
		out = append(out, InventoryItem{
			ItemID:  stack.ItemID,
			Name:    itemName(stack.ItemID),
			Amount:  stack.Amount,
			Quality: stack.Quality,
		})
	}
	return out
}

func itemName(itemID inventory.ItemID) string {
	switch itemID {
	case ItemWood:
		return "wood"
	default:
		return "unknown"
	}
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
		Temperature: append([]uint8(nil), ch.Temperature...),
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

func copyChunks(chunks map[world.ChunkCoord]*world.Chunk) map[world.ChunkCoord]*world.Chunk {
	copied := make(map[world.ChunkCoord]*world.Chunk, len(chunks))
	for coord, ch := range chunks {
		if ch == nil {
			continue
		}
		next := world.NewChunk(ch.X, ch.Y)
		copy(next.Base, ch.Base)
		copy(next.Water, ch.Water)
		copy(next.Cover, ch.Cover)
		copy(next.Stock, ch.Stock)
		copy(next.Meta, ch.Meta)
		copy(next.Temperature, ch.Temperature)
		next.Dirty = ch.Dirty
		copied[coord] = next
	}
	return copied
}
