package tui

import (
	"strings"
	"testing"
)

func TestMarketPageShowsSegmentsAndRivals(t *testing.T) {
	m := testModel(t)
	m.page = PageMarket
	v := renderMarket(m)
	for _, w := range []string{"消費者", "企業", "開發者", "對手"} {
		if !strings.Contains(v, w) {
			t.Errorf("market page missing %q:\n%s", w, v)
		}
	}
}
