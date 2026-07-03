package game

import (
	"context"

	"islands/internal/actor"
	"islands/internal/realtime"
	"islands/internal/world"
)

type ActionRequest struct {
	ActionType     string `json:"action_type"`
	ClientActionID string `json:"client_action_id,omitempty"`
	X              int32  `json:"x,omitempty"`
	Y              int32  `json:"y,omitempty"`
}

type ActorSnapshot struct {
	ID actor.ID `json:"id"`
	X  int32    `json:"x"`
	Y  int32    `json:"y"`
}

type ActionResult struct {
	Accepted       bool   `json:"accepted"`
	ClientActionID string `json:"client_action_id,omitempty"`
	ActionType     string `json:"action_type,omitempty"`
	EventID        uint64 `json:"event_id"`
}

type EntityPatch struct {
	Actor ActorSnapshot `json:"actor"`
}

type InventoryPatch struct {
	ActorID     uint64          `json:"actor_id"`
	InventoryID uint64          `json:"inventory_id"`
	Inventory   []InventoryItem `json:"inventory"`
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
		result := ActionResult{Accepted: true, ClientActionID: req.ClientActionID, ActionType: req.ActionType, EventID: eventID}
		patch := EntityPatch{Actor: actorSnapshot(*act)}
		s.mu.Unlock()
		s.hub.SetActorInterest(worldID, actorID, interest)
		s.hub.Publish(realtime.Event{ID: eventID, Type: "entity_patch", WorldID: worldID, ChangedChunks: changed, Data: patch})
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
		if !s.addStackLocked(act.PocketInventoryID, ItemWood, 1) {
			s.mu.Unlock()
			return ActionResult{}, ErrConflict
		}
		ch.Stock[index]--
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
		inventoryPatchID := s.nextEventIDLocked()
		snapshot := snapshotChunk(ch, s.tick)
		inventoryPatch := s.inventoryPatchLocked(*act)
		result := ActionResult{Accepted: true, ClientActionID: req.ClientActionID, ActionType: req.ActionType, EventID: eventID}
		patch := EntityPatch{Actor: actorSnapshot(*act)}
		s.mu.Unlock()
		s.hub.Publish(realtime.Event{
			ID:            eventID,
			Type:          "entity_patch",
			WorldID:       worldID,
			ChangedChunks: []world.ChunkCoord{coord},
			Data:          patch,
		})
		snapshotID := s.nextEventID()
		s.hub.Publish(realtime.Event{
			ID:            snapshotID,
			Type:          "chunk_snapshot",
			WorldID:       worldID,
			ChangedChunks: []world.ChunkCoord{coord},
			Data:          snapshot,
		})
		s.hub.Publish(realtime.Event{
			ID:            inventoryPatchID,
			Type:          "inventory_patch",
			WorldID:       worldID,
			TargetActorID: actorID,
			Data:          inventoryPatch,
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

func actorSnapshot(act actor.Actor) ActorSnapshot {
	return ActorSnapshot{
		ID: act.ID,
		X:  act.X,
		Y:  act.Y,
	}
}
