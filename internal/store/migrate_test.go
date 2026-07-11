package store

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestMigrateV0InfersMaxGenTrainingAndIndustryTime(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		GameTime:     5000,
		UnlockedTech: []string{balance.GenUnlockNodeID(2), balance.GenUnlockNodeID(3)},
		Models:       []model.Model{{Gen: 2, Online: true, Quality: q(10, 10, 10, 10)}},
		HasTraining:  true,
		Training:     model.TrainingJob{Gen: 3, WorkRemaining: 100},
		Competitors:  balance.DefaultCompetitors(),
	}
	// Exploded OpenAI-like quality on capability.
	s.Competitors[0].Quality[model.DimCapability] = 135185
	s.Competitors[0].Skill[model.DimCapability] = 1.25 // needs clamp; strongest dim
	s.Competitors[0].MomentumPct[model.DimCapability] = 0.5
	s.Competitors[0].MomentumCycles = 9

	ns, err := migrateV0(s, b)
	if err != nil {
		t.Fatal(err)
	}
	if ns.Progression.MaxUnlockedGen != 3 {
		t.Fatalf("MaxUnlockedGen = %d, want 3 (tech+training)", ns.Progression.MaxUnlockedGen)
	}
	if ns.Progression.IndustryTime != 5000 {
		t.Fatalf("IndustryTime = %v, want GameTime 5000", ns.Progression.IndustryTime)
	}
	if ns.Competitors[0].MomentumCycles != 0 || ns.Competitors[0].MomentumPct[model.DimCapability] != 0 {
		t.Fatalf("momentum not cleared: %+v", ns.Competitors[0])
	}
	// Skill clamp preserves capability as strongest.
	sk := ns.Competitors[0].Skill
	if sk[model.DimCapability] < skillFloor || sk[model.DimCapability] > skillCeil {
		t.Fatalf("skill cap out of band: %v", sk[model.DimCapability])
	}
	for d := range sk {
		if d == int(model.DimCapability) {
			continue
		}
		if sk[d] > sk[model.DimCapability]+1e-12 {
			t.Fatalf("strongest dim not preserved: skill=%v", sk)
		}
	}
	// Leaders initialized.
	if ns.Progression.Rivals.Era < 1 || len(ns.Progression.Rivals.Leaders) == 0 {
		t.Fatalf("rivals era state not initialized: %+v", ns.Progression.Rivals)
	}
	// Era rows for reached procedural eras (Gen3 → era 2; only III+ seeded).
	// Gen3 is era II — no procedural eras required. With max gen 5+ would seed.
}

func TestMigrateV0RankMapping(t *testing.T) {
	b := balance.Default()
	// Fixed player frontier so GlobalFrontier is player-dominated on every dim.
	s := model.GameState{
		GameTime: 0,
		Models:   []model.Model{{Gen: 1, Online: true, Quality: q(100, 100, 100, 100)}},
		Competitors: []model.Competitor{
			{Name: "A", Quality: q(1, 70, 1, 1), Skill: q(1, 1, 1, 1)},
			{Name: "B", Quality: q(2, 60, 1, 1), Skill: q(1, 1, 1, 1)},
			{Name: "C", Quality: q(3, 50, 1, 1), Skill: q(1, 1, 1, 1)},
			{Name: "D", Quality: q(4, 40, 1, 1), Skill: q(1, 1, 1, 1)},
			{Name: "E", Quality: q(5, 30, 1, 1), Skill: q(1, 1, 1, 1)},
			{Name: "F", Quality: q(6, 20, 1, 1), Skill: q(1, 1, 1, 1)},
			{Name: "G", Quality: q(7, 10, 1, 1), Skill: q(1, 1, 1, 1)},
		},
	}
	ns, err := migrateV0(s, b)
	if err != nil {
		t.Fatal(err)
	}
	// Capability ranks by original quality 1..7 → pct ladder.
	wantPct := []float64{0.88, 0.92, 0.96, 1.00, 1.04, 1.09, 1.14}
	byName := map[string]model.Competitor{}
	for _, c := range ns.Competitors {
		byName[c.Name] = c
	}
	// A lowest (1) → 88%, G highest (7) → 114% of frontier (~100).
	names := []string{"A", "B", "C", "D", "E", "F", "G"}
	for i, name := range names {
		got := byName[name].Quality[model.DimCapability]
		want := 100 * wantPct[i]
		if math.Abs(got-want) > 0.5 { // allow tiny frontier blend from time
			// Global frontier may be exactly 100 from player.
			if math.Abs(got-want) > 1 {
				t.Fatalf("%s cap = %v, want ~%v", name, got, want)
			}
		}
	}
	// Efficiency ranks reverse order (G lowest … A highest).
	effNames := []string{"G", "F", "E", "D", "C", "B", "A"}
	for i, name := range effNames {
		got := byName[name].Quality[model.DimEfficiency]
		want := 100 * wantPct[i]
		if math.Abs(got-want) > 1 {
			t.Fatalf("%s eff = %v, want ~%v", name, got, want)
		}
	}
}

