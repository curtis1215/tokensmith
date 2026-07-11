// internal/tui/theme_test.go
package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestCardInForcesWidth(t *testing.T) {
	a := CardIn(CardDefault, 40, "短", "內容")
	b := CardIn(CardAccent, 40, "很長的標題喔喔喔", "不同長度的內容行")
	if lipgloss.Width(a) != 40 {
		t.Fatalf("CardDefault width = %d, want 40", lipgloss.Width(a))
	}
	if lipgloss.Width(b) != 40 {
		t.Fatalf("CardAccent width = %d, want 40", lipgloss.Width(b))
	}
}

func TestCardInAutoWidthWhenZero(t *testing.T) {
	got := CardIn(CardDefault, 0, "標題", "行")
	if lipgloss.Width(got) >= 40 {
		t.Fatalf("auto width should shrink to content, got %d", lipgloss.Width(got))
	}
	if !strings.Contains(got, "標題") {
		t.Fatalf("missing title: %q", got)
	}
}

func TestCardVariantsRenderTitleAndBody(t *testing.T) {
	for _, k := range []CardKind{CardDefault, CardAccent, CardThreat, CardGold} {
		got := CardIn(k, 0, "T", "B")
		if !strings.Contains(got, "T") || !strings.Contains(got, "B") {
			t.Fatalf("kind %d missing content: %q", k, got)
		}
	}
}

func TestCardBackwardCompatible(t *testing.T) {
	if Card("標題", "內容") != CardIn(CardDefault, 0, "標題", "內容") {
		t.Fatal("Card must delegate to CardIn(CardDefault, 0, ...)")
	}
}

func TestBarWidthAndChars(t *testing.T) {
	for _, frac := range []float64{-0.5, 0, 0.33, 0.5, 1, 1.7} {
		got := Bar(frac, 10)
		if lipgloss.Width(got) != 10 {
			t.Fatalf("Bar(%v) width = %d, want 10", frac, lipgloss.Width(got))
		}
	}
	if !strings.Contains(Bar(1, 4), "████") {
		t.Fatalf("full bar should be solid blocks: %q", Bar(1, 4))
	}
	if !strings.Contains(Bar(0, 4), "░░░░") {
		t.Fatalf("empty bar should be all shade: %q", Bar(0, 4))
	}
}

func TestLoadColorThresholds(t *testing.T) {
	if loadColor(0.5) != colorCyan {
		t.Fatal("0.5 should be cyan")
	}
	if loadColor(0.75) != colorAmber {
		t.Fatal("0.75 should be amber")
	}
	if loadColor(0.95) != colorLoss {
		t.Fatal("0.95 should be red")
	}
}

func TestFilledCellsClamps(t *testing.T) {
	if filledCells(-1, 10) != 0 || filledCells(2, 10) != 10 {
		t.Fatal("filledCells must clamp frac to [0,1]")
	}
}
