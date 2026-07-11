package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestHQStageClamps(t *testing.T) {
	if hqStage(-1) != 0 || hqStage(0) != 0 || hqStage(3) != 3 || hqStage(99) != 7 {
		t.Fatal("hqStage must clamp to [0,7]")
	}
}

func TestHQArtShapes(t *testing.T) {
	for s := 0; s < 8; s++ {
		art := hqArt(s, false)
		lines := strings.Split(art, "\n")
		if len(lines) != 5 {
			t.Fatalf("stage %d: %d lines, want 5", s, len(lines))
		}
		for _, ln := range lines {
			if lipgloss.Width(ln) > 30 {
				t.Fatalf("stage %d art too wide: %q", s, ln)
			}
		}
	}
	if !strings.Contains(hqArt(2, true), "●") {
		t.Fatal("lit art should contain ●")
	}
	if strings.Contains(hqArt(2, false), "●") {
		t.Fatal("unlit art should not contain ●")
	}
}

func TestRenderHQWideAndNarrow(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	m.state.MilestonesReached = 4
	wide := renderHQ(m, 110)
	if !strings.Contains(wide, "摩天大樓") {
		t.Fatalf("wide HQ missing stage name: %q", wide)
	}
	narrow := renderHQ(m, 80)
	if strings.Count(narrow, "\n") > 4 {
		t.Fatalf("narrow HQ should be compact, got %d lines", strings.Count(narrow, "\n")+1)
	}
	if !strings.Contains(narrow, "🏙") {
		t.Fatalf("narrow HQ missing stage icons: %q", narrow)
	}
}
