package mapgen

import (
	"encoding/binary"
	"fmt"
	"io"
	"sort"

	"islands/internal/world"
)

var mapMagic = [8]byte{'I', 'S', 'L', 'M', 'A', 'P', '0', '1'}

func SaveBinary(w io.Writer, m *Map) error {
	if _, err := w.Write(mapMagic[:]); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(m.Width)); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(m.Height)); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(world.ChunkSize)); err != nil {
		return err
	}

	coords := make([]world.ChunkCoord, 0, len(m.Chunks))
	for coord := range m.Chunks {
		coords = append(coords, coord)
	}
	sort.Slice(coords, func(i, j int) bool {
		if coords[i].Y == coords[j].Y {
			return coords[i].X < coords[j].X
		}
		return coords[i].Y < coords[j].Y
	})

	if err := binary.Write(w, binary.LittleEndian, uint32(len(coords))); err != nil {
		return err
	}

	for _, coord := range coords {
		ch := m.Chunks[coord]
		if err := ch.Validate(); err != nil {
			return fmt.Errorf("chunk %d,%d: %w", coord.X, coord.Y, err)
		}
		if err := binary.Write(w, binary.LittleEndian, ch.X); err != nil {
			return err
		}
		if err := binary.Write(w, binary.LittleEndian, ch.Y); err != nil {
			return err
		}
		if err := writeUint16s(w, ch.Base); err != nil {
			return err
		}
		if _, err := w.Write(ch.Water); err != nil {
			return err
		}
		if err := writeUint16s(w, ch.Cover); err != nil {
			return err
		}
		if err := writeUint16s(w, ch.Stock); err != nil {
			return err
		}
		if _, err := w.Write(ch.Meta); err != nil {
			return err
		}
	}

	return nil
}

func LoadBinary(r io.Reader) (*Map, error) {
	var magic [8]byte
	if _, err := io.ReadFull(r, magic[:]); err != nil {
		return nil, err
	}
	if magic != mapMagic {
		return nil, fmt.Errorf("invalid map magic")
	}

	var width uint32
	var height uint32
	var chunkSize uint32
	var chunkCount uint32
	if err := binary.Read(r, binary.LittleEndian, &width); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &height); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &chunkSize); err != nil {
		return nil, err
	}
	if chunkSize != world.ChunkSize {
		return nil, fmt.Errorf("unsupported chunk size %d", chunkSize)
	}
	if err := binary.Read(r, binary.LittleEndian, &chunkCount); err != nil {
		return nil, err
	}

	m := &Map{
		Width:  int(width),
		Height: int(height),
		Chunks: make(map[world.ChunkCoord]*world.Chunk, chunkCount),
	}

	for i := uint32(0); i < chunkCount; i++ {
		var cx int32
		var cy int32
		if err := binary.Read(r, binary.LittleEndian, &cx); err != nil {
			return nil, err
		}
		if err := binary.Read(r, binary.LittleEndian, &cy); err != nil {
			return nil, err
		}

		ch := world.NewChunk(cx, cy)
		if err := readUint16s(r, ch.Base); err != nil {
			return nil, err
		}
		if _, err := io.ReadFull(r, ch.Water); err != nil {
			return nil, err
		}
		if err := readUint16s(r, ch.Cover); err != nil {
			return nil, err
		}
		if err := readUint16s(r, ch.Stock); err != nil {
			return nil, err
		}
		if _, err := io.ReadFull(r, ch.Meta); err != nil {
			return nil, err
		}
		m.Chunks[world.ChunkCoord{X: cx, Y: cy}] = ch
	}

	m.Stats = collectStats(m)
	return m, nil
}

func writeUint16s(w io.Writer, values []uint16) error {
	for _, value := range values {
		if err := binary.Write(w, binary.LittleEndian, value); err != nil {
			return err
		}
	}
	return nil
}

func readUint16s(r io.Reader, values []uint16) error {
	for i := range values {
		if err := binary.Read(r, binary.LittleEndian, &values[i]); err != nil {
			return err
		}
	}
	return nil
}
