package game

import (
	"islands/internal/actor"
	"islands/internal/inventory"
)

const (
	ItemWood        inventory.ItemID = 1
	PocketSlotLimit                  = 9
)

type InventoryItem struct {
	ItemID  inventory.ItemID `json:"item_id"`
	Name    string           `json:"name"`
	Amount  uint32           `json:"amount"`
	Quality uint8            `json:"quality"`
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

func (s *Service) addStackLocked(invID uint64, itemID inventory.ItemID, amount uint32) bool {
	id := inventory.ID(invID)
	stackSet := s.stacks[id]
	if stackSet == nil {
		stackSet = inventory.NewStackSet()
		s.stacks[id] = stackSet
	}
	if stackSet.Add(inventory.Stack{InventoryID: id, ItemID: itemID, Amount: amount}, s.inventorySlotLimitLocked(id)) {
		return true
	}
	if stackSet.Len() == 0 {
		delete(s.stacks, id)
	}
	return false
}

func (s *Service) stackLocked(invID uint64, itemID inventory.ItemID) (inventory.Stack, bool) {
	stackSet := s.stacks[inventory.ID(invID)]
	if stackSet == nil {
		return inventory.Stack{}, false
	}
	return stackSet.Get(itemID)
}

func (s *Service) restoreStackLocked(invID uint64, itemID inventory.ItemID, previous inventory.Stack, hadStack bool) {
	id := inventory.ID(invID)
	stackSet := s.stacks[id]
	if !hadStack {
		if stackSet != nil {
			stackSet.Restore(itemID, previous, false)
			if stackSet.Len() == 0 {
				delete(s.stacks, id)
			}
		}
		return
	}
	if stackSet == nil {
		stackSet = inventory.NewStackSet()
		s.stacks[id] = stackSet
	}
	stackSet.Restore(itemID, previous, true)
}

func (s *Service) inventorySnapshotLocked(act actor.Actor) []InventoryItem {
	stackSet := s.stacks[inventory.ID(act.PocketInventoryID)]
	if stackSet == nil || stackSet.Len() == 0 {
		return nil
	}
	stacks := stackSet.Items(s.inventorySlotLimitLocked(inventory.ID(act.PocketInventoryID)))
	out := make([]InventoryItem, 0, len(stacks))
	for _, stack := range stacks {
		out = append(out, InventoryItem{
			ItemID:  stack.ItemID,
			Name:    itemName(stack.ItemID),
			Amount:  stack.Amount,
			Quality: stack.Quality,
		})
	}
	return out
}

func (s *Service) inventorySlotLimitLocked(invID inventory.ID) int {
	inv := s.inventories[invID]
	if inv != nil && inv.Kind == inventory.KindPocket {
		return PocketSlotLimit
	}
	return 0
}

func (s *Service) inventoryPatchLocked(act actor.Actor) InventoryPatch {
	return InventoryPatch{
		ActorID:     uint64(act.ID),
		InventoryID: act.PocketInventoryID,
		Inventory:   s.inventorySnapshotLocked(act),
	}
}

func itemName(itemID inventory.ItemID) string {
	switch itemID {
	case ItemWood:
		return "wood"
	default:
		return "unknown"
	}
}
