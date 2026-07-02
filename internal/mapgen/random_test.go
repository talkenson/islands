package mapgen

import (
	"math"
	"testing"
)

func TestRandomMatchesCanvasGenerator(t *testing.T) {
	r := newRandom("talkenson")
	want := []float64{
		0.061956643127,
		0.733431965578,
		0.774458873319,
		0.765504633542,
		0.776750893798,
	}

	for i, expected := range want {
		got := r.next()
		if math.Abs(got-expected) > 0.000000000001 {
			t.Fatalf("next %d: got %.12f, want %.12f", i, got, expected)
		}
	}
}
