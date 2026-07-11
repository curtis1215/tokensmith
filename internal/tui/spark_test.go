// internal/tui/spark_test.go
package tui

import "testing"

func TestSparkPushWrapsAndOrders(t *testing.T) {
	s := newSpark(3)
	for _, v := range []float64{1, 2, 3, 4} {
		s.push(v)
	}
	got := s.values()
	want := []float64{2, 3, 4}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("values() = %v, want %v", got, want)
		}
	}
}

func TestSparkRender(t *testing.T) {
	s := newSpark(8)
	s.push(0)
	s.push(50)
	s.push(100)
	out := s.Render(8)
	r := []rune(out)
	if len(r) != 3 {
		t.Fatalf("rune len = %d, want 3 (%q)", len(r), out)
	}
	if r[0] != '▁' || r[2] != '█' {
		t.Fatalf("min/max runes wrong: %q", out)
	}
}

func TestSparkRenderFlatAndEmpty(t *testing.T) {
	s := newSpark(4)
	if s.Render(4) != "" {
		t.Fatal("empty spark should render empty string")
	}
	s.push(5)
	if s.Render(4) != "" {
		t.Fatal("single sample should render empty string")
	}
	s.push(5)
	out := []rune(s.Render(4))
	if len(out) != 2 || out[0] != out[1] {
		t.Fatalf("flat series should be uniform runes: %q", string(out))
	}
}

func TestSparkRenderTruncatesToWidth(t *testing.T) {
	s := newSpark(16)
	for i := 0; i < 16; i++ {
		s.push(float64(i))
	}
	if got := len([]rune(s.Render(6))); got != 6 {
		t.Fatalf("render width = %d, want 6", got)
	}
}
