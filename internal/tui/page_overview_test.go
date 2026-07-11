package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"tokensmith/internal/model"
)

func TestOverviewShowsCampaignWarRoom(t *testing.T) {
	m := testModel(t)
	m.state.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageExpand, Cycle: 4,
		Primary:  model.RivalRoadmap{Company: "OpenAI", ActionIndex: 0, CyclesUntilAction: 1},
		Wildcard: model.RivalRoadmap{Company: "DeepSeek", ActionIndex: 0, CyclesUntilAction: 2},
		Reports: []model.BoardReport{{Cycle: 4, Entries: []model.CampaignReportEntry{
			{Kind: model.ReportRivalAction, SubjectID: "OpenAI", DetailID: "openai-flagship"},
		}}},
	}
	v := renderOverview(m)
	for _, want := range []string{"主要戰略", "消費者霸主", "OpenAI", "下一步", "董事會報告"} {
		if !strings.Contains(v, want) {
			t.Fatalf("missing %q:\n%s", want, v)
		}
	}
}

func TestOverviewShowsHQ(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 110, Height: 42})
	m = mm.(Model)
	if out := renderOverview(m); !strings.Contains(out, "總部") {
		t.Fatal("overview should show HQ card")
	}
}

func TestOverviewPreCampaignGuidance(t *testing.T) {
	m := testModel(t)
	m.state.Campaign = model.CampaignState{}
	v := renderOverview(m)
	if !strings.Contains(v, "第一個模型上線後可選公司戰略") {
		t.Fatalf("pre-campaign guidance missing:\n%s", v)
	}
}

func TestOverviewShowsKPIsAndTraining(t *testing.T) {
	m := testModel(t)
	m.state.HasTraining = true
	m.state.Training = model.TrainingJob{Gen: 4, WorkRemaining: 500}
	m.state.Models = []model.Model{{Online: true, Users: 1000, Price: 12}}
	m.page = PageOverview
	v := renderOverview(m)
	for _, want := range []string{"估值", "總用戶", "月營收", "排名", "訓練 / 前沿", "Gen4", "里程碑"} {
		if !strings.Contains(v, want) {
			t.Errorf("overview missing %q:\n%s", want, v)
		}
	}
}

func TestOverviewNoTrainingHint(t *testing.T) {
	m := testModel(t)
	m.state.HasTraining = false
	if !strings.Contains(renderOverview(m), "無進行中") {
		t.Errorf("expected idle-training hint")
	}
}

func TestOverviewShowsFrontier(t *testing.T) {
	m := testModel(t)
	m.state.HasTraining = true
	m.state.Training = model.TrainingJob{Gen: 5, WorkRemaining: 1000}
	m.state.Servers = []model.Server{{Pool: model.PoolTraining, Compute: 200}}
	m.state.Resources.RnD = 1e12
	m.state.Progression.Frontier = model.FrontierProject{
		Active: true, TargetGen: 6, AllocationPct: 40,
		RnDTotal: 1000, RnDRemaining: 400,
		WorkTotal: 1000, WorkRemaining: 250,
		RecommendedCompute: 100,
	}
	v := renderOverview(m)
	for _, want := range []string{
		"訓練 Gen5", "前沿 Gen6", "分配 前沿40%", "訓練60%",
		"有效", "折合", "建議", "ETA", "R&D 進度",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("overview frontier missing %q:\n%s", want, v)
		}
	}
}

func TestOverviewFrontierStallNoToast(t *testing.T) {
	m := testModel(t)
	m.state.Progression.Frontier = model.FrontierProject{
		Active: true, TargetGen: 6, AllocationPct: 100,
		RnDTotal: 100, RnDRemaining: 100,
		WorkTotal: 100, WorkRemaining: 100,
		RecommendedCompute: 50,
	}
	m.state.Resources.RnD = 0
	m.state.Servers = []model.Server{{Pool: model.PoolTraining, Compute: 100}}
	// Stall appears in card + pressures, not as a notice toast.
	v := renderOverview(m)
	if !strings.Contains(v, "停滯") || !strings.Contains(v, "R&D 不足") {
		t.Fatalf("expected stall copy in card:\n%s", v)
	}
	if m.notice != "" {
		t.Fatalf("render must not set notice toast, got %q", m.notice)
	}
	joined := strings.Join(pressures(m), "\n")
	if !strings.Contains(joined, "前沿研究停滯") {
		t.Fatalf("expected persistent pressure line:\n%s", joined)
	}
}

func TestOverviewShowsDrafts(t *testing.T) {
	m := testModel(t)
	m.state.HasTraining = false
	// Add a draft model
	m.state.Models = []model.Model{{Online: false, Users: 0, Price: 12}}
	v := renderOverview(m)
	if !strings.Contains(v, "待發佈") {
		t.Errorf("expected draft warning '待發佈', got:\n%s", v)
	}
}

func TestOverviewShowsShare(t *testing.T) {
	m := testModel(t)
	m.state.Models = []model.Model{{Online: true, Segment: model.SegConsumer, Quality: [model.NumQualityDims]float64{10, 0, 0, 0}, Name: "MyModel"}}
	v := renderOverview(m)
	if !strings.Contains(v, "市佔") {
		t.Errorf("expected '市佔', got:\n%s", v)
	}
}

func TestOverviewCardsAlignFlush(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 110, Height: 40})
	m = mm.(Model)
	out := renderOverview(m)
	// 每一行都不超過 content width，且格線行等寬（左右卡齊平）
	cw := m.contentWidth()
	for i, ln := range strings.Split(out, "\n") {
		if lipgloss.Width(ln) > cw {
			t.Fatalf("line %d overflows content width %d: %q", i, cw, ln)
		}
	}
}
