package mapgen

import "math"

type valueNoise struct {
	seed   string
	values map[[2]int]float64
}

func newValueNoise(seed string) *valueNoise {
	return &valueNoise{
		seed:   seed,
		values: make(map[[2]int]float64),
	}
}

func fade(t float64) float64 {
	return t * t * t * (t*(t*6-15) + 10)
}

func lerp(a, b, t float64) float64 {
	return a + (b-a)*t
}

func (n *valueNoise) lattice(ix, iy int) float64 {
	key := [2]int{ix, iy}
	if value, ok := n.values[key]; ok {
		return value
	}
	r := newRandom(makeSeed(n.seed, stringKey(ix, iy)))
	value := r.next()
	n.values[key] = value
	return value
}

func stringKey(ix, iy int) string {
	return itoa(ix) + "," + itoa(iy)
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func (n *valueNoise) noise2D(x, y float64) float64 {
	x0 := int(math.Floor(x))
	y0 := int(math.Floor(y))
	x1 := x0 + 1
	y1 := y0 + 1
	sx := fade(x - float64(x0))
	sy := fade(y - float64(y0))

	n0 := lerp(n.lattice(x0, y0), n.lattice(x1, y0), sx)
	n1 := lerp(n.lattice(x0, y1), n.lattice(x1, y1), sx)
	return lerp(n0, n1, sy)
}

func (n *valueNoise) octaveNoise2D(x, y float64, octaves int, persistence float64) float64 {
	amplitude := 1.0
	frequency := 1.0
	total := 0.0
	maxValue := 0.0

	for i := 0; i < octaves; i++ {
		total += n.noise2D(x*frequency, y*frequency) * amplitude
		maxValue += amplitude
		amplitude *= persistence
		frequency *= 2
	}

	return total / maxValue
}
