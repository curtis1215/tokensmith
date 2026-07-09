package tui

import (
	"strings"
	"testing"

	"tokensmith/internal/balance"
)

func TestTechCatalogCoversDefaultNodes(t *testing.T) {
	for _, n := range balance.Default().TechNodes {
		if techLabel(n.ID).Name == n.ID && !strings.HasPrefix(n.ID, "x") {
			if _, ok := techCatalog[n.ID]; !ok {
				t.Errorf("missing tech meta for %s", n.ID)
			}
		}
	}
}
