package tui

import (
	"fmt"
	"strings"
	"testing"

	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

func TestInferencePressureShown(t *testing.T) {
	m := testModel(t)
	m.state.Models = []model.Model{{Online: true, Users: 1e6, Price: 12}}
	m.state.Compute.RentedInference = map[string]int{"N7": 1} // tiny → overloaded
	m.state.Compute.InferenceLoad = 100
	if !strings.Contains(strings.Join(pressures(m), "\n"), "推理") {
		t.Fatalf("expected inference pressure warning")
	}
}

func TestNoModelPressureShown(t *testing.T) {
	m := testModel(t)
	m.state.Models = nil
	m.state.HasTraining = false
	if !strings.Contains(strings.Join(pressures(m), "\n"), "模型") {
		t.Fatalf("expected no-online-model warning")
	}
}

func TestFinancialDistressPressureShown(t *testing.T) {
	// Distress>=1: red crisis line with strategy-exit guidance.
	m1 := testModel(t)
	m1.state.Campaign.FinancialDistressCycles = 1
	joined1 := strings.Join(pressures(m1), "\n")
	if !strings.Contains(joined1, "財務危機") {
		t.Fatalf("distress=1 should warn about finance:\n%s", joined1)
	}
	if !strings.Contains(joined1, "[E]") {
		t.Fatalf("distress=1 should mention [E]:\n%s", joined1)
	}

	m2 := testModel(t)
	m2.state.Campaign.FinancialDistressCycles = 2
	joined2 := strings.Join(pressures(m2), "\n")
	if !strings.Contains(joined2, "財務危機") {
		t.Fatalf("distress=2 should warn about finance:\n%s", joined2)
	}
	if !strings.Contains(joined2, "第 2 週期") {
		t.Fatalf("distress=2 should show cycle count:\n%s", joined2)
	}
}

func TestPendingPerkAndNoDoctrinePressures(t *testing.T) {
	// Online model + no doctrine → choose-strategy pressure.
	m := testModel(t)
	m.state.Models = []model.Model{{Online: true, Users: 10, Price: 12}}
	m.state.Campaign = model.CampaignState{}
	joined := strings.Join(pressures(m), "\n")
	if !strings.Contains(joined, "尚未選擇公司戰略") {
		t.Fatalf("expected no-doctrine pressure:\n%s", joined)
	}

	// Pending perk tier → choose-perk pressure.
	m2 := testModel(t)
	m2.state.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer, PerkTierPending: 1,
	}
	joined2 := strings.Join(pressures(m2), "\n")
	if !strings.Contains(joined2, "可選第 1 階路線能力") {
		t.Fatalf("expected pending-perk pressure:\n%s", joined2)
	}
}

func TestFrontierStallPressure(t *testing.T) {
	m := testModel(t)
	m.state.Progression.Frontier = model.FrontierProject{
		Active: true, TargetGen: 6, AllocationPct: 100,
		RnDTotal: 10, RnDRemaining: 10, WorkTotal: 10, WorkRemaining: 10,
		RecommendedCompute: 50,
	}
	m.state.Resources.RnD = 0
	m.state.Servers = []model.Server{{Pool: model.PoolTraining, Compute: 100}}
	joined := strings.Join(pressures(m), "\n")
	if !strings.Contains(joined, "前沿研究停滯") || !strings.Contains(joined, "R&D 不足") {
		t.Fatalf("expected frontier stall pressure:\n%s", joined)
	}
	// Paused allocation.
	m2 := testModel(t)
	m2.state.Progression.Frontier = model.FrontierProject{
		Active: true, TargetGen: 6, AllocationPct: 0,
		RnDTotal: 10, RnDRemaining: 10, WorkTotal: 10, WorkRemaining: 10,
		RecommendedCompute: 50,
	}
	m2.state.Resources.RnD = 1e9
	m2.state.Servers = []model.Server{{Pool: model.PoolTraining, Compute: 100}}
	j2 := strings.Join(pressures(m2), "\n")
	if !strings.Contains(j2, "暫停") && !strings.Contains(j2, "0%") {
		t.Fatalf("expected paused stall pressure:\n%s", j2)
	}
}

func TestOverviewHelpShowsCampaignKeys(t *testing.T) {
	m := testModel(t)
	m.page = PageOverview
	hint := pageKeys(m)
	for _, want := range []string{"[c]公司策略", "[d]高層指令", "[t]訓練", "[X]重來"} {
		if !strings.Contains(hint, want) {
			t.Fatalf("overview help missing %q: %q", want, hint)
		}
	}
	if strings.Contains(hint, "[P]勝利結算") {
		t.Fatalf("victory settle key must not show pre-victory: %q", hint)
	}
	if strings.Contains(hint, "[E]策略退出") {
		t.Fatalf("exit key must not show before unlock: %q", hint)
	}

	m.state.Campaign.Victory = model.DoctrineConsumer
	if !strings.Contains(pageKeys(m), "[P]勝利結算") {
		t.Fatalf("expected [P]勝利結算 after victory: %q", pageKeys(m))
	}

	m2 := testModel(t)
	m2.page = PageOverview
	m2.state.Campaign.Cycle = 18
	if !strings.Contains(pageKeys(m2), "[E]策略退出") {
		t.Fatalf("expected [E]策略退出 at cycle 18: %q", pageKeys(m2))
	}

	m3 := testModel(t)
	m3.page = PageOverview
	m3.state.Campaign.FinancialDistressCycles = 2
	if !strings.Contains(pageKeys(m3), "[E]策略退出") {
		t.Fatalf("expected [E]策略退出 after two distress cycles: %q", pageKeys(m3))
	}
}

func TestResourceBarShowsPerRealSecondRnDRate(t *testing.T) {
	m := testModel(t)
	// Employee-office: seed a researcher so R&D rate is non-zero.
	m.state.Employees = []model.Employee{{
		PrimaryRole: model.RoleResearcher,
		Stats:       [model.NumRoles]int{50, 0, 0, 0},
	}}
	m.state.Research.EfficiencyMult = 1
	bar := renderResourceBar(m)
	want := human(sim.RnDRatePerSec(m.state, m.cfg) * gameSecPerRealSec)
	if !strings.Contains(bar, fmt.Sprintf("+%s/s", want)) {
		t.Fatalf("expected per-real-second R&D rate +%s/s:\n%s", want, bar)
	}
}

func TestViewShowsDay(t *testing.T) {
	m := testModel(t)
	m.state.GameTime = 3 * 86400
	if !strings.Contains(m.View(), "Day 3") {
		t.Fatalf("View should show Day 3")
	}
}

func TestPrestigeKeyResetsWhenUnlocked(t *testing.T) {
	m := testModel(t)
	m.page = PageOverview
	m.state.PeakValuation = 2e9 // above PrestigeUnlockValuation 1e9
	m.state.Models = []model.Model{{Online: true, Users: 5, Price: 12}}
	nmAny, _ := m.Update(key("P"))
	nm := nmAny.(Model)
	if nm.state.Prestige.Patents <= 0 {
		t.Fatalf("P should bank patents on prestige, got %v", nm.state.Prestige.Patents)
	}
	if len(nm.state.Models) != 0 {
		t.Fatalf("prestige reset should clear models")
	}
}
