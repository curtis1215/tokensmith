package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "save.json")
	var s model.GameState
	s.Resources.Cash = 12345
	s.Resources.RnD = 6789
	s.Models = []model.Model{{Gen: 2, Online: true, Users: 1000, Price: 12}}
	s.Prestige.Patents = 3
	s.Office.Level = 1
	s.Employees = []model.Employee{{ID: "e1", Name: "Ada", MonthlySalary: 2500}}
	s.Campaign = model.CampaignState{
		RandState: 7, Cycle: 4, Doctrine: model.DoctrineConsumer,
		Stage: model.CampaignStageExpand, Perks: []string{"consumer-premium"},
		Secondary: model.DoctrineDeveloper, SecondaryPerk: "developer-open",
		Primary: model.RivalRoadmap{
			Company: "OpenAI", ActionIndex: 1, CyclesUntilAction: 2, IntelFull: true, LastExecutedCycle: 3,
		},
		Wildcard: model.RivalRoadmap{
			Company: "DeepSeek", ActionIndex: 0, CyclesUntilAction: 1,
		},
		Reports: []model.BoardReport{
			{Cycle: 3, Entries: []model.CampaignReportEntry{{Kind: model.ReportRivalAction, SubjectID: "OpenAI"}}},
			{Cycle: 4, Entries: []model.CampaignReportEntry{{Kind: model.ReportStageAdvanced, SubjectID: string(model.CampaignStageExpand)}}},
		},
		Legacy: model.LegacyChoice{Kind: model.LegacyIntel},
	}
	s.Prestige.RouteBadges = []model.Doctrine{model.DoctrineConsumer}
	if err := Save(path, s); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, ok, err := Load(path)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if got.Resources.Cash != 12345 || got.Resources.RnD != 6789 {
		t.Errorf("resources not restored: %+v", got.Resources)
	}
	if len(got.Models) != 1 || got.Models[0].Users != 1000 {
		t.Errorf("models not restored: %+v", got.Models)
	}
	if got.Prestige.Patents != 3 || got.Office.Level != 1 || len(got.Employees) != 1 || got.Employees[0].ID != "e1" {
		t.Errorf("prestige/office/employees not restored: prestige=%+v office=%+v employees=%+v",
			got.Prestige, got.Office, got.Employees)
	}
	if got.Campaign.Cycle != 4 || got.Campaign.Doctrine != model.DoctrineConsumer {
		t.Fatalf("campaign not restored: %+v", got.Campaign)
	}
	if len(got.Prestige.RouteBadges) != 1 || got.Prestige.RouteBadges[0] != model.DoctrineConsumer {
		t.Fatalf("badges=%+v", got.Prestige.RouteBadges)
	}
	if !reflect.DeepEqual(got.Campaign.Perks, s.Campaign.Perks) {
		t.Fatalf("perks=%+v want %+v", got.Campaign.Perks, s.Campaign.Perks)
	}
	if !reflect.DeepEqual(got.Campaign.Primary, s.Campaign.Primary) || !reflect.DeepEqual(got.Campaign.Wildcard, s.Campaign.Wildcard) {
		t.Fatalf("roadmaps primary=%+v wildcard=%+v", got.Campaign.Primary, got.Campaign.Wildcard)
	}
	if !reflect.DeepEqual(got.Campaign.Reports, s.Campaign.Reports) {
		t.Fatalf("reports=%+v want %+v", got.Campaign.Reports, s.Campaign.Reports)
	}
	if !reflect.DeepEqual(got.Campaign.Legacy, s.Campaign.Legacy) {
		t.Fatalf("legacy=%+v want %+v", got.Campaign.Legacy, s.Campaign.Legacy)
	}
	if got.Campaign.Secondary != model.DoctrineDeveloper || got.Campaign.SecondaryPerk != "developer-open" {
		t.Fatalf("secondary=%s perk=%s", got.Campaign.Secondary, got.Campaign.SecondaryPerk)
	}
}

