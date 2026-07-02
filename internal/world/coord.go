package world

const (
	ChunkSize  = 32
	ChunkCells = ChunkSize * ChunkSize
)

type ChunkCoord struct {
	X int32
	Y int32
}

func ToChunkCoord(x, y int32) (ChunkCoord, uint16) {
	cx, lx := divmodFloor(x, ChunkSize)
	cy, ly := divmodFloor(y, ChunkSize)
	return ChunkCoord{X: cx, Y: cy}, uint16(ly*ChunkSize + lx)
}

func LocalXY(index uint16) (x, y uint8) {
	return uint8(index % ChunkSize), uint8(index / ChunkSize)
}

func divmodFloor(v int32, d int32) (q int32, r int32) {
	q = v / d
	r = v % d
	if r < 0 {
		q--
		r += d
	}
	return q, r
}
