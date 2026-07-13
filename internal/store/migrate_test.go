package store

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
	"tokensmith/internal/sim"
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

func TestValidateTrainingNegativeCashBonusRejected(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		Progression: model.ProgressionState{MaxUnlockedGen: 1},
		HasTraining: true,
		Training: model.TrainingJob{
			Gen: 1, WorkRemaining: 10,
			CashBonus: [model.NumQualityDims]float64{-1, 0, 0, 0},
		},
	}
	if err := validateState(&s, b); err == nil {
		t.Fatal("expected error for negative CashBonus")
	}
}

func TestValidateTrainingNegativeBoostCashPaidRejected(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		Progression: model.ProgressionState{MaxUnlockedGen: 1},
		HasTraining: true,
		Training: model.TrainingJob{
			Gen: 1, WorkRemaining: 10,
			BoostCashPaid: -50,
		},
	}
	if err := validateState(&s, b); err == nil {
		t.Fatal("expected error for negative BoostCashPaid")
	}
}

func TestValidateTrainingNonFiniteBoostFieldsRejected(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		Progression: model.ProgressionState{MaxUnlockedGen: 1},
		HasTraining: true,
		Training: model.TrainingJob{
			Gen: 1, WorkRemaining: 10,
			CashBonus: [model.NumQualityDims]float64{math.NaN(), 0, 0, 0},
		},
	}
	if err := validateState(&s, b); err == nil {
		t.Fatal("expected error for NaN CashBonus")
	}
	s.Training.CashBonus = [model.NumQualityDims]float64{}
	s.Training.BoostCashPaid = math.Inf(1)
	if err := validateState(&s, b); err == nil {
		t.Fatal("expected error for Inf BoostCashPaid")
	}
}

func TestValidateTrainingRepairsOrphanCashBonus(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		Progression: model.ProgressionState{MaxUnlockedGen: 1},
		HasTraining: true,
		Training: model.TrainingJob{
			Gen: 1, WorkRemaining: 10,
			Boosts:        [model.NumQualityDims]bool{false, true, false, false},
			CashBonus:     [model.NumQualityDims]float64{5, 3.75, 2, 0},
			BoostCashPaid: 999, // historical charge; must not be recomputed
		},
	}
	if err := validateState(&s, b); err != nil {
		t.Fatal(err)
	}
	// !Boosts[d] → CashBonus[d] forced to 0; Boosts[d] keeps bonus.
	if s.Training.CashBonus[0] != 0 || s.Training.CashBonus[2] != 0 {
		t.Fatalf("orphan CashBonus not repaired: %v", s.Training.CashBonus)
	}
	if s.Training.CashBonus[1] != 3.75 {
		t.Fatalf("boosted dim CashBonus mutated: %v", s.Training.CashBonus)
	}
	if s.Training.BoostCashPaid != 999 {
		t.Fatalf("BoostCashPaid recomputed: got %v want 999", s.Training.BoostCashPaid)
	}
}

func TestLoadRepairsOrphanTrainingCashBonus(t *testing.T) {
	b := balance.Default()
	path := filepath.Join(t.TempDir(), "boost-repair.json")
	s := model.GameState{
		Resources:   model.Resources{Cash: 100, RnD: 100},
		Progression: model.ProgressionState{MaxUnlockedGen: 1},
		HasTraining: true,
		Training: model.TrainingJob{
			Gen: 1, WorkRemaining: 50,
			Boosts:        [model.NumQualityDims]bool{false, false, false, false},
			CashBonus:     [model.NumQualityDims]float64{1.5, 0, 0, 0},
			BoostCashPaid: 42,
		},
	}
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadWithConfig(path, b)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if got.Training.CashBonus[0] != 0 {
		t.Fatalf("CashBonus not repaired on load: %v", got.Training.CashBonus)
	}
	if got.Training.BoostCashPaid != 42 {
		t.Fatalf("BoostCashPaid = %v, want 42", got.Training.BoostCashPaid)
	}
}

