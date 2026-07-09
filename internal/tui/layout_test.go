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
