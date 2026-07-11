package tui

import (
	"path/filepath"
	"testing"

	"tokensmith/internal/model"
)

func TestAchievementCatalogWellFormed(t *testing.T) {
	if len(achievementCatalog) < 25 {
		t.Fatalf("catalog too small: %d", len(achievementCatalog))
	}
	seen := map[string]bool{}
	for _, a := range achievementCatalog {
		if a.ID == "" || a.Name == "" || a.Desc == "" || a.Check == nil {
			t.Fatalf("malformed achievement: %+v", a)
		}
		if seen[a.ID] {
			t.Fatalf("duplicate id %q", a.ID)
		}
		seen[a.ID] = true
	}
}

func TestAchievementChecksOnFreshGame(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	for _, a := range achievementCatalog {
		if a.Check(m) {
			t.Fatalf("fresh game should unlock nothing, but %q fired", a.ID)
		}
	}
}

func TestAchievementChecksFire(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	m.state.Models = []model.Model{{Gen: 2, Online: true}}
	m.state.MilestonesReached = 4
	m.streakDays = 7
	m.state.Campaign.Doctrine = model.DoctrineConsumer
	m.state.Prestige.Patents = 12
	fired := map[string]bool{}
	for _, a := range achievementCatalog {
		if a.Check(m) {
			fired[a.ID] = true
		}
	}
	for _, want := range []string{"first-online", "gen-2", "ms-1m", "ms-1b", "streak-3", "streak-7", "doctrine-chosen", "prestige-first", "patents-10"} {
		if !fired[want] {
			t.Fatalf("expected %q to fire, fired set: %v", want, fired)
		}
	}
	if fired["gen-5"] || fired["streak-10"] || fired["ms-1t"] {
		t.Fatalf("over-firing: %v", fired)
	}
}
