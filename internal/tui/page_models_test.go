package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"tokensmith/internal/model"
)

func TestModelsPageListsModels(t *testing.T) {
	m := testModel(t)
	m.state.Models = []model.Model{{Gen: 2, Segment: model.SegConsumer, Online: true, Users: 500, Price: 12}}
	m.page = PageModels
	v := renderModels(m)
	if !strings.Contains(v, "Gen2") || !strings.Contains(v, "消費者") {
		t.Fatalf("models list missing entries:\n%s", v)
	}
}

func TestTKeyOpensTrainDialog(t *testing.T) {
	m := testModel(t)
	m.page = PageModels
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if nm.(Model).dialog == nil {
		t.Fatalf("t should open the training dialog on models page")
	}
}
