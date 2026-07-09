package tui

import (
	"strings"
	"testing"
)

func TestCardContainsTitleAndBody(t *testing.T) {
	s := Card("公司", "估值 $1M")
	if !strings.Contains(s, "公司") || !strings.Contains(s, "估值") {
		t.Fatalf("card missing content: %q", s)
	}
}

func TestTruncateRunes(t *testing.T) {
	if Truncate("你好世界", 2) != "你好" {
		t.Fatalf("got %q", Truncate("你好世界", 2))
	}
	if Truncate("abc", 10) != "abc" {
		t.Fatal("no-op truncate failed")
	}
}

func TestFooterIncludesGlobalKeys(t *testing.T) {
	f := Footer("[t]訓練")
	if !strings.Contains(f, "[t]訓練") || !strings.Contains(f, "[Tab]") || !strings.Contains(f, "[q]") {
		t.Fatalf("footer: %q", f)
	}
}

func TestHRow(t *testing.T) {
	s := HRow(2, "A", "B", "C")
	if !strings.Contains(s, "A") || !strings.Contains(s, "B") || !strings.Contains(s, "C") {
		t.Fatalf("HRow missing content: %q", s)
	}
}

func TestVStack(t *testing.T) {
	s := VStack("Line1", "Line2")
	if !strings.Contains(s, "Line1") || !strings.Contains(s, "Line2") {
		t.Fatalf("VStack missing content: %q", s)
	}
}

func TestKV(t *testing.T) {
	s := KV("估值", "$1M")
	if !strings.Contains(s, "估值") || !strings.Contains(s, "$1M") {
		t.Fatalf("KV missing content: %q", s)
	}
}

func TestBar(t *testing.T) {
	s := Bar(0.5, 10)
	if !strings.Contains(s, "▓") || !strings.Contains(s, "░") {
		t.Fatalf("Bar missing content: %q", s)
	}
}

func TestResponsiveRow(t *testing.T) {
	// Case 1: width < 80 should stack vertically (contain newline)
	narrow := ResponsiveRow(50, 2, "AAAAA", "BBBBB")
	if !strings.Contains(narrow, "\n") {
		t.Errorf("expected narrow row to stack vertically, got %q", narrow)
	}

	// Case 2: width >= 80, but parts are too wide to fit in the available width
	part1 := strings.Repeat("A", 50)
	part2 := strings.Repeat("B", 50)
	tooWide := ResponsiveRow(90, 2, part1, part2)
	if !strings.Contains(tooWide, "\n") {
		t.Errorf("expected too wide row to stack vertically, got %q", tooWide)
	}

	// Case 3: width >= 80, parts fit in the available width
	wideFit := ResponsiveRow(100, 2, "AAA", "BBB")
	if strings.Contains(wideFit, "\n") {
		t.Errorf("expected wide fitting row to be horizontal (no newline), got %q", wideFit)
	}
}
