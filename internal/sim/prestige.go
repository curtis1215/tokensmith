package sim

import (
	"math"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func isPrestigeUnlocked(s model.GameState, id string) bool {
	for _, u := range s.Prestige.UnlockedPrestige {
		if u == id {
			return true
		}
	}
	return false
}

// prestigeEffects aggregates permanent upgrades (neutral when none unlocked).
func prestigeEffects(unlocked []string, b balance.Config) model.PrestigeEffects {
	agg := model.NeutralPrestigeEffects()
	for _, node := range b.PrestigeNodes {
		if !contains(unlocked, node.ID) {
			continue
		}
		agg.StartCash += node.Effects.StartCash
		agg.StartRnD += node.Effects.StartRnD
		agg.RnDMult *= node.Effects.RnDMult
		agg.CashMult *= node.Effects.CashMult
	}
	return agg
}

func contains(ss []string, id string) bool {
	for _, s := range ss {
		if s == id {
			return true
		}
	}
	return false
}

// patentsFor is the patents earned by prestiging at a given peak valuation.
func patentsFor(peak float64, b balance.Config) float64 {
	if peak <= 0 {
		return 0
	}
	return math.Floor(math.Sqrt(peak / b.PatentK))
}

// freshRun produces a new run's starting state, preserving prestige.
func freshRun(p model.Prestige, b balance.Config) model.GameState {
	pe := prestigeEffects(p.UnlockedPrestige, b)
	var ns model.GameState
	ns.Prestige = p
	ns.Competitors = balance.DefaultCompetitors()
	ns.Research.EfficiencyMult = 1
	ns.Resources.Cash = b.StartingCash + pe.StartCash
	ns.Resources.RnD = pe.StartRnD
	return ns
}
