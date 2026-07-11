package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"tokensmith/internal/model"
)

func TestMarketPageShowsSegmentsAndRivals(t *testing.T) {
	m := testModel(t)
	m.page = PageMarket
	v := renderMarket(m)
	for _, w := range []string{"消費者", "企業", "開發者", "對手", "你的用戶", "市場規模", "威脅"} {
		if !strings.Contains(v, w) {
			t.Errorf("market page missing %q:\n%s", w, v)
		}
	}
}

func TestMarketSegmentUsers(t *testing.T) {
	m := testModel(t)
	m.state.Models = []model.Model{
		{Online: true, Segment: model.SegConsumer, Users: 1500},
		{Online: true, Segment: model.SegEnterprise, Users: 42},
	}
	if segmentUsers(m.state, model.SegConsumer) != 1500 {
		t.Fatalf("consumer users = %v, want 1500", segmentUsers(m.state, model.SegConsumer))
	}
	if segmentUsers(m.state, model.SegDeveloper) != 0 {
		t.Fatalf("developer users = %v, want 0", segmentUsers(m.state, model.SegDeveloper))
	}
	// consumer(1000) is the biggest scale → 大; enterprise(500) smallest → 小
	if marketSizeLabel(m.cfg, model.SegConsumer) != "大" || marketSizeLabel(m.cfg, model.SegEnterprise) != "小" {
		t.Fatalf("market size labels wrong")
	}
}

func TestRankArrow(t *testing.T) {
	if rankArrow(0, 3) != "" {
		t.Fatal("no history → no arrow")
	}
	if got := rankArrow(5, 3); !strings.Contains(got, "↑2") {
		t.Fatalf("rank 5→3 should be ↑2: %q", got)
	}
	if got := rankArrow(3, 5); !strings.Contains(got, "↓2") {
		t.Fatalf("rank 3→5 should be ↓2: %q", got)
	}
	if rankArrow(4, 4) != "" {
		t.Fatal("same rank → no arrow")
	}
}

func TestMarketHighlightsYouRow(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 110, Height: 40})
	m = mm.(Model)
	out := renderMarket(m)
	if !strings.Contains(out, "你") {
		t.Fatalf("market should contain your row: %q", out)
	}
}

func TestMarketShowsFrontier(t *testing.T) {
	m := testModel(t)
	m.page = PageMarket
	m.state.Models = []model.Model{{
		Gen: 1, Online: true, Users: 100, Price: 12,
		Quality: [model.NumQualityDims]float64{100, 100, 100, 100},
	}}
	m.state.Competitors = []model.Competitor{{
		Name:           "OpenAI",
		Quality:        [model.NumQualityDims]float64{110, 90, 100, 95},
		Skill:          [model.NumQualityDims]float64{1.08, 1.00, 0.96, 1.04},
		MomentumCycles: 3,
	}}
	m.state.Progression.Rivals = model.RivalEraState{Era: 3, Leaders: []string{"OpenAI"}}
	m.state.Campaign.Active = []model.CampaignModifier{{
		ID: "rival-deepseek-price-war-1", CyclesRemaining: 2,
		Effects: func() model.CampaignEffects {
			e := model.NeutralCampaignEffects()
			e.RefPriceMult[model.SegDeveloper] = 0.85
			return e
		}(),
	}}
	v := renderMarket(m)
	for _, want := range []string{
		"全球前沿", "時代", "85%", "115%",
		"OpenAI", "領袖", "專長", "威脅",
		"市況修正", "剩餘 2 週期", "開發者價",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("market frontier missing %q:\n%s", want, v)
		}
	}
	// Ranking surface still present (unchanged appeal path).
	if !strings.Contains(v, "排名") {
		t.Fatalf("rank display missing:\n%s", v)
	}
}
