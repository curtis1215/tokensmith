package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestCampaignRoadmapsDeterministicAndDistinct(t *testing.T) {
	b := balance.Default()
	a := model.GameState{Campaign: model.CampaignState{RandState: 42}}
	c := a
	a = initCampaignRoadmaps(a, model.DoctrineConsumer, b)
	c = initCampaignRoadmaps(c, model.DoctrineConsumer, b)
	if a.Campaign.Primary != c.Campaign.Primary || a.Campaign.Wildcard != c.Campaign.Wildcard {
		t.Fatalf("same seed diverged: %+v %+v", a.Campaign, c.Campaign)
	}
	if a.Campaign.Primary.Company == a.Campaign.Wildcard.Company {
		t.Fatal("rival roles must differ")
	}
}

func TestCampaignRoadmapsPrimaryMatchesDoctrine(t *testing.T) {
	b := balance.Default()
	// Seed a few times; primary must always be a Consumer primary-capable rival.
	allowed := map[string]bool{"OpenAI": true, "xAI": true, "Gemini": true}
	for seed := uint64(1); seed <= 20; seed++ {
		s := model.GameState{Campaign: model.CampaignState{RandState: seed}}
		s = initCampaignRoadmaps(s, model.DoctrineConsumer, b)
		if s.Campaign.Primary.Company == "" || s.Campaign.Wildcard.Company == "" {
			t.Fatalf("seed %d empty roadmaps: %+v", seed, s.Campaign)
		}
		if !allowed[s.Campaign.Primary.Company] {
			t.Fatalf("seed %d primary %q not in consumer primary set", seed, s.Campaign.Primary.Company)
		}
		if s.Campaign.Primary.Company == s.Campaign.Wildcard.Company {
			t.Fatalf("seed %d roles collided", seed)
		}
		if s.Campaign.Primary.ActionIndex != 0 || s.Campaign.Wildcard.ActionIndex != 0 {
			t.Fatalf("seed %d action index not zero: %+v", seed, s.Campaign)
		}
		if s.Campaign.Primary.CyclesUntilAction <= 0 || s.Campaign.Wildcard.CyclesUntilAction <= 0 {
			t.Fatalf("seed %d lead not scheduled: %+v", seed, s.Campaign)
		}
	}
}

func TestChooseDoctrineSeedsRoadmaps(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Models: []model.Model{{Online: true}}}
	s.Campaign.RandState = 7
	ns, err := Apply(s, model.ChooseDoctrine{Doctrine: model.DoctrineEnterprise}, b)
	if err != nil {
		t.Fatal(err)
	}
	if ns.Campaign.Primary.Company == "" || ns.Campaign.Wildcard.Company == "" {
		t.Fatalf("choose doctrine should seed roadmaps: %+v", ns.Campaign)
	}
	if ns.Campaign.Primary.Company == ns.Campaign.Wildcard.Company {
		t.Fatal("roles must differ")
	}
}

func TestPivotReseedsRoadmaps(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Resources: model.Resources{Cash: 100000, RnD: 50000}}
	s.Models = []model.Model{{Online: true, Users: 1000, Price: 12}}
	s.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageExpand,
		Perks: []string{"consumer-premium"}, RandState: 99,
		Primary:  model.RivalRoadmap{Company: "OpenAI", ActionIndex: 1, CyclesUntilAction: 1},
		Wildcard: model.RivalRoadmap{Company: "DeepSeek", ActionIndex: 1, CyclesUntilAction: 1},
	}
	ns, err := Apply(s, model.PivotDoctrine{Doctrine: model.DoctrineEnterprise}, b)
	if err != nil {
		t.Fatal(err)
	}
	if ns.Campaign.Primary.Company == "" || ns.Campaign.Primary.ActionIndex != 0 {
		t.Fatalf("pivot should reseed roadmaps: %+v", ns.Campaign)
	}
}