// TestOldSavePreCampaignJSON loads a literal pre-campaign save (resources only)
// and asserts cash/RnD survive with a zero Campaign.
func TestOldSavePreCampaignJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old-save.json")
	// Pre-campaign shape: only resources; no campaign / prestige fields.
	const raw = `{"Resources":{"Cash":4242,"RnD":1337}}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok, err := Load(path)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if got.Resources.Cash != 4242 || got.Resources.RnD != 1337 {
		t.Fatalf("resources reset or lost: %+v", got.Resources)
	}
	if !reflect.DeepEqual(got.Campaign, model.CampaignState{}) {
		t.Fatalf("expected zero campaign, got %+v", got.Campaign)
	}
	if !reflect.DeepEqual(got.Prestige, model.Prestige{}) {
		t.Fatalf("expected zero prestige, got %+v", got.Prestige)
	}
}

func TestLoadMissing(t *testing.T) {
	_, ok, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil || ok {
		t.Fatalf("missing file: ok=%v err=%v, want false/nil", ok, err)
	}
}

func TestSaveEnvelopeShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "env.json")
	var s model.GameState
	s.Resources.Cash = 99
	s.Progression.MaxUnlockedGen = 5
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Top-level envelope keys — not a bare GameState.
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatal(err)
	}
	if _, ok := top["schemaVersion"]; !ok {
		t.Fatalf("missing schemaVersion in %s", raw)
	}
	if _, ok := top["state"]; !ok {
		t.Fatalf("missing state in %s", raw)
	}
	// Bare GameState fields must not be top-level.
	if _, ok := top["Resources"]; ok {
		t.Fatal("legacy bare GameState written at top level")
	}
	var env SaveFile
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	if env.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("schemaVersion = %d, want %d", env.SchemaVersion, CurrentSchemaVersion)
	}
	if env.State.Resources.Cash != 99 || env.State.Progression.MaxUnlockedGen != 5 {
		t.Fatalf("envelope state wrong: %+v", env.State)
	}
}

func TestSaveEnvelopeRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rt.json")
	var s model.GameState
	s.Resources.Cash = 777
	s.Resources.RnD = 42
	s.Progression.MaxUnlockedGen = 6
	s.Progression.IndustryTime = 12345
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	got, ok, err := Load(path)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if got.Resources.Cash != 777 || got.Resources.RnD != 42 {
		t.Fatalf("resources: %+v", got.Resources)
	}
	if got.Progression.MaxUnlockedGen != 6 || got.Progression.IndustryTime != 12345 {
		t.Fatalf("progression: %+v", got.Progression)
	}
}

func TestLoadClampsOverheatedIndustryTime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hot.json")
	b := balance.Default()
	var s model.GameState
	s.Progression.MaxUnlockedGen = 5
	g10, err := balance.Generation(10)
	if err != nil {
		t.Fatal(err)
	}
	// Past Gen10 baseline (well past Gen6 player-lead cap for MaxUnlockedGen=5).
	s.Progression.IndustryTime = (g10.TimeBaselineDay + 500) * 86400
	s.Models = []model.Model{{
		Online: true, Gen: 5, Segment: model.SegConsumer,
		Quality: [model.NumQualityDims]float64{80, 40, 40, 40},
	}}
	s.Competitors = []model.Competitor{{
		Name: "OpenAI",
		Skill: [model.NumQualityDims]float64{1, 1, 1, 1},
		// Pre-clamp absolute quality as if time frontier was huge.
		Quality: [model.NumQualityDims]float64{500, 500, 500, 500},
	}}
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	got, ok, err := Load(path)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	cap := sim.IndustryTimeCapSec(got, b)
	if got.Progression.IndustryTime > cap+1e-6 {
		t.Fatalf("IndustryTime = %v, want ≤ cap %v", got.Progression.IndustryTime, cap)
	}
	gf := sim.GlobalFrontier(got, b)
	for d := 0; d < model.NumQualityDims; d++ {
		q := got.Competitors[0].Quality[d]
		lo, hi := gf[d]*0.85, gf[d]*1.15
		if q < lo-1e-6 || q > hi+1e-6 {
			t.Fatalf("rival dim %d Q=%v outside [%v,%v]", d, q, lo, hi)
		}
	}
	// Player quality untouched.
	if got.Models[0].Quality[0] != 80 {
		t.Fatalf("player quality rewritten: %v", got.Models[0].Quality)
	}
}

func TestLoadLegacyShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.json")
	// Unwrapped GameState (no schemaVersion) — pre-envelope saves.
	const raw = `{"Resources":{"Cash":111,"RnD":222},"GameTime":3600}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok, err := Load(path)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	// Mid-run bare save: flat RestructuringGrant (no probeable staff).
	wantCash := 111 + balance.Default().RestructuringGrant
	if got.Resources.Cash != wantCash || got.Resources.RnD != 222 || got.GameTime != 3600 {
		t.Fatalf("legacy not restored: %+v wantCash=%v", got, wantCash)
	}
	if got.Office.Level != 1 || len(got.Market.Candidates) == 0 {
		t.Fatalf("employee office not seeded: office=%+v market=%d", got.Office, len(got.Market.Candidates))
	}
	// Migration rewrites to the versioned envelope; original bytes are in .v0.bak.
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var env SaveFile
	if err := json.Unmarshal(onDisk, &env); err != nil {
		t.Fatalf("expected envelope after migrate: %v raw=%s", err, onDisk)
	}
	if env.SchemaVersion != CurrentSchemaVersion || env.State.Resources.Cash != wantCash {
		t.Fatalf("envelope wrong: %+v", env)
	}
	bak, err := os.ReadFile(path + ".v0.bak")
	if err != nil || string(bak) != raw {
		t.Fatalf("v0 backup missing or wrong: err=%v bak=%s", err, bak)
	}
	// IndustryTime migrated from GameTime.
	if got.Progression.IndustryTime != 3600 {
		t.Fatalf("IndustryTime = %v, want 3600", got.Progression.IndustryTime)
	}
	if got.Progression.MaxUnlockedGen < 1 {
		t.Fatalf("MaxUnlockedGen = %d", got.Progression.MaxUnlockedGen)
	}
}

func TestLoadCorruptUntouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.json")
	const garbage = `{not valid json!!!`
	if err := os.WriteFile(path, []byte(garbage), 0o644); err != nil {
		t.Fatal(err)
	}
	_, ok, err := Load(path)
	if err == nil || ok {
		t.Fatalf("corrupt load: ok=%v err=%v, want error", ok, err)
	}
	onDisk, err2 := os.ReadFile(path)
	if err2 != nil {
		t.Fatal(err2)
	}
	if string(onDisk) != garbage {
		t.Fatalf("corrupt file was rewritten: %q", onDisk)
	}
}

func TestLoadRejectsFutureSchemaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.json")
	// Future envelope must not load as v1 (would drop unknown fields on autosave).
	raw := `{"schemaVersion":999,"state":{"Resources":{"Cash":1,"RnD":2}},"futureOnlyField":true}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok, err := Load(path)
	if err == nil || ok {
		t.Fatalf("future schema loaded: ok=%v err=%v state=%+v", ok, err, got)
	}
	if err != nil && !strings.Contains(err.Error(), "unsupported schema") {
		t.Fatalf("error = %v, want unsupported schema", err)
	}
	// Original bytes untouched — no rewrite/downgrade.
	onDisk, err2 := os.ReadFile(path)
	if err2 != nil {
		t.Fatal(err2)
	}
	if string(onDisk) != raw {
		t.Fatalf("future save was rewritten: %q", onDisk)
	}
}
