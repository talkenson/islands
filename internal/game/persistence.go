package game

import (
	"context"
	"sort"

	"islands/internal/actor"
	"islands/internal/inventory"
	"islands/internal/storage"
)

func (s *Service) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.shuttingDown = true
	for id, move := range s.pendingMoves {
		if move != nil && move.Timer != nil {
			move.Timer.Stop()
		}
		delete(s.pendingMoves, id)
	}
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
		s.stacks = make(map[inventory.ID]*inventory.StackSet)
		for _, stack := range state.Stacks {
			stackSet := s.stacks[stack.InventoryID]
			if stackSet == nil {
				stackSet = inventory.NewStackSet()
				s.stacks[stack.InventoryID] = stackSet
			}
			stackSet.Add(stack, 0)
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
		WorldTime:   s.worldTime,
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
	stackInventoryIDs := make([]inventory.ID, 0, len(s.stacks))
	for id := range s.stacks {
		stackInventoryIDs = append(stackInventoryIDs, id)
	}
	sort.Slice(stackInventoryIDs, func(i, j int) bool {
		return stackInventoryIDs[i] < stackInventoryIDs[j]
	})
	for _, id := range stackInventoryIDs {
		state.Stacks = append(state.Stacks, s.stacks[id].Items(0)...)
	}
	return state
}
