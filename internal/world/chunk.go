package world

import "fmt"

type Chunk struct {
	X int32
	Y int32

	Base  []uint16
	Water []uint8
	Cover []uint16
	Stock []uint16
	Meta  []uint8

	Dirty bool
}

func NewChunk(x, y int32) *Chunk {
	return &Chunk{
		X:     x,
		Y:     y,
		Base:  make([]uint16, ChunkCells),
		Water: make([]uint8, ChunkCells),
		Cover: make([]uint16, ChunkCells),
		Stock: make([]uint16, ChunkCells),
		Meta:  make([]uint8, ChunkCells),
	}
}

func (c *Chunk) Validate() error {
	if len(c.Base) != ChunkCells {
		return fmt.Errorf("base length: got %d, want %d", len(c.Base), ChunkCells)
	}
	if len(c.Water) != ChunkCells {
		return fmt.Errorf("water length: got %d, want %d", len(c.Water), ChunkCells)
	}
	if len(c.Cover) != ChunkCells {
		return fmt.Errorf("cover length: got %d, want %d", len(c.Cover), ChunkCells)
	}
	if len(c.Stock) != ChunkCells {
		return fmt.Errorf("stock length: got %d, want %d", len(c.Stock), ChunkCells)
	}
	if len(c.Meta) != ChunkCells {
		return fmt.Errorf("meta length: got %d, want %d", len(c.Meta), ChunkCells)
	}
	return nil
}

func (c *Chunk) SetBase(index uint16, cell BaseCell) {
	c.Base[index] = uint16(cell)
	c.Dirty = true
}

func (c *Chunk) BaseCell(index uint16) BaseCell {
	return BaseCell(c.Base[index])
}

func (c *Chunk) SetWater(index uint16, cell WaterCell) {
	c.Water[index] = uint8(cell)
	c.Dirty = true
}

func (c *Chunk) WaterCell(index uint16) WaterCell {
	return WaterCell(c.Water[index])
}

func (c *Chunk) SetCover(index uint16, cell CoverCell) {
	c.Cover[index] = uint16(cell)
	c.Dirty = true
}

func (c *Chunk) CoverCell(index uint16) CoverCell {
	return CoverCell(c.Cover[index])
}

func (c *Chunk) SetStock(index uint16, stock uint16) {
	c.Stock[index] = stock
	c.Dirty = true
}
