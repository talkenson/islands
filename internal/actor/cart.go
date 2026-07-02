package actor

type LockState uint8

const (
	LockOpen LockState = iota
	LockClosed
	LockBroken
)

type Cart struct {
	ID      uint64
	WorldID uint64

	X int32
	Y int32

	OwnerID uint64

	InventoryID uint64
	LockID      uint64
	LockState   LockState

	AttachedToActorID uint64
	HP                uint16
}

func (c Cart) IsAttached() bool {
	return c.AttachedToActorID != 0
}
