package tui

import (
	"strings"
	"testing"

	"tokensmith/internal/model"
)

func TestInferencePressureShown(t *testing.T) {
	m := testModel(t)
	m.state.Models = []model.Model{{Online: true, Users: 1e6, Price: 12}}
	m.state.Compute.InferenceCapacity = 1 // tiny → overloaded
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

func TestResourceBarShowsPerRealSecondRnDRate(t *testing.T) {
	m := testModel(t) // fresh game seeds 2 T1 researchers
	bar := renderResourceBar(m)
	// 2 × 0.005/game-sec × 14400 game-sec/real-sec = 144/real-sec (not +0/s)
	if strings.Contains(bar, "+0/s") || !strings.Contains(bar, "144") {
		t.Fatalf("expected a non-zero per-real-second R&D rate:\n%s", bar)
	}
}

func TestResourceBarShowsDay(t *testing.T) {
	m := testModel(t)
	m.state.GameTime = 3 * 86400
	if !strings.Contains(renderResourceBar(m), "Day 3") {
		t.Fatalf("resource bar should show Day 3")
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