func q(a, b, c, d float64) [model.NumQualityDims]float64 {
	return [model.NumQualityDims]float64{a, b, c, d}
}

func TestMigratedExplodedSavePlayable(t *testing.T) {
	b := balance.Default()
	path := filepath.Join(t.TempDir(), "exploded.json")
	// Legacy bare save with exploded OpenAI quality and high player model.
	legacy := model.GameState{
		GameTime:  7000 * 86400,
		Resources: model.Resources{Cash: 1e6, RnD: 1e9},
		Models: []model.Model{{
			Gen: 5, Online: true, Users: 5000, Price: 12, Name: "Flagship",
			Quality: q(100, 80, 90, 70),
		}},
		UnlockedTech: []string{
			balance.GenUnlockNodeID(2), balance.GenUnlockNodeID(3),
			balance.GenUnlockNodeID(4), balance.GenUnlockNodeID(5),
		},
		Competitors: balance.DefaultCompetitors(),
		Campaign: model.CampaignState{
			Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageExpand, Cycle: 2,
			Primary:   model.RivalRoadmap{Company: "OpenAI", ActionIndex: 0, CyclesUntilAction: 1},
			Wildcard:  model.RivalRoadmap{Company: "DeepSeek", ActionIndex: 0, CyclesUntilAction: 3},
			RandState: 7,
		},
	}
	// Explode OpenAI absolute quality (pre-migration runaway).
	legacy.Competitors[0].Quality = q(135185, 90000, 50000, 40000)
	legacy.Competitors[0].Skill = q(1.25, 1.1, 0.8, 1.0)
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	s, ok, err := LoadWithConfig(path, b)
	if err != nil || !ok {
		t.Fatalf("migrate load: ok=%v err=%v", ok, err)
	}
	// After migration, OpenAI must sit inside the frontier band.
	assertRivalsBounded(t, s, b)

	// Tick still works.
	s = sim.Tick(s, 3600, nil, b)
	assertRivalsBounded(t, s, b)

	// Campaign board cycles still work and stay bounded.
	for i := 0; i < 20; i++ {
		s = sim.AdvanceCampaignCycle(s, b)
	}
	assertRivalsBounded(t, s, b)

	// Save/reload envelope remains playable and bounded.
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	s2, ok, err := LoadWithConfig(path, b)
	if err != nil || !ok {
		t.Fatalf("reload: ok=%v err=%v", ok, err)
	}
	assertRivalsBounded(t, s2, b)
	// Player model quality preserved through migrate+save+reload.
	if len(s2.Models) == 0 || s2.Models[0].Quality[model.DimCapability] != 100 {
		t.Fatalf("model quality lost: %+v", s2.Models)
	}
	s2 = sim.Tick(s2, 100, nil, b)
	assertRivalsBounded(t, s2, b)
}

func TestModelQualityStableAcrossReload(t *testing.T) {
	b := balance.Default()
	path := filepath.Join(t.TempDir(), "stable.json")
	s := model.GameState{
		Resources: model.Resources{Cash: 100, RnD: 100},
		Models: []model.Model{{
			Gen: 3, Online: true, Users: 10, Price: 12,
			Quality: q(40, 20, 15, 10),
		}},
		Progression: model.ProgressionState{MaxUnlockedGen: 3, IndustryTime: 2500 * 86400},
		Competitors: balance.DefaultCompetitors(),
	}
	want := s.Models[0].Quality
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadWithConfig(path, b)
	if err != nil || !ok {
		t.Fatalf("load: %v", err)
	}
	if got.Models[0].Quality != want {
		t.Fatalf("quality after load: %v want %v", got.Models[0].Quality, want)
	}
	// Frontier/industry movement must not rewrite stored quality.
	got.Progression.IndustryTime += 20000 * 86400
	got = sim.Tick(got, 3600, nil, b)
	if got.Models[0].Quality != want {
		t.Fatalf("quality after frontier move: %v want %v", got.Models[0].Quality, want)
	}
}

