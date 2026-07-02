package actor

type ID uint64

type Actor struct {
	ID      ID
	WorldID uint64

	X int32
	Y int32

	PocketInventoryID uint64
	AttachedCartID    uint64
}
