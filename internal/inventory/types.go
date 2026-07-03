package inventory

type ID uint64
type ItemID uint16

type Kind uint8

const (
	KindPocket Kind = iota
	KindCart
	KindGroundPile
	KindSettlementStorage
	KindBuildingStorage
)

type OwnerType uint8

const (
	OwnerNone OwnerType = iota
	OwnerActor
	OwnerSettlement
	OwnerBuilding
	OwnerCart
)

type Inventory struct {
	ID      ID
	WorldID uint64
	Kind    Kind

	OwnerType OwnerType
	OwnerID   uint64

	MaxWeight   uint32
	MaxVolume   uint32
	UpdatedTick uint64
}

type Stack struct {
	InventoryID ID
	ItemID      ItemID
	Amount      uint32
	Quality     uint8
}

type StackSet struct {
	order  []ItemID
	byItem map[ItemID]*Stack
}

func NewStackSet() *StackSet {
	return &StackSet{byItem: make(map[ItemID]*Stack)}
}

func (s *StackSet) Len() int {
	if s == nil {
		return 0
	}
	return len(s.order)
}

func (s *StackSet) Get(itemID ItemID) (Stack, bool) {
	if s == nil || s.byItem == nil || s.byItem[itemID] == nil {
		return Stack{}, false
	}
	return *s.byItem[itemID], true
}

func (s *StackSet) Add(stack Stack, slotLimit int) bool {
	if s.byItem == nil {
		s.byItem = make(map[ItemID]*Stack)
	}
	existing := s.byItem[stack.ItemID]
	if existing != nil {
		existing.Amount += stack.Amount
		if stack.Quality != 0 {
			existing.Quality = stack.Quality
		}
		return true
	}
	if slotLimit > 0 && len(s.order) >= slotLimit {
		return false
	}
	copied := stack
	s.byItem[stack.ItemID] = &copied
	s.order = append(s.order, stack.ItemID)
	return true
}

func (s *StackSet) Restore(itemID ItemID, previous Stack, hadStack bool) {
	if s.byItem == nil {
		s.byItem = make(map[ItemID]*Stack)
	}
	if !hadStack {
		if _, ok := s.byItem[itemID]; ok {
			delete(s.byItem, itemID)
			for i, orderedID := range s.order {
				if orderedID == itemID {
					s.order = append(s.order[:i], s.order[i+1:]...)
					break
				}
			}
		}
		return
	}
	if _, ok := s.byItem[itemID]; !ok {
		s.order = append(s.order, itemID)
	}
	copied := previous
	s.byItem[itemID] = &copied
}

func (s *StackSet) Items(limit int) []Stack {
	if s == nil || len(s.order) == 0 {
		return nil
	}
	out := make([]Stack, 0, len(s.order))
	for _, itemID := range s.order {
		stack := s.byItem[itemID]
		if stack == nil || stack.Amount == 0 {
			continue
		}
		out = append(out, *stack)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

type ItemTag uint32

const (
	TagTool ItemTag = 1 << iota
	TagKey
	TagMoney
	TagMap
	TagSeed
	TagSmallValuable
	TagBulkResource
)

type ItemDef struct {
	ID       ItemID
	Tags     ItemTag
	Weight   uint16
	Volume   uint16
	MaxStack uint32
}

func (d ItemDef) FitsPocket() bool {
	return d.Tags&TagBulkResource == 0
}
