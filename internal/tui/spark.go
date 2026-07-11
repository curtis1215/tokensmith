// internal/tui/spark.go
package tui

var sparkRunes = []rune("▁▂▃▄▅▆▇█")

// spark is a fixed-capacity ring buffer of display-layer samples.
// TUI-memory only; never persisted.
type spark struct {
	buf  []float64
	head int // next write index
	n    int
}

func newSpark(capacity int) spark {
	return spark{buf: make([]float64, capacity)}
}

func (s *spark) push(v float64) {
	s.buf[s.head] = v
	s.head = (s.head + 1) % len(s.buf)
	if s.n < len(s.buf) {
		s.n++
	}
}

// values returns samples oldest→newest.
func (s *spark) values() []float64 {
	out := make([]float64, 0, s.n)
	start := (s.head - s.n + len(s.buf)) % len(s.buf)
	for i := 0; i < s.n; i++ {
		out = append(out, s.buf[(start+i)%len(s.buf)])
	}
	return out
}

// Render draws the newest `width` samples; "" when fewer than 2 samples.
func (s *spark) Render(width int) string {
	vals := s.values()
	if len(vals) > width {
		vals = vals[len(vals)-width:]
	}
	if len(vals) < 2 {
		return ""
	}
	lo, hi := vals[0], vals[0]
	for _, v := range vals {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	out := make([]rune, 0, len(vals))
	for _, v := range vals {
		idx := 0
		if hi > lo {
			idx = int((v - lo) / (hi - lo) * float64(len(sparkRunes)-1))
		}
		out = append(out, sparkRunes[idx])
	}
	return string(out)
}