func assertRivalsBounded(t *testing.T, s model.GameState, b balance.Config) {
	t.Helper()
	gf := sim.GlobalFrontier(s, b)
	for _, c := range s.Competitors {
		for d := range model.NumQualityDims {
			if gf[d] <= 0 {
				continue
			}
			hi := gf[d] * 1.15
			q := c.Quality[d]
			// Anti-runaway ceiling; floor is approach-target soft constraint.
			if q > hi+1e-3 || q < 0 {
				t.Fatalf("%s dim %d = %v above ceiling %v or negative (gf=%v)", c.Name, d, q, hi, gf[d])
			}
		}
	}
}

func TestProbeLegacyStaffEnvelope(t *testing.T) {
	raw := []byte(`{
		"schemaVersion":1,
		"state":{
			"Engineers":3,
			"Ops":2,
			"Marketing":1,
			"HiredStars":["aria-chen","marcus-cole"],
			"Research":{"Researchers":[0,4,1,0],"EfficiencyMult":1},
			"Resources":{"Cash":1000,"RnD":0},
			"GameTime":100
		}
	}`)
	leg := probeLegacyStaff(raw)
	if leg.Engineers != 3 || leg.Ops != 2 || leg.Marketing != 1 {
		t.Fatalf("staff counts: %+v", leg)
	}
	if leg.Researchers != [4]int{0, 4, 1, 0} {
		t.Fatalf("researchers: %v", leg.Researchers)
	}
	if len(leg.HiredStars) != 2 {
		t.Fatalf("stars: %v", leg.HiredStars)
	}
	// heads = 3+2+1+4+1 = 11; stars = 2 → 11*2000 + 2*50000 = 122000
	if got := leg.compensation(); got != 122_000 {
		t.Fatalf("compensation = %v, want 122000", got)
	}
}

func TestProbeLegacyStaffBare(t *testing.T) {
	raw := []byte(`{"Engineers":5,"HiredStars":["x"],"Research":{"Researchers":[1,0,0,0]}}`)
	leg := probeLegacyStaff(raw)
	if leg.Engineers != 5 || leg.Researchers[0] != 1 || len(leg.HiredStars) != 1 {
		t.Fatalf("bare probe: %+v", leg)
	}
	// 5+1 heads + 1 star = 6*2000 + 50000 = 62000
	if got := leg.compensation(); got != 62_000 {
		t.Fatalf("compensation = %v, want 62000", got)
	}
}

func TestMigrateToEmployeeOfficeProbeCompensation(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		GameTime:  500,
		Resources: model.Resources{Cash: 1000},
	}
	leg := legacyStaffFields{
		Engineers: 2, Ops: 1, Marketing: 0,
		Researchers: [4]int{0, 1, 0, 0},
		HiredStars:  []string{"aria-chen"},
	}
	// heads=4 → 8000 + star 50000 = 58000; cash 1000 → 59000
	ns := migrateToEmployeeOffice(s, b, leg)
	if ns.Office.Level != 1 {
		t.Fatalf("Office.Level = %d, want 1", ns.Office.Level)
	}
	if ns.Employees == nil {
		t.Fatal("Employees still nil")
	}
	if ns.Resources.Cash != 59_000 {
		t.Fatalf("Cash = %v, want 59000", ns.Resources.Cash)
	}
	if len(ns.Market.Candidates) != b.MarketPoolSize {
		t.Fatalf("market size = %d, want %d", len(ns.Market.Candidates), b.MarketPoolSize)
	}
	// Probe compensation must NOT also apply flat RestructuringGrant.
	if ns.Resources.Cash == 1000+b.RestructuringGrant {
		t.Fatal("double-applied RestructuringGrant")
	}
}

