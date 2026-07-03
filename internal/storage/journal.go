package storage

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"

	"islands/internal/codec"
	"islands/internal/world"
)

var (
	journalMagic        = [8]byte{'I', 'S', 'L', 'J', 'R', 'E', 'C', '1'}
	ErrJournalTruncated = errors.New("journal truncated")
	ErrJournalChecksum  = errors.New("journal checksum mismatch")
)

const journalHeaderSize = 32

type journalRecord struct {
	Tick    uint64
	Coord   world.ChunkCoord
	Payload []byte
}

func writeJournalRecord(w io.Writer, tick uint64, ch *world.Chunk) error {
	payload, err := codec.EncodeChunk(ch)
	if err != nil {
		return err
	}
	if len(payload) > int(^uint32(0)) {
		return fmt.Errorf("journal payload too large: %d", len(payload))
	}

	header := make([]byte, journalHeaderSize)
	copy(header[:8], journalMagic[:])
	binary.LittleEndian.PutUint64(header[8:16], tick)
	binary.LittleEndian.PutUint32(header[16:20], uint32(ch.X))
	binary.LittleEndian.PutUint32(header[20:24], uint32(ch.Y))
	binary.LittleEndian.PutUint32(header[24:28], uint32(len(payload)))
	binary.LittleEndian.PutUint32(header[28:32], journalChecksum(header, payload))

	if _, err := w.Write(header); err != nil {
		return err
	}
	_, err = w.Write(payload)
	return err
}

func readJournal(r io.Reader, apply func(journalRecord) error) (uint64, error) {
	var maxTick uint64
	for {
		record, err := readJournalRecord(r)
		if errors.Is(err, io.EOF) {
			return maxTick, nil
		}
		if err != nil {
			return maxTick, err
		}
		if record.Tick > maxTick {
			maxTick = record.Tick
		}
		if err := apply(record); err != nil {
			return maxTick, err
		}
	}
}

func readJournalRecord(r io.Reader) (journalRecord, error) {
	header := make([]byte, journalHeaderSize)
	n, err := io.ReadFull(r, header)
	if errors.Is(err, io.EOF) && n == 0 {
		return journalRecord{}, io.EOF
	}
	if err != nil {
		return journalRecord{}, ErrJournalTruncated
	}

	var magic [8]byte
	copy(magic[:], header[:8])
	if magic != journalMagic {
		return journalRecord{}, fmt.Errorf("invalid journal magic")
	}

	tick := binary.LittleEndian.Uint64(header[8:16])
	cx := int32(binary.LittleEndian.Uint32(header[16:20]))
	cy := int32(binary.LittleEndian.Uint32(header[20:24]))
	payloadLen := binary.LittleEndian.Uint32(header[24:28])
	expectedChecksum := binary.LittleEndian.Uint32(header[28:32])
	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(r, payload); err != nil {
		return journalRecord{}, ErrJournalTruncated
	}
	if got := journalChecksum(header, payload); got != expectedChecksum {
		return journalRecord{}, ErrJournalChecksum
	}
	return journalRecord{
		Tick:    tick,
		Coord:   world.ChunkCoord{X: cx, Y: cy},
		Payload: payload,
	}, nil
}

func journalChecksum(header []byte, payload []byte) uint32 {
	hash := crc32.NewIEEE()
	_, _ = hash.Write(header[8:28])
	_, _ = hash.Write(payload)
	return hash.Sum32()
}
