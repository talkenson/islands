package game

import (
	"context"

	"islands/internal/actor"
	"islands/internal/realtime"
	"islands/internal/world"
)

func (s *Service) completePendingMove(ctx context.Context, worldID, actorID uint64) {
	s.mu.Lock()
	act, ok := s.actors[actor.ID(actorID)]
	if !ok || act.WorldID != worldID {
		delete(s.pendingMoves, actor.ID(actorID))
		s.mu.Unlock()
		return
	}
	move := s.pendingMoves[act.ID]
	if move == nil || move.WorldID != worldID {
		s.mu.Unlock()
		return
	}
	if s.shuttingDown {
		delete(s.pendingMoves, act.ID)
		s.mu.Unlock()
		return
	}
	if act.X != move.FromX || act.Y != move.FromY {
		delete(s.pendingMoves, act.ID)
		s.mu.Unlock()
		return
	}

	oldCenter, _ := world.ToChunkCoord(act.X, act.Y)
	act.X = move.TargetX
	act.Y = move.TargetY
	delete(s.pendingMoves, act.ID)
	s.tick++

	var persistenceErr error
	store := s.store
	if store != nil {
		if err := store.SavePlayerState(ctx, s.playerStateLocked(), s.tick); err != nil {
			persistenceErr = err
		}
	}

	eventID := s.nextEventIDLocked()
	var persistenceErrEventID uint64
	if persistenceErr != nil {
		persistenceErrEventID = s.nextEventIDLocked()
	}
	center, _ := world.ToChunkCoord(act.X, act.Y)
	interest := realtime.VisibleChunks(center, s.config.VisibleChunkRadius)
	oldInterest := realtime.VisibleChunks(oldCenter, s.config.VisibleChunkRadius)
	newChunks := interestDifference(interest, oldInterest)
	changed := interestList(interest)
	snapshots := s.chunkSnapshotsLocked(worldID, newChunks)
	patch := EntityPatch{Actor: actorSnapshot(*act)}
	s.mu.Unlock()

	s.hub.SetActorInterest(worldID, actorID, interest)
	s.hub.Publish(realtime.Event{ID: eventID, Type: "entity_patch", WorldID: worldID, ChangedChunks: changed, Data: patch})
	if persistenceErr != nil {
		s.hub.Publish(realtime.Event{
			ID:            persistenceErrEventID,
			Type:          "stream_error",
			WorldID:       worldID,
			TargetActorID: actorID,
			Data: map[string]string{
				"message": "player state save failed after movement: " + persistenceErr.Error(),
			},
		})
	}
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
}
