package mapgen

import "math/bits"

type random struct {
	state uint32
}

func newRandom(seed string) *random {
	h := hashSeed(seed)
	return &random{state: h()}
}

func hashSeed(seed string) func() uint32 {
	text := []rune(seed)
	h := uint32(1779033703) ^ uint32(len(text))

	for _, r := range text {
		h = uint32(uint64(h^uint32(r)) * 3432918353)
		h = bits.RotateLeft32(h, 13)
	}

	return func() uint32 {
		h = uint32(uint64(h^(h>>16)) * 2246822507)
		h = uint32(uint64(h^(h>>13)) * 3266489909)
		h ^= h >> 16
		return h
	}
}

func makeSeed(seed, salt string) string {
	return seed + ":" + salt
}

func (r *random) next() float64 {
	r.state += 0x6d2b79f5
	t := r.state
	t = uint32(uint64(t^(t>>15)) * uint64(t|1))
	t ^= t + uint32(uint64(t^(t>>7))*uint64(t|61))
	return float64((t^(t>>14))>>0) / 4294967296
}

func (r *random) rangeFloat(min, max float64) float64 {
	return min + (max-min)*r.next()
}

func (r *random) int(min, max int) int {
	return min + int(r.next()*float64(max-min+1))
}
