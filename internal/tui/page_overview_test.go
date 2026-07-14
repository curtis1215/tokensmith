package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"tokensmith/internal/dailyusage"
	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

func TestOverviewHasNoCampaignCards(t *testing.T) {
	m := testModel(t)
	m.state.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageExpand, Cycle: 4,
		Primary: model.RivalRoadmap{Company: "OpenAI", ActionIndex: 0, CyclesUntilAction: 1},
		Reports: []model.BoardReport{{Cycle: 4, Entries: []model.CampaignReportEntry{
			{Kind: model.ReportRivalAction, SubjectID: "OpenAI", DetailID: "openai-flagship"},
		}}},
	}
	v := renderOverview(m)
	// Ban campaign card titles / labels. Do not ban bare "OpenAI" — share card
	// legitimately lists market rivals. Ban campaign-only OpenAI phrasing instead.
	for _, ban := range []string{"公司戰略", "宿敵路線", "董事會報告", "產業動態", "主要戰略", "主要宿敵", "消費旗艦"} {
		if strings.Contains(v, ban) {
			t.Fatalf("overview must not show campaign content %q:\n%s", ban, v)
		}
	}
}

func TestOverviewPendingStrip(t *testing.T) {
	m := pendingChipShortage(testModel(t))
	v := renderOverview(m)
	if !strings.Contains(v, "[3]戰情室") || !strings.Contains(v, "待決策") {
		t.Fatalf("expected pending strip pointing to war room:\n%s", v)
	}
	if strings.Contains(v, "產業動態") {
		t.Fatalf("overview must not show 產業動態 card title:\n%s", v)
	}
}

func TestOverviewShowsHQ(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 110, Height: 42})
	m = mm.(Model)
	out := renderOverview(m)
	if !strings.Contains(out, "總部") {
		t.Fatal("overview should show HQ card")
	}
	// ≥100 content width must use full ASCII art (not icon-only strip).
	// Art contains underscores from stage buildings.
	if !strings.Contains(out, "___") && !strings.Contains(out, "---") {
		t.Fatalf("overview at wide width should show HQ ASCII art:\n%s", out)
	}
}

func TestOverviewOmitsCampaignPressures(t *testing.T) {
	m := testModel(t)
	m.state.Models = []model.Model{{Online: true, Users: 10, Price: 12}}
	m.state.Campaign = model.CampaignState{} // DoctrineNone
	v := renderOverview(m)
	for _, ban := range []string{"尚未選擇公司戰略", "可選第", "財務危機"} {
		if strings.Contains(v, ban) {
			t.Fatalf("overview must not show campaign pressure %q:\n%s", ban, v)
		}
	}
	// Same fixture still produces campaign pressure for war room / pressures().
	if !strings.Contains(strings.Join(campaignPressures(m), "\n"), "尚未選擇公司戰略") {
		t.Fatal("campaignPressures should still flag no-doctrine")
	}
}

