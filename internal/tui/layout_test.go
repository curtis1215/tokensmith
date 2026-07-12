package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
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

func TestFooterDialogHidesShellKeys(t *testing.T) {
	f := Footer("")
	if strings.Contains(f, "[Tab]") || strings.Contains(f, "[q]") {
		t.Fatalf("dialog footer should not advertise shell keys: %q", f)
	}
	if !strings.Contains(f, "[Esc]") {
		t.Fatalf("dialog footer should mention Esc: %q", f)
	}
}

func TestTruncateWidth(t *testing.T) {
	if TruncateWidth("abcdef", 3) != "abc" {
		t.Fatalf("got %q", TruncateWidth("abcdef", 3))
	}
	if TruncateWidth("你好世界", 4) != "你好" { // CJK: 2 cells each
		t.Fatalf("got %q want 你好", TruncateWidth("你好世界", 4))
	}
	if TruncateWidth("short", 100) != "short" {
		t.Fatal("no-op TruncateWidth failed")
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
	if !strings.Contains(s, "█") || !strings.Contains(s, "░") {
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

func TestGridEqualWidths(t *testing.T) {
	cell := func(label string) func(int) string {
		return func(w int) string { return CardIn(CardDefault, w, label, "x") }
	}
	got := Grid(100, 2, cell("A"), cell("B"), cell("C"))
	lines := strings.Split(got, "\n")
	// 第一行是兩張 49 寬卡 + 2 gap = 100
	if lipgloss.Width(lines[0]) != 100 {
		t.Fatalf("row width = %d, want 100", lipgloss.Width(lines[0]))
	}
	// 奇數尾 cell 拿全寬
	last := lines[len(lines)-1]
	if lipgloss.Width(last) != 100 {
		t.Fatalf("orphan cell width = %d, want 100", lipgloss.Width(last))
	}
}

func TestGridStacksWhenNarrow(t *testing.T) {
	cell := func(w int) string { return CardIn(CardDefault, w, "T", "B") }
	got := Grid(60, 2, cell, cell)
	for _, ln := range strings.Split(got, "\n") {
		if lipgloss.Width(ln) > 60 {
			t.Fatalf("narrow grid line overflows: %d > 60", lipgloss.Width(ln))
		}
	}
}

func TestPadBodyLines(t *testing.T) {
	got := padBodyLines("a\nb", 4)
	if lipgloss.Height(got) != 4 {
		t.Fatalf("height=%d want 4 (%q)", lipgloss.Height(got), got)
	}
	short := padBodyLines("a\nb\nc\nd\ne", 2)
	if lipgloss.Height(short) != 2 || !strings.Contains(short, "a") {
		t.Fatalf("truncate failed: %q", short)
	}
}

func TestEqualHeight(t *testing.T) {
	a := "1\n2\n3"
	b := "x"
	out := EqualHeight(a, b)
	if lipgloss.Height(out[0]) != lipgloss.Height(out[1]) {
		t.Fatalf("heights %d vs %d", lipgloss.Height(out[0]), lipgloss.Height(out[1]))
	}
	if lipgloss.Height(out[0]) != 3 {
		t.Fatalf("want height 3, got %d", lipgloss.Height(out[0]))
	}
}

func TestHRowEqual(t *testing.T) {
	left := CardIn(CardDefault, 40, "L", "a\nb\nc")
	right := CardIn(CardDefault, 40, "R", "x")
	// Finished-card EqualHeight only pads below the border.
	row := HRowEqual(2, left, right)
	if lipgloss.Height(row) < lipgloss.Height(left) {
		t.Fatalf("row shorter than tallest card")
	}
}

func TestHRowEqualCardsBordersMatch(t *testing.T) {
	row := HRowEqualCards(2,
		cardContent{kind: CardDefault, w: 40, title: "L", body: "a\nb\nc"},
		cardContent{kind: CardDefault, w: 40, title: "R", body: "x"},
	)
	// Reconstruct the two equalized cards and compare heights.
	left := cardContent{kind: CardDefault, w: 40, title: "L", body: "a\nb\nc"}
	right := cardContent{kind: CardDefault, w: 40, title: "R", body: "x"}
	maxH := lipgloss.Height(CardIn(left.kind, left.w, left.title, left.body))
	if h := lipgloss.Height(CardIn(right.kind, right.w, right.title, right.body)); h > maxH {
		maxH = h
	}
	lb, rb := left.body, right.body
	for lipgloss.Height(CardIn(left.kind, left.w, left.title, lb)) < maxH {
		lb += "\n"
	}
	for lipgloss.Height(CardIn(right.kind, right.w, right.title, rb)) < maxH {
		rb += "\n"
	}
	a := CardIn(left.kind, left.w, left.title, lb)
	b := CardIn(right.kind, right.w, right.title, rb)
	if lipgloss.Height(a) != lipgloss.Height(b) {
		t.Fatalf("equalized cards heights %d vs %d", lipgloss.Height(a), lipgloss.Height(b))
	}
	if lipgloss.Height(row) != lipgloss.Height(a) {
		t.Fatalf("row height %d want %d", lipgloss.Height(row), lipgloss.Height(a))
	}
	if !strings.Contains(row, "L") || !strings.Contains(row, "R") {
		t.Fatalf("row missing cards: %q", row)
	}
}

func TestHRowEqualCardsAccountsForWidthWrap(t *testing.T) {
	// Narrow cards force long single-line body to wrap into many visual lines.
	long := strings.Repeat("word ", 40) // will wrap inside w=30
	row := HRowEqualCards(2,
		cardContent{kind: CardDefault, w: 30, title: "短", body: "x"},
		cardContent{kind: CardDefault, w: 30, title: "長", body: long},
	)
	// Extract by re-equalizing the same contents and comparing component heights.
	shortC := cardContent{kind: CardDefault, w: 30, title: "短", body: "x"}
	longC := cardContent{kind: CardDefault, w: 30, title: "長", body: long}
	maxH := 0
	for _, c := range []cardContent{shortC, longC} {
		if h := lipgloss.Height(CardIn(c.kind, c.w, c.title, c.body)); h > maxH {
			maxH = h
		}
	}
	sb, lb := shortC.body, longC.body
	for lipgloss.Height(CardIn(shortC.kind, shortC.w, shortC.title, sb)) < maxH {
		sb += "\n"
	}
	for lipgloss.Height(CardIn(longC.kind, longC.w, longC.title, lb)) < maxH {
		lb += "\n"
	}
	sh := lipgloss.Height(CardIn(shortC.kind, shortC.w, shortC.title, sb))
	lh := lipgloss.Height(CardIn(longC.kind, longC.w, longC.title, lb))
	if sh != lh {
		t.Fatalf("wrap-aware equal heights %d vs %d (maxH=%d)", sh, lh, maxH)
	}
	if maxH < 5 {
		t.Fatalf("expected long body to wrap to >5 lines, got maxH=%d", maxH)
	}
	if lipgloss.Height(row) != sh {
		t.Fatalf("row height %d want %d", lipgloss.Height(row), sh)
	}
}

func TestGridNThreeColumns(t *testing.T) {
	cell := func(label string) func(int) string {
		return func(w int) string { return CardIn(CardDefault, w, label, "x") }
	}
	got := GridN(102, 2, 3, cell("A"), cell("B"), cell("C"))
	// First visual row should be ~102 wide (3 cols + 2 gaps).
	first := strings.Split(got, "\n")[0]
	if lipgloss.Width(first) != 102 {
		t.Fatalf("row width=%d want 102", lipgloss.Width(first))
	}
}

func TestGridNStacksWhenNarrow(t *testing.T) {
	cell := func(w int) string { return CardIn(CardDefault, w, "T", "B") }
	got := GridN(60, 2, 3, cell, cell, cell)
	for _, ln := range strings.Split(got, "\n") {
		if lipgloss.Width(ln) > 60 {
			t.Fatalf("overflow %d > 60", lipgloss.Width(ln))
		}
	}
}