func TestMigrateToEmployeeOfficeRestructuringGrantFallback(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		GameTime:  1000,
		Resources: model.Resources{Cash: 500},
	}
	ns := migrateToEmployeeOffice(s, b, legacyStaffFields{})
	want := 500 + b.RestructuringGrant
	if ns.Resources.Cash != want {
		t.Fatalf("Cash = %v, want %v (grant fallback)", ns.Resources.Cash, want)
	}
	if ns.Office.Level != 1 || len(ns.Market.Candidates) == 0 {
		t.Fatalf("office/market not seeded: level=%d cands=%d", ns.Office.Level, len(ns.Market.Candidates))
	}
}

func TestMigrateToEmployeeOfficeNoGrantAtGameStart(t *testing.T) {
	b := balance.Default()
	s := model.GameState{GameTime: 0, Resources: model.Resources{Cash: 100}}
	ns := migrateToEmployeeOffice(s, b, legacyStaffFields{})
	if ns.Resources.Cash != 100 {
		t.Fatalf("Cash = %v, want 100 (no grant at t=0)", ns.Resources.Cash)
	}
}

func TestLoadSchema1LegacyStaffCompensation(t *testing.T) {
	b := balance.Default()
	path := filepath.Join(t.TempDir(), "v1-staff.json")
	// Schema 1 envelope still carrying retired staff/star fields in JSON.
	raw := []byte(`{
		"schemaVersion":1,
		"state":{
			"GameTime":3600,
			"Resources":{"Cash":10000,"RnD":50},
			"Engineers":2,
			"Ops":1,
			"Marketing":1,
			"HiredStars":["aria-chen"],
			"Research":{"Researchers":[0,3,0,0],"EfficiencyMult":1},
			"Progression":{"MaxUnlockedGen":1,"IndustryTime":3600}
		}
	}`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadWithConfig(path, b)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	// heads = 2+1+1+3 = 7 → 14000; stars 1 → 50000; cash 10000 → 74000
	if got.Resources.Cash != 74_000 {
		t.Fatalf("Cash = %v, want 74000", got.Resources.Cash)
	}
	if got.Office.Level != 1 {
		t.Fatalf("Office.Level = %d, want 1", got.Office.Level)
	}
	if len(got.Employees) != 0 {
		t.Fatalf("Employees = %d, want empty", len(got.Employees))
	}
	if len(got.Market.Candidates) != b.MarketPoolSize {
		t.Fatalf("market = %d, want %d", len(got.Market.Candidates), b.MarketPoolSize)
	}
	// Rewritten as schema 2.
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var env SaveFile
	if err := json.Unmarshal(onDisk, &env); err != nil {
		t.Fatal(err)
	}
	if env.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("schemaVersion = %d, want %d", env.SchemaVersion, CurrentSchemaVersion)
	}
	// Idempotent: second load must not re-grant.
	cashAfter := got.Resources.Cash
	got2, ok, err := LoadWithConfig(path, b)
	if err != nil || !ok {
		t.Fatalf("second load: %v", err)
	}
	if got2.Resources.Cash != cashAfter {
		t.Fatalf("double compensation: first=%v second=%v", cashAfter, got2.Resources.Cash)
	}
}

