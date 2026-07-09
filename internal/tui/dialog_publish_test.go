package tui

import (
	"math"
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

func TestPublishDialogWithTechRefPrice(t *testing.T) {
	m := testModel(t)
	m.state.Models = []model.Model{{Online: false, Users: 0, Gen: 1, Segment: model.SegConsumer, Price: 0}}
	m.page = PageModels

	d, ok := newPublishDialog(m, 0)
	if !ok || d.refPrice != 12 {
		t.Fatalf("expected refPrice 12, got %v", d.refPrice)
	}

	m.state.UnlockedTech = []string{"biz-price-1"}
	d2, ok := newPublishDialog(m, 0)
	if !ok || math.Abs(d2.refPrice-13.2) > 1e-9 {
		t.Fatalf("expected refPrice 13.2, got %v", d2.refPrice)
	}

	m.publish = &d2
	d2.price = 6.6
	v2 := renderPublishDialog(d2, m)
	if !strings.Contains(v2, "×2.83") {
		t.Fatalf("expected demand multiplier ×2.83 in view:\n%s", v2)
	}
}

func TestPublishDialogConfirmPublish(t *testing.T) {
	m := testModel(t)
	m.state.Models = []model.Model{{Online: false, Users: 0, Gen: 1, Segment: model.SegConsumer}}
	m.page = PageModels

	d, ok := newPublishDialog(m, 0)
	if !ok {
		t.Fatal("failed to create dialog")
	}
	m.publish = &d
	m.publish.name = "MyModel"
	m.publish.price = 10

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := nm.(Model)

	if updated.publish != nil {
		t.Fatal("publish dialog should close on successful publish")
	}
	if len(updated.state.Models) != 1 || !updated.state.Models[0].Online || updated.state.Models[0].Name != "MyModel" || updated.state.Models[0].Price != 10 {
		t.Fatalf("model not published properly: %+v", updated.state.Models[0])
	}
	if !strings.Contains(updated.notice, "MyModel") {
		t.Fatalf("notice missing name, got: %s", updated.notice)
	}
}

func TestPublishDialogConfirmReprice(t *testing.T) {
	m := testModel(t)
	m.state.Models = []model.Model{{Online: true, Users: 100, Gen: 1, Segment: model.SegConsumer, Name: "LiveModel", Price: 15}}
	m.page = PageModels

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'$'}})
	updated := nm.(Model)
	if updated.publish == nil || !updated.publish.priceOnly {
		t.Fatal("failed to open reprice dialog")
	}

	updated.publish.price = 25
	nm2, _ := updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated2 := nm2.(Model)

	if updated2.publish != nil {
		t.Fatal("reprice dialog should close on confirm")
	}
	if updated2.state.Models[0].Price != 25 {
		t.Fatalf("price not updated, got %v", updated2.state.Models[0].Price)
	}
}
