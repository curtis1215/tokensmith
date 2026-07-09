package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"tokensmith/internal/model"
)

func TestPublishDialogCommand(t *testing.T) {
	d := publishDialog{index: 0, name: "A", price: 10}
	cmd := d.command()
	if cmd.ModelIndex != 0 || cmd.Name != "A" || cmd.Price != 10 {
		t.Fatalf("%+v", cmd)
	}
}

func TestPublishDialogRejectsNonDraft(t *testing.T) {
	m := testModel(t)
	m.state.Models = []model.Model{{Online: true, Users: 1, Name: "x"}}
	if _, ok := newPublishDialog(m, 0); ok {
		t.Fatal("should reject live model")
	}
}

func TestPublishDialogEmptyNameValidation(t *testing.T) {
	m := testModel(t)
	m.state.Models = []model.Model{{Online: false, Users: 0, Gen: 1, Segment: model.SegConsumer}}
	m.page = PageModels

	d, ok := newPublishDialog(m, 0)
	if !ok {
		t.Fatal("failed to create publish dialog")
	}
	m.publish = &d
	m.publish.name = "   "

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := nm.(Model)

	if updated.publish == nil {
		t.Fatal("publish dialog was silently closed on empty name")
	}
	if !strings.Contains(updated.notice, "名稱") {
		t.Fatalf("expected validation notice, got: %s", updated.notice)
	}
}

func TestVisualIndices(t *testing.T) {
	models := []model.Model{
		{Online: true, Users: 100},
		{Online: false, Users: 0},
		{Online: true, Users: 50},
		{Online: false, Users: 0},
	}
	vis := visualIndices(models)
	expected := []int{1, 3, 0, 2}
	if len(vis) != len(expected) {
		t.Fatalf("len %v, want %v", len(vis), len(expected))
	}
	for i, v := range vis {
		if v != expected[i] {
			t.Errorf("vis[%d] = %d, want %d", i, v, expected[i])
		}
	}
}
