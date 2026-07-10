package sim

import (
	"errors"
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestChooseDoctrineRequiresOnlineModel(t *testing.T) {
	_, err := Apply(model.GameState{}, model.ChooseDoctrine{Doctrine: model.DoctrineConsumer}, balance.Default())
	if !errors.Is(err, ErrCampaignNeedsModel) {
		t.Fatalf("err=%v", err)
	}
}

func TestChooseDoctrineStartsEstablishStage(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Models: []model.Model{{Online: true}}}
	s.Campaign.RandState = 1
	ns, err := Apply(s, model.ChooseDoctrine{Doctrine: model.DoctrineConsumer}, b)
	if err != nil {
		t.Fatal(err)
	}
	if ns.Campaign.Doctrine != model.DoctrineConsumer || ns.Campaign.Stage != model.CampaignStageEstablish {
		t.Fatalf("campaign=%+v", ns.Campaign)
	}
}

func TestChoosePerkValidatesTierAndDoctrine(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Campaign: model.CampaignState{Doctrine: model.DoctrineConsumer, PerkTierPending: 1}}
	ns, err := Apply(s, model.ChooseDoctrinePerk{PerkID: "consumer-premium"}, b)
	if err != nil {
		t.Fatal(err)
	}
	if len(ns.Campaign.Perks) != 1 || ns.Campaign.PerkTierPending != 0 {
		t.Fatalf("campaign=%+v", ns.Campaign)
	}
}

func TestChooseSecondaryIncludesOneTierOnePerk(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Campaign: model.CampaignState{Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageShowdown}}
	ns, err := Apply(s, model.ChooseSecondaryDoctrine{Doctrine: model.DoctrineDeveloper, PerkID: "developer-open"}, b)
	if err != nil {
		t.Fatal(err)
	}
	if ns.Campaign.Secondary != model.DoctrineDeveloper || ns.Campaign.SecondaryPerk != "developer-open" {
		t.Fatalf("campaign=%+v", ns.Campaign)
	}
}

func TestPivotChargesAndResetsBuild(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Resources: model.Resources{Cash: 100000, RnD: 50000}}
	s.Models = []model.Model{{Online: true, Users: 1000, Price: 12}}
	s.Campaign = model.CampaignState{Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageExpand, Perks: []string{"consumer-premium"}}
	ns, err := Apply(s, model.PivotDoctrine{Doctrine: model.DoctrineEnterprise}, b)
	if err != nil {
		t.Fatal(err)
	}
	if ns.Campaign.Doctrine != model.DoctrineEnterprise || !ns.Campaign.PivotUsed || len(ns.Campaign.Perks) != 0 {
		t.Fatalf("campaign=%+v", ns.Campaign)
	}
	if ns.Resources.Cash != 80000 || ns.Resources.RnD != 45000 {
		t.Fatalf("resources=%+v", ns.Resources)
	}
}
