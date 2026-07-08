package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func testModel(t *testing.T) Model {
	m := newAt(filepath.Join(t.TempDir(), "s.json"))
	m.poller = ingestEmptyPoller(t)
	return m
}

func TestTabCyclesPages(t *testing.T) {
	m := testModel(t)
	if m.page != PageOverview {
		t.Fatalf("start page = %v, want overview", m.page)
	}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if nm.(Model).page != PageModels {
		t.Fatalf("after Tab = %v, want models", nm.(Model).page)
	}
}

func TestNumberKeyJumpsPage(t *testing.T) {
	m := testModel(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'5'}})
	if nm.(Model).page != PageTeam {
		t.Fatalf("key 5 = %v, want team", nm.(Model).page)
	}
}

func TestViewHasChrome(t *testing.T) {
	m := testModel(t)
	v := m.View()
	if !strings.Contains(v, "Tokensmith") || !strings.Contains(v, "總覽") || !strings.Contains(v, "團隊") {
		t.Fatalf("view missing chrome:\n%s", v)
	}
}

func TestProgressBar(t *testing.T) {
	got := progressBar(0.5, 10)
	full := strings.Count(got, "▓")
	if full != 5 {
		t.Fatalf("progressBar(0.5,10) filled=%d, want 5 (%q)", full, got)
	}
}
