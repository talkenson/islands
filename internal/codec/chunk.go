package codec

import (
	"encoding/binary"
	"fmt"
	"io"

	"islands/internal/world"
)

var chunkMagic = [8]byte{'I', 'S', 'L', 'C', 'H', 'N', 'K', '1'}

const chunkPayloadSize = 8 + 4 + 4 +
	world.ChunkCells*2 +
	world.ChunkCells +
	world.ChunkCells*2 +
	world.ChunkCells*2 +
	world.ChunkCells

func EncodeChunk(ch *world.Chunk) ([]byte, error) {
	if ch == nil {
		return nil, fmt.Errorf("nil chunk")
	}
	if err := ch.Validate(); err != nil {
		return nil, err
	}

	buf := make([]byte, chunkPayloadSize)
	copy(buf[:8], chunkMagic[:])
	binary.LittleEndian.PutUint32(buf[8:12], uint32(ch.X))
	binary.LittleEndian.PutUint32(buf[12:16], uint32(ch.Y))
	offset := 16
	offset = putUint16s(buf, offset, ch.Base)
	copy(buf[offset:], ch.Water)
	offset += len(ch.Water)
	offset = putUint16s(buf, offset, ch.Cover)
	offset = putUint16s(buf, offset, ch.Stock)
	copy(buf[offset:], ch.Meta)
	return buf, nil
}

func DecodeChunk(payload []byte) (*world.Chunk, error) {
	if len(payload) != chunkPayloadSize {
		return nil, fmt.Errorf("chunk payload length: got %d, want %d", len(payload), chunkPayloadSize)
	}
	var magic [8]byte
	copy(magic[:], payload[:8])
	if magic != chunkMagic {
		return nil, fmt.Errorf("invalid chunk magic")
	}

	x := int32(binary.LittleEndian.Uint32(payload[8:12]))
	y := int32(binary.LittleEndian.Uint32(payload[12:16]))
	ch := world.NewChunk(x, y)
	offset := 16
	offset = readUint16s(payload, offset, ch.Base)
	copy(ch.Water, payload[offset:offset+world.ChunkCells])
	offset += world.ChunkCells
	offset = readUint16s(payload, offset, ch.Cover)
	offset = readUint16s(payload, offset, ch.Stock)
	copy(ch.Meta, payload[offset:offset+world.ChunkCells])
	if err := ch.Validate(); err != nil {
		return nil, err
	}
	return ch, nil
}

func WriteChunk(w io.Writer, ch *world.Chunk) error {
	payload, err := EncodeChunk(ch)
	if err != nil {
		return err
	}
	_, err = w.Write(payload)
	return err
}

func putUint16s(buf []byte, offset int, values []uint16) int {
	for _, value := range values {
		binary.LittleEndian.PutUint16(buf[offset:], value)
		offset += 2
	}
	return offset
}

func readUint16s(buf []byte, offset int, values []uint16) int {
	for i := range values {
		values[i] = binary.LittleEndian.Uint16(buf[offset:])
		offset += 2
	}
	return offset
}