func TestLoadSchema1RestructuringGrantFallback(t *testing.T) {
	b := balance.Default()
	path := filepath.Join(t.TempDir(), "v1-midrun.json")
	// Mid-run v1 with no probeable staff (fields already dropped or never set).
	raw := []byte(`{
		"schemaVersion":1,
		"state":{
			"GameTime":999,
			"Resources":{"Cash":1,"RnD":0},
			"Progression":{"MaxUnlockedGen":2,"IndustryTime":999}
		}
	}`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadWithConfig(path, b)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if got.Resources.Cash != 1+b.RestructuringGrant {
		t.Fatalf("Cash = %v, want %v", got.Resources.Cash, 1+b.RestructuringGrant)
	}
	if got.Office.Level != 1 || len(got.Market.Candidates) == 0 {
		t.Fatalf("defaults missing: office=%d market=%d", got.Office.Level, len(got.Market.Candidates))
	}
}

func TestLoadSchema2SoftRepairsOfficeAndMarket(t *testing.T) {
	b := balance.Default()
	path := filepath.Join(t.TempDir(), "v2-zero-office.json")
	// Already schema 2 but Office.Level=0 and empty market — soft repair, no grant.
	s := model.GameState{
		GameTime:    5000,
		Resources:   model.Resources{Cash: 42, RnD: 1},
		Progression: model.ProgressionState{MaxUnlockedGen: 1, IndustryTime: 5000},
		// Office.Level 0, Employees nil, Market empty (uninitialized)
	}
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	// Force schema 2 envelope with zero office (Save already writes CurrentSchemaVersion).
	got, ok, err := LoadWithConfig(path, b)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if got.Office.Level != 1 {
		t.Fatalf("Office.Level = %d, want 1", got.Office.Level)
	}
	if got.Employees == nil {
		t.Fatal("Employees still nil")
	}
	if len(got.Market.Candidates) != b.MarketPoolSize {
		t.Fatalf("market = %d, want %d", len(got.Market.Candidates), b.MarketPoolSize)
	}
	// No RestructuringGrant on already-current schema.
	if got.Resources.Cash != 42 {
		t.Fatalf("Cash = %v, want 42 (no grant on schema 2 soft-repair)", got.Resources.Cash)
	}
}

func TestLoadSchema2PreservesDepletedMarket(t *testing.T) {
	b := balance.Default()
	path := filepath.Join(t.TempDir(), "v2-empty-market.json")
	// Legal runtime: hired everyone, waiting on free refresh, high reroll count.
	s := model.GameState{
		GameTime:  100,
		Resources: model.Resources{Cash: 99},
		Office:    model.Office{Level: 2},
		Employees: []model.Employee{{ID: "e1", MonthlySalary: 1000}},
		Market: model.TalentMarket{
			Candidates:    []model.Employee{}, // non-nil empty
			NextRefreshAt: 10000,
			RerollCount:   4,
			RandState:     42,
		},
		Progression: model.ProgressionState{MaxUnlockedGen: 1, IndustryTime: 100},
	}
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadWithConfig(path, b)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if len(got.Market.Candidates) != 0 {
		t.Fatalf("depleted market was refilled: %d candidates", len(got.Market.Candidates))
	}
	if got.Market.RerollCount != 4 {
		t.Fatalf("RerollCount=%d want 4 (no free reset on reload)", got.Market.RerollCount)
	}
	if got.Market.NextRefreshAt != 10000 {
		t.Fatalf("NextRefreshAt=%v want 10000", got.Market.NextRefreshAt)
	}
	if got.Market.RandState != 42 {
		t.Fatalf("RandState=%d want 42", got.Market.RandState)
	}
}

func TestLoadBareLegacyMigratesOffice(t *testing.T) {
	b := balance.Default()
	path := filepath.Join(t.TempDir(), "bare-staff.json")
	raw := []byte(`{
		"GameTime":100,
		"Resources":{"Cash":0,"RnD":0},
		"Engineers":1,
		"Ops":0,
		"Marketing":0,
		"HiredStars":[],
		"Research":{"Researchers":[0,1,0,0]}
	}`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadWithConfig(path, b)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	// heads = 1+1 = 2 → 4000
	if got.Resources.Cash != 4000 {
		t.Fatalf("Cash = %v, want 4000", got.Resources.Cash)
	}
	if got.Office.Level != 1 || len(got.Market.Candidates) == 0 {
		t.Fatalf("office/market: level=%d cands=%d", got.Office.Level, len(got.Market.Candidates))
	}
	if got.Progression.MaxUnlockedGen < 1 {
		t.Fatalf("MaxUnlockedGen = %d", got.Progression.MaxUnlockedGen)
	}
	onDisk, _ := os.ReadFile(path)
	var env SaveFile
	if err := json.Unmarshal(onDisk, &env); err != nil {
		t.Fatal(err)
	}
	if env.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("schema = %d, want %d", env.SchemaVersion, CurrentSchemaVersion)
	}
}
