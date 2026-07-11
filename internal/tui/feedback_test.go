// internal/tui/feedback_test.go
package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestDetectTrainingCompleted(t *testing.T) {
	cfg := balance.Default()
	prev := model.GameState{HasTraining: true}
	next := model.GameState{Models: []model.Model{{Gen: 2}}}
	got := detectMoments(prev, next, cfg)
	if len(got) != 1 || got[0].Level != LevelMajor || !strings.Contains(got[0].Text, "Gen2") {
		t.Fatalf("want one Gen2 major moment, got %+v", got)
	}
}

func TestDetectMilestone(t *testing.T) {
	cfg := balance.Default()
	prev := model.GameState{MilestonesReached: 0}
	next := model.GameState{MilestonesReached: 1}
	got := detectMoments(prev, next, cfg)
	if len(got) != 1 || !strings.Contains(got[0].Text, "里程碑") {
		t.Fatalf("want milestone moment, got %+v", got)
	}
}

func TestDetectCampaignReports(t *testing.T) {
	cfg := balance.Default()
	prev := model.GameState{}
	next := model.GameState{}
	next.Campaign.Reports = []model.BoardReport{{
		Cycle: 1,
		Entries: []model.CampaignReportEntry{
			{Kind: model.ReportShowdown, SubjectID: "showdown"},
			{Kind: model.ReportVictory, SubjectID: string(model.DoctrineConsumer)},
		},
	}}
	got := detectMoments(prev, next, cfg)
	if len(got) != 2 {
		t.Fatalf("want 2 moments, got %+v", got)
	}
	if got[1].Level != LevelEpic {
		t.Fatalf("victory must be Epic, got %+v", got[1])
	}
}

func TestNewReportEntriesSameCycleGrowth(t *testing.T) {
	prev := model.GameState{}
	prev.Campaign.Reports = []model.BoardReport{{
		Cycle:   3,
		Entries: []model.CampaignReportEntry{{Kind: model.ReportRivalAction}},
	}}
	next := model.GameState{}
	next.Campaign.Reports = []model.BoardReport{{
		Cycle: 3,
		Entries: []model.CampaignReportEntry{
			{Kind: model.ReportRivalAction},
			{Kind: model.ReportFinancialRisk},
		},
	}}
	got := newReportEntries(prev, next)
	if len(got) != 1 || got[0].Kind != model.ReportFinancialRisk {
		t.Fatalf("want only the appended entry, got %+v", got)
	}
}

func TestDetectNothingOnNoChange(t *testing.T) {
	cfg := balance.Default()
	s := model.GameState{MilestonesReached: 2}
	if got := detectMoments(s, s, cfg); len(got) != 0 {
		t.Fatalf("no change should yield no moments, got %+v", got)
	}
}

func TestDetectCounteredRivalAction(t *testing.T) {
	cfg := balance.Default()
	prev := model.GameState{}
	next := model.GameState{}
	next.Campaign.Reports = []model.BoardReport{{
		Cycle: 2,
		Entries: []model.CampaignReportEntry{
			{Kind: model.ReportRivalAction, SubjectID: "OpenAI", DetailID: "openai-flagship", Countered: true},
		},
	}}
	got := detectMoments(prev, next, cfg)
	if len(got) != 1 || !strings.Contains(got[0].Text, "反制奏效") {
		t.Fatalf("countered action should celebrate the counter, got %+v", got)
	}
}

func TestPushBannerCapsQueue(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	for i := 0; i < 10; i++ {
		m.pushBanner(Moment{LevelMajor, fmt.Sprintf("b%d", i)})
	}
	if len(m.banners) != maxBanners {
		t.Fatalf("queue len = %d, want %d", len(m.banners), maxBanners)
	}
	if m.banners[0].Text != "b2" {
		t.Fatalf("oldest should be dropped, head = %q", m.banners[0].Text)
	}
}

func TestBannerFadesAfterTicks(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	m.pushBanner(Moment{LevelMajor, "hello"})
	for i := 0; i < bannerShowTicks; i++ {
		m.advanceDisplay()
	}
	if len(m.banners) != 0 {
		t.Fatalf("banner should fade after %d ticks, still %d queued", bannerShowTicks, len(m.banners))
	}
}

func TestViewShowsBanner(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	m.pushBanner(Moment{LevelMajor, "🏁 里程碑達成"})
	if out := m.View(); !strings.Contains(out, "里程碑達成") {
		t.Fatalf("View should show banner: %q", out)
	}
}
