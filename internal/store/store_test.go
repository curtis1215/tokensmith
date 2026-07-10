package store

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"tokensmith/internal/model"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "save.json")
	var s model.GameState
	s.Resources.Cash = 12345
	s.Resources.RnD = 6789
	s.Models = []model.Model{{Gen: 2, Online: true, Users: 1000, Price: 12}}
	s.Prestige.Patents = 3
	s.HiredStars = []string{"aria-chen"}
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
	if got.Prestige.Patents != 3 || len(got.HiredStars) != 1 {
		t.Errorf("prestige/stars not restored: %+v %+v", got.Prestige, got.HiredStars)
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
