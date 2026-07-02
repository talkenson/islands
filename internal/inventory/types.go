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
