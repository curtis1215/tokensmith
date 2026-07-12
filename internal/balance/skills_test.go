package balance

import (
	"testing"

	"tokensmith/internal/model"
)

func TestDefaultSkillsCountAndTiers(t *testing.T) {
	b := Default()
	if len(b.Skills) < 50 {
		t.Fatalf("skills=%d want >=50", len(b.Skills))
	}
	var m, d, g, sig int
	ids := map[string]bool{}
	for _, sk := range b.Skills {
		if ids[sk.ID] {
			t.Fatalf("dup id %s", sk.ID)
		}
		ids[sk.ID] = true
		switch sk.Tier {
		case model.SkillTierManager:
			m++
			if sk.Signature {
				t.Fatalf("manager signature %s", sk.ID)
			}
		case model.SkillTierDirector:
			d++
		case model.SkillTierGod:
			g++
			if sk.Signature {
				sig++
			}
		}
	}
	if m < 18 || d < 18 || g < 12 || sig < 9 {
		t.Fatalf("tier counts m=%d d=%d g=%d sig=%d", m, d, g, sig)
	}
}

func TestSkillByID(t *testing.T) {
	b := Default()
	sk, ok := SkillByID(b, "gs-token-oracle")
	if !ok || !sk.Signature || sk.TokenRnDMult < 1.1 {
		t.Fatalf("%+v ok=%v", sk, ok)
	}
}

func TestSkillsByTier(t *testing.T) {
	b := Default()
	mgr := SkillsByTier(b, model.SkillTierManager)
	if len(mgr) < 18 {
		t.Fatalf("manager skills=%d", len(mgr))
	}
	for _, sk := range mgr {
		if sk.Tier != model.SkillTierManager {
			t.Fatalf("wrong tier %s", sk.ID)
		}
	}
}

func TestDefaultSkillsRequiredIDs(t *testing.T) {
	b := Default()
	required := []string{
		"m-deep-research", "m-sre-craft", "m-ops-playbook", "m-growth-hacks",
		"m-thrifty", "m-mentor", "m-night-owl", "m-doc-driven",
		"m-perf-budget", "m-oncall", "m-copy-chief", "m-cross-train",
		"m-loyal", "m-sprinter", "m-frugal-stack", "m-pipeline",
		"m-community", "m-process-nerd",
		"d-lab-lead", "d-infra-scale", "d-sla-guard", "d-brand",
		"d-talent-magnet", "d-comp-opt", "d-hiring-blitz", "d-bench-strength",
		"d-qa-gate", "d-cost-ctrl", "d-partner", "d-security",
		"d-platform", "d-revops", "d-research-ops", "d-market-sense",
		"d-desk-layout", "d-retention",
		"g-polymath", "g-frontier", "g-rainmaker", "g-crisis",
		"g-architect", "g-scientist", "g-operator", "g-evangelist",
		"g-talent-blackhole", "g-equity-mind", "g-compounder", "g-full-stack-exec",
		"gs-token-oracle", "gs-poach-shield", "gs-moonshot", "gs-open-source-halo",
		"gs-chip-whisperer", "gs-regulatory-sage", "gs-viral-loop", "gs-war-chest",
		"gs-one-person-army",
	}
	for _, id := range required {
		if _, ok := SkillByID(b, id); !ok {
			t.Fatalf("missing skill %s", id)
		}
	}
	if len(b.Skills) != 57 {
		t.Fatalf("skills=%d want 57", len(b.Skills))
	}
}
