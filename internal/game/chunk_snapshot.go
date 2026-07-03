package game

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
)

type Uint16Layer []uint16

func (l Uint16Layer) MarshalJSON() ([]byte, error) {
	buf := make([]byte, len(l)*2)
	for i, value := range l {
		binary.LittleEndian.PutUint16(buf[i*2:], value)
	}
	return json.Marshal(base64.StdEncoding.EncodeToString(buf))
}
