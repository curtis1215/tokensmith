package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestHQStageFromOffice(t *testing.T) {
	// Office.Level 1..8 → art index 0..7
	if hqStageFromOffice(1) != 0 {
		t.Fatal("level 1 → garage art index 0")
	}
	if hqStageFromOffice(8) != 7 {
		t.Fatal("level 8 → last art index 7")
	}
	if hqStageFromOffice(0) != 0 || hqStageFromOffice(-3) != 0 {
		t.Fatal("level < 1 clamps to index 0")
	}
	if hqStageFromOffice(99) != 7 {
		t.Fatal("level > 8 clamps to index 7")
	}
	if hqStageFromOffice(5) != 4 {
		t.Fatal("level 5 → art index 4")
	}
}

func TestHQStageClamps(t *testing.T) {
	// hqStage still clamps raw art indices used by hqArt.
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
	// Office level 6 → art index 5 → 摩天樓 (balance.OfficeNames)
	m.state.Office.Level = 6
	wide := renderHQ(m, 110)
	if !strings.Contains(wide, "摩天樓") {
		t.Fatalf("wide HQ missing stage name: %q", wide)
	}
	narrow := renderHQ(m, 80)
	if strings.Count(narrow, "\n") > 4 {
		t.Fatalf("narrow HQ should be compact, got %d lines", strings.Count(narrow, "\n")+1)
	}
	if !strings.Contains(narrow, "🗼") {
		t.Fatalf("narrow HQ missing stage icons: %q", narrow)
	}
}

func TestHQStageNamesMatchOfficeNames(t *testing.T) {
	// Align with balance.Default().OfficeNames[1..8]
	want := [8]string{
		"車庫", "小辦公室", "開放式樓層", "辦公樓", "園區", "摩天樓", "巨塔", "太空電梯",
	}
	if hqStageNames != want {
		t.Fatalf("hqStageNames=%v want %v", hqStageNames, want)
	}
}
