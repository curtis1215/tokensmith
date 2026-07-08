package tui

import (
	"strings"
	"testing"

	"tokensmith/internal/model"
)

func TestMarketPageShowsSegmentsAndRivals(t *testing.T) {
	m := testModel(t)
	m.page = PageMarket
	v := renderMarket(m)
	for _, w := range []string{"消費者", "企業", "開發者", "對手", "你的用戶", "市場規模"} {
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
