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
