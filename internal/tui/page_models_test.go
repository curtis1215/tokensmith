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

func TestRenderModelsShowsDraftAndLive(t *testing.T) {
	m := testModel(t)
	m.state.Models = []model.Model{
		{Gen: 1, Segment: model.SegConsumer, Online: false, Users: 0, Quality: [model.NumQualityDims]float64{25, 0, 0, 0}},
		{Gen: 1, Name: "Nova", Online: true, Users: 500, Price: 12, Segment: model.SegConsumer},
	}
	m.page = PageModels
	v := renderModels(m)
	if !strings.Contains(v, "待發佈") || !strings.Contains(v, "營運中") {
		t.Fatalf("missing sections: %s", v)
	}
	if !strings.Contains(v, "Nova") {
		t.Fatalf("missing live name: %s", v)
	}
}

func TestRenderModelsDetailContents(t *testing.T) {
	m := testModel(t)
	m.state.Models = []model.Model{
		{Gen: 1, Segment: model.SegConsumer, Online: false, Users: 0, Quality: [model.NumQualityDims]float64{25, 0, 0, 0}},
	}
	m.page = PageModels
	m.modelCursor = 0
	v := renderModels(m)
	if !strings.Contains(v, "待發佈草稿") {
		t.Errorf("expected draft details to contain '待發佈草稿', got:\n%s", v)
	}

	m.state.Models[0].Online = true
	m.state.Models[0].Users = 100
	v = renderModels(m)
	if !strings.Contains(v, "用戶數") {
		t.Errorf("expected online details to contain '用戶數', got:\n%s", v)
	}
}

func TestModelShowsObsolescence(t *testing.T) {
	m := testModel(t)
	// Stored absolute quality fixed at 25; industry frontier will rise over time.
	m.state.Models = []model.Model{{
		Gen: 1, Online: true, Users: 10, Price: 12,
		Quality: [model.NumQualityDims]float64{25, 5, 5, 5},
	}}
	m.modelCursor = 0
	m.page = PageModels
	beforeAbs := m.state.Models[0].Quality
	v1 := renderModels(m)
	if !strings.Contains(v1, "25") || !strings.Contains(v1, "能力") {
		t.Fatalf("absolute quality missing:\n%s", v1)
	}
	if !strings.Contains(v1, "相對前沿") && !strings.Contains(v1, "落後") && !strings.Contains(v1, "前沿") {
		t.Fatalf("expected frontier relative copy:\n%s", v1)
	}
	if !strings.Contains(v1, "等效世代") || !strings.Contains(v1, "Gen1") {
		t.Fatalf("expected equivalent-gen line:\n%s", v1)
	}
	// Advance industry clock — stored quality must not change; relative text may.
	m.state.Progression.IndustryTime = 20000 * 86400
	v2 := renderModels(m)
	if m.state.Models[0].Quality != beforeAbs {
		t.Fatalf("stored quality mutated: %v → %v", beforeAbs, m.state.Models[0].Quality)
	}
	if !strings.Contains(v2, "25") {
		t.Fatalf("absolute 25 must remain visible after frontier moves:\n%s", v2)
	}
}