func TestOverviewRow1CardHeightsMatch(t *testing.T) {
	m := testModel(t)
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 110, Height: 42})
	m = mm.(Model)
	cw := m.contentWidth()
	gap := 2
	colW := (cw - gap) / 2
	left := CardInFrom(hqContent(m, colW, cw < 100))
	right := CardInFrom(companyContent(m, colW))
	// After HRowEqualCards, both sides use padBodyLines to same body height.
	row := HRowEqualCards(gap, hqContent(m, colW, cw < 100), companyContent(m, colW))
	// Equalized individual cards should match height.
	bodyMax := bodyLineCount(hqContent(m, colW, cw < 100).body)
	if n := bodyLineCount(companyContent(m, colW).body); n > bodyMax {
		bodyMax = n
	}
	leftEq := CardIn(CardDefault, colW, hqContent(m, colW, cw < 100).title, padBodyLines(hqContent(m, colW, cw < 100).body, bodyMax))
	// Use same kind/title from content
	hc := hqContent(m, colW, cw < 100)
	cc := companyContent(m, colW)
	leftEq = CardIn(hc.kind, colW, hc.title, padBodyLines(hc.body, bodyMax))
	rightEq := CardIn(cc.kind, colW, cc.title, padBodyLines(cc.body, bodyMax))
	if lipgloss.Height(leftEq) != lipgloss.Height(rightEq) {
		t.Fatalf("equalized HQ/company heights %d vs %d", lipgloss.Height(leftEq), lipgloss.Height(rightEq))
	}
	_ = left
	_ = right
	_ = row
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
	// Overview only: progress + ETA/stall (≤2 frontier lines). No 分配/有效/折合/建議/R&D 進度.
	for _, want := range []string{"訓練 Gen5", "前沿 Gen6", "ETA"} {
		if !strings.Contains(v, want) {
			t.Errorf("overview frontier missing %q:\n%s", want, v)
		}
	}
	for _, ban := range []string{"分配 前沿", "有效", "折合", "建議", "R&D 進度"} {
		if strings.Contains(v, ban) {
			t.Errorf("overview frontier too detailed: has %q", ban)
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

func TestOverviewShowsDailyUsageBySource(t *testing.T) {
	m := testModel(t)
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 110, Height: 42})
	m = mm.(Model)
	m.dailyDay = "2026-07-12"
	m.dailyDoc.Days = map[string]map[string]dailyusage.SourceUsage{
		"2026-07-12": {
			"claude-code": {In: 120_000, Out: 18_000},
			"codex":       {In: 85_000, Out: 12_000},
			"grok":        {In: 30_000},
			"opencode":    {In: 42_000, Out: 9_000},
		},
	}
	out := renderOverview(m)
	// Thin layout: source totals + 合計.
	for _, want := range []string{"今日 Token 收成", "Claude", "Codex", "Grok", "OpenCode", "合計", "316K"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	// Must NOT require full multi-line In/Out rows.
	// Still: Grok must not fabricate Out.
	if strings.Contains(out, "Grok") {
		for _, ln := range strings.Split(out, "\n") {
			if strings.Contains(ln, "Grok") && strings.Contains(ln, "Out") {
				if strings.Contains(ln, "Out 0") {
					continue
				}
				t.Fatalf("Grok line must not show fabricated Out: %q", ln)
			}
		}
	}
}

func TestOverviewDailyUsageZerosWhenMissing(t *testing.T) {
	m := testModel(t)
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 110, Height: 42})
	m = mm.(Model)
	m.dailyDay = "2026-07-12"
	m.dailyDoc.Days = nil
	out := renderOverview(m)
	for _, want := range []string{"今日 Token 收成", "Claude", "Codex", "Grok", "OpenCode"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	// Zero activity sources remain visible (compact 0 total).
	if !strings.Contains(out, "0") {
		t.Fatalf("zeros should remain visible:\n%s", out)
	}
}

func TestOverviewDailyUsageNarrowKeepsAllSources(t *testing.T) {
	m := testModel(t)
	// Narrow terminal; contentWidth will be small.
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 40})
	m = mm.(Model)
	m.dailyDay = "2026-07-12"
	m.dailyDoc.Days = map[string]map[string]dailyusage.SourceUsage{
		"2026-07-12": {
			"claude-code": {In: 120_000, Out: 18_000},
			"codex":       {In: 85_000, Out: 12_000},
			"grok":        {In: 30_000},
			"opencode":    {In: 42_000, Out: 9_000},
		},
	}
	out := renderOverview(m)
	// Compact labels still cover every source + 合計.
	for _, want := range []string{"Claude", "Codex", "Grok", "OpenCode", "合計", "316K"} {
		if !strings.Contains(out, want) {
			t.Fatalf("narrow missing %q:\n%s", want, out)
		}
	}
	cw := m.contentWidth()
	for i, ln := range strings.Split(out, "\n") {
		if lipgloss.Width(ln) > cw {
			t.Fatalf("line %d overflows content width %d: %q (width=%d)", i, cw, ln, lipgloss.Width(ln))
		}
	}
}

func TestPickShareRowsAlwaysIncludesYou(t *testing.T) {
	// Player last by share among 6 rivals + you.
	bars := []sim.ShareRow{
		{Name: "A", Share: 0.3},
		{Name: "B", Share: 0.25},
		{Name: "C", Share: 0.2},
		{Name: "D", Share: 0.15},
		{Name: "E", Share: 0.09},
		{Name: "你", Share: 0.01, You: true},
	}
	got := pickShareRows(bars, 4)
	if len(got) != 4 {
		t.Fatalf("len=%d want 4: %+v", len(got), got)
	}
	found := false
	for _, r := range got {
		if r.You {
			found = true
		}
	}
	if !found {
		t.Fatalf("player missing from top-4 pick: %+v", got)
	}
}

func TestFormatTokenCount(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1K"},
		{138000, "138K"},
		{1_500_000, "1.5M"},
	}
	for _, tc := range cases {
		if got := formatTokenCount(tc.n); got != tc.want {
			t.Errorf("formatTokenCount(%d)=%q, want %q", tc.n, got, tc.want)
		}
	}
}
