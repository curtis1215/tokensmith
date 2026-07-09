package tui

import (
	"testing"

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