func TestMigrationBackup(t *testing.T) {
	b := balance.Default()
	path := filepath.Join(t.TempDir(), "save.json")
	legacy := model.GameState{Resources: model.Resources{Cash: 50, RnD: 10}, GameTime: 99}
	raw, _ := json.Marshal(legacy)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	_, ok, err := LoadWithConfig(path, b)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	bak := path + ".v0.bak"
	data, err := os.ReadFile(bak)
	if err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	if string(data) != string(raw) {
		t.Fatalf("backup contents differ from original")
	}
	// Second load must not overwrite backup (still original bytes).
	// Mutate backup marker would be wrong — write a sentinel into bak then reload.
	// Instead: ensure bak mtime/content stable after second Load of already-migrated file.
	// First load rewrote save as envelope; second load is schema 1 (no re-backup).
	before, _ := os.ReadFile(bak)
	_, _, err = LoadWithConfig(path, b)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(bak)
	if string(before) != string(after) {
		t.Fatal("existing v0 backup was overwritten")
	}
}

func TestMigrationIdempotent(t *testing.T) {
	b := balance.Default()
	path := filepath.Join(t.TempDir(), "save.json")
	legacy := model.GameState{
		Resources:   model.Resources{Cash: 1, RnD: 2},
		GameTime:    1000,
		Models:      []model.Model{{Gen: 1, Online: true, Quality: q(25, 0, 0, 0)}},
		Competitors: balance.DefaultCompetitors(),
	}
	raw, _ := json.Marshal(legacy)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	a, ok, err := LoadWithConfig(path, b)
	if err != nil || !ok {
		t.Fatalf("first load: %v", err)
	}
	// Second load of migrated envelope must not alter state.
	c, ok, err := LoadWithConfig(path, b)
	if err != nil || !ok {
		t.Fatalf("second load: %v", err)
	}
	if a.Progression.MaxUnlockedGen != c.Progression.MaxUnlockedGen ||
		a.Progression.IndustryTime != c.Progression.IndustryTime ||
		a.Resources.Cash != c.Resources.Cash {
		t.Fatalf("not idempotent: first=%+v second=%+v", a.Progression, c.Progression)
	}
	// migrateV0 on already-migrated state with same gen/time keeps max gen & industry.
	m2, err := migrateV0(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if m2.Progression.MaxUnlockedGen != a.Progression.MaxUnlockedGen {
		t.Fatalf("re-migrate max gen %d → %d", a.Progression.MaxUnlockedGen, m2.Progression.MaxUnlockedGen)
	}
	if m2.Progression.IndustryTime != a.Progression.IndustryTime {
		t.Fatalf("re-migrate industry time changed")
	}
}

func TestMigrationFailurePreservesOriginal(t *testing.T) {
	b := balance.Default()
	path := filepath.Join(t.TempDir(), "save.json")
	// migrateV0 does not flip negative RnD → validate fails → original preserved.
	raw := []byte(`{"Resources":{"Cash":10,"RnD":-5},"GameTime":1}`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	_, ok, err := LoadWithConfig(path, b)
	if err == nil || ok {
		t.Fatalf("expected validation failure, ok=%v err=%v", ok, err)
	}
	onDisk, err2 := os.ReadFile(path)
	if err2 != nil {
		t.Fatal(err2)
	}
	if string(onDisk) != string(raw) {
		t.Fatalf("original rewritten on failure:\n got %s\nwant %s", onDisk, raw)
	}
}

func TestMigrateV0ActiveTrainingRaisesMaxGen(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		HasTraining: true,
		Training:    model.TrainingJob{Gen: 4, WorkRemaining: 50},
	}
	ns, err := migrateV0(s, b)
	if err != nil {
		t.Fatal(err)
	}
	if ns.Progression.MaxUnlockedGen != 4 {
		t.Fatalf("MaxUnlockedGen = %d, want 4 from training", ns.Progression.MaxUnlockedGen)
	}
}

func q(a, b, c, d float64) [model.NumQualityDims]float64 {
	return [model.NumQualityDims]float64{a, b, c, d}
}
