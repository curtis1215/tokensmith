package store

import (
	"os"
	"path/filepath"
	"testing"

	"tokensmith/internal/model"
)

func TestMetaRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "meta.json")
	if _, ok, _ := LoadMeta(p); ok {
		t.Fatal("missing meta should be ok=false")
	}
	in := Meta{
		ConsumedSources: map[string]model.SourceTotals{
			"claude-code": {In: 10, Out: 5},
		},
		LastRealUnix:      42,
		LastActiveDate:    "2026-07-10",
		StreakDays:        3,
		LastCampaignUnix:  84,
		LastCampaignCycle: 7,
	}
	if err := SaveMeta(p, in); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadMeta(p)
	if err != nil || !ok {
		t.Fatalf("meta round-trip: ok=%v err=%v", ok, err)
	}
	if got.ConsumedSources["claude-code"] != (model.SourceTotals{In: 10, Out: 5}) {
		t.Fatalf("ConsumedSources not round-tripped: %+v", got.ConsumedSources)
	}
	if got.LastRealUnix != 42 || got.LastActiveDate != "2026-07-10" || got.StreakDays != 3 {
		t.Fatalf("meta round-trip mismatch: %+v", got)
	}
	if got.LastCampaignUnix != 84 {
		t.Fatalf("LastCampaignUnix not round-tripped: %d", got.LastCampaignUnix)
	}
	if got.LastCampaignCycle != 7 {
		t.Fatalf("LastCampaignCycle not round-tripped: %d", got.LastCampaignCycle)
	}
}

func TestMetaAchievementsRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.json")
	in := Meta{Achievements: map[string]int64{"first-online": 1700000000}}
	if err := SaveMeta(path, in); err != nil {
		t.Fatal(err)
	}
	out, ok, err := LoadMeta(path)
	if err != nil || !ok {
		t.Fatalf("load failed: %v ok=%v", err, ok)
	}
	if out.Achievements["first-online"] != 1700000000 {
		t.Fatalf("achievements lost in roundtrip: %+v", out.Achievements)
	}
}

func TestMetaOldFileNilAchievements(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.json")
	if err := os.WriteFile(path, []byte(`{"streakDays":3}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, ok, _ := LoadMeta(path)
	if !ok || out.Achievements != nil {
		t.Fatalf("old meta should load with nil achievements, got %+v", out.Achievements)
	}
}
