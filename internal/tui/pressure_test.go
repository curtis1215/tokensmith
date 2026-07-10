package tui

import (
	"strings"
	"testing"

	"tokensmith/internal/model"
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
	m := testModel(t)
	m.state.Campaign.FinancialDistressCycles = 2
	joined := strings.Join(pressures(m), "\n")
	if !strings.Contains(joined, "財務") {
		t.Fatalf("expected financial distress pressure:\n%s", joined)
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
	m := testModel(t) // fresh game seeds 2 T1 researchers
	bar := renderResourceBar(m)
	// 2 × (0.005/14400 game-sec) × 14400 game-sec/real-sec = 0.01/real-sec exactly —
	// small on purpose (root-cause fix: passive income no longer secretly
	// inherits the 14400x sim-time compression). human() shows sub-1 values to
	// 2dp so this doesn't misleadingly render as "+0/s".
	if !strings.Contains(bar, "+0.01/s") {
		t.Fatalf("expected the un-inflated per-real-second R&D rate:\n%s", bar)
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
