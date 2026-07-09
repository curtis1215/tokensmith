package sim

import "testing"

func TestNextRandDeterministic(t *testing.T) {
	s1, r1 := nextRand(42)
	s2, r2 := nextRand(42)
	if s1 != s2 || r1 != r2 {
		t.Fatalf("same input state must give same output: (%d,%v) vs (%d,%v)", s1, r1, s2, r2)
	}
	if s1 == 42 {
		t.Fatal("state must advance")
	}
}

func TestNextRandRangeAndSpread(t *testing.T) {
	state := uint64(7)
	var lo, hi int
	for i := 0; i < 1000; i++ {
		var r float64
		state, r = nextRand(state)
		if r < 0 || r >= 1 {
			t.Fatalf("r = %v out of [0,1)", r)
		}
		if r < 0.5 {
			lo++
		} else {
			hi++
		}
	}
	if lo < 400 || hi < 400 {
		t.Fatalf("distribution too skewed: lo=%d hi=%d", lo, hi)
	}
}

func TestNextRandZeroStateWorks(t *testing.T) {
	state, r := nextRand(0)
	if state == 0 {
		t.Fatal("state must advance from 0")
	}
	if r < 0 || r >= 1 {
		t.Fatalf("r = %v out of [0,1)", r)
	}
}
