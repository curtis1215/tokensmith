package store

import (
	"errors"
	"math"
	"os"
	"sort"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

// rivalRankPct maps a 0-based rank among up to 7 rivals onto the approved
// relative-frontier ladder (lowest → highest).
var rivalRankPct = []float64{0.88, 0.92, 0.96, 1.00, 1.04, 1.09, 1.14}

const (
	skillFloor = 0.92
	skillCeil  = 1.08
)

// migrateV0 upgrades a legacy (schema 0 / bare GameState) save into the
// long-run progression shape. Pure with respect to the filesystem.
func migrateV0(s model.GameState, b balance.Config) (model.GameState, error) {
	ns := s
	ns.Progression.MaxUnlockedGen = inferMaxUnlockedGen(ns, b)
	ns.Progression.IndustryTime = ns.GameTime

	// Seed empty era rows for every procedural era already reached (III+).
	ns.Progression.Eras = seedEraProgress(ns.Progression.MaxUnlockedGen, ns.Progression.Eras)

	// Clear legacy roadmap compounding residue.
	if len(ns.Competitors) > 0 {
		comps := append([]model.Competitor(nil), ns.Competitors...)
		for i := range comps {
			comps[i].Skill = clampSkillPreserveStrongest(comps[i].Skill)
			comps[i].MomentumPct = [model.NumQualityDims]float64{}
			comps[i].MomentumCycles = 0
		}
		ns.Competitors = comps
	}

	// Map rival absolute quality ranks → frontier-relative percentages.
	ns = remapRivalQualitiesByRank(ns, b)

	// Initialize / refresh era leaders with zero momentum (cleared above).
	ns = sim.EnsureRivalEraState(ns, b)

	// Final band clamp using post-migration global frontier.
	ns = clampMigratedRivals(ns, b)
	return ns, nil
}

func inferMaxUnlockedGen(s model.GameState, b balance.Config) int {
	g := 1
	// Contiguous legacy model-gen-2..5 tech nodes.
	for n := 2; n <= 5; n++ {
		if !hasTech(s, balance.GenUnlockNodeID(n)) {
			break
		}
		g = n
	}
	for _, m := range s.Models {
		if m.Gen > g {
			g = m.Gen
		}
	}
	if s.HasTraining && s.Training.Gen > g {
		g = s.Training.Gen
	}
	if s.Progression.MaxUnlockedGen > g {
		g = s.Progression.MaxUnlockedGen
	}
	if g < 1 {
		return 1
	}
	return g
}

func hasTech(s model.GameState, id string) bool {
	for _, u := range s.UnlockedTech {
		if u == id {
			return true
		}
	}
	return false
}

func seedEraProgress(maxGen int, existing []model.EraProgress) []model.EraProgress {
	era, err := balance.EraForGen(maxGen)
	if err != nil || era < 3 {
		return existing
	}
	byEra := map[int]model.EraProgress{}
	for _, ep := range existing {
		byEra[ep.Era] = ep
	}
	out := make([]model.EraProgress, 0, era-2)
	for e := 3; e <= era; e++ {
		if ep, ok := byEra[e]; ok {
			out = append(out, ep)
		} else {
			out = append(out, model.EraProgress{Era: e})
		}
	}
	return out
}

func clampSkillPreserveStrongest(skill [model.NumQualityDims]float64) [model.NumQualityDims]float64 {
	strongest := 0
	for d := 1; d < model.NumQualityDims; d++ {
		if skill[d] > skill[strongest] {
			strongest = d
		}
	}
	var out [model.NumQualityDims]float64
	for d := range out {
		v := skill[d]
		if v < skillFloor {
			v = skillFloor
		}
		if v > skillCeil {
			v = skillCeil
		}
		out[d] = v
	}
	// Keep the original strongest dimension among the max after clamping.
	maxOther := skillFloor
	for d := range out {
		if d == strongest {
			continue
		}
		if out[d] > maxOther {
			maxOther = out[d]
		}
	}
	if out[strongest] < maxOther {
		out[strongest] = maxOther
	}
	return out
}

func remapRivalQualitiesByRank(s model.GameState, b balance.Config) model.GameState {
	if len(s.Competitors) == 0 {
		return s
	}
	ns := s
	gf := sim.GlobalFrontier(ns, b)
	comps := append([]model.Competitor(nil), ns.Competitors...)
	n := len(comps)
	for d := range model.NumQualityDims {
		type pair struct {
			i int
			q float64
		}
		order := make([]pair, n)
		for i := range comps {
			order[i] = pair{i: i, q: comps[i].Quality[d]}
		}
		sort.SliceStable(order, func(a, b int) bool {
			if order[a].q == order[b].q {
				return order[a].i < order[b].i
			}
			return order[a].q < order[b].q
		})
		for rank, p := range order {
			pct := rankToPct(rank, n)
			front := gf[d]
			if front <= 0 {
				// No frontier signal: keep non-negative finite quality.
				if comps[p.i].Quality[d] < 0 || math.IsNaN(comps[p.i].Quality[d]) || math.IsInf(comps[p.i].Quality[d], 0) {
					comps[p.i].Quality[d] = 0
				}
				continue
			}
			comps[p.i].Quality[d] = front * pct
		}
	}
	ns.Competitors = comps
	return ns
}

func rankToPct(rank, n int) float64 {
	if n <= 1 {
		return 1.0
	}
	// Map rank in [0, n-1] onto rivalRankPct indices [0, 6].
	idx := rank * (len(rivalRankPct) - 1) / (n - 1)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(rivalRankPct) {
		idx = len(rivalRankPct) - 1
	}
	return rivalRankPct[idx]
}

func clampMigratedRivals(s model.GameState, b balance.Config) model.GameState {
	if len(s.Competitors) == 0 {
		return s
	}
	ns := s
	gf := sim.GlobalFrontier(ns, b)
	comps := append([]model.Competitor(nil), ns.Competitors...)
	for i := range comps {
		for d := range model.NumQualityDims {
			q := comps[i].Quality[d]
			if math.IsNaN(q) || math.IsInf(q, 0) {
				q = 0
			}
			if gf[d] > 0 {
				lo := gf[d] * 0.85
				hi := gf[d] * 1.15
				if q < lo {
					q = lo
				}
				if q > hi {
					q = hi
				}
			} else if q < 0 {
				q = 0
			}
			comps[i].Quality[d] = q
		}
	}
	ns.Competitors = comps
	return ns
}

// validateState rejects non-finite or illegal progression/resource values.
// When HasTraining, repairs CashBonus[d]=0 if !Boosts[d] (in place); does not
// recompute BoostCashPaid (historical charge).
func validateState(s *model.GameState, _ balance.Config) error {
	check := func(name string, v float64, allowNeg bool) error {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return errors.New("store: invalid " + name)
		}
		if !allowNeg && v < 0 {
			return errors.New("store: negative " + name)
		}
		return nil
	}
	if err := check("GameTime", s.GameTime, false); err != nil {
		return err
	}
	if err := check("IndustryTime", s.Progression.IndustryTime, false); err != nil {
		return err
	}
	if err := check("RnD", s.Resources.RnD, false); err != nil {
		return err
	}
	if err := check("Cash", s.Resources.Cash, true); err != nil {
		return err
	}
	if s.Progression.MaxUnlockedGen < 1 {
		return errors.New("store: MaxUnlockedGen < 1")
	}
	if s.HasTraining {
		if err := check("Training.WorkRemaining", s.Training.WorkRemaining, false); err != nil {
			return err
		}
		if s.Training.Gen < 1 {
			return errors.New("store: invalid training gen")
		}
		if err := check("Training.BoostCashPaid", s.Training.BoostCashPaid, false); err != nil {
			return err
		}
		for d := range model.NumQualityDims {
			if err := check("Training.CashBonus", s.Training.CashBonus[d], false); err != nil {
				return err
			}
			// Trust Boosts for bonus presence: orphan bonus is soft-repaired.
			if !s.Training.Boosts[d] {
				s.Training.CashBonus[d] = 0
			}
		}
	}
	for i, m := range s.Models {
		for d, q := range m.Quality {
			if math.IsNaN(q) || math.IsInf(q, 0) || q < 0 {
				return errors.New("store: invalid model quality")
			}
			_ = d
		}
		if m.Gen < 1 {
			return errors.New("store: invalid model gen")
		}
		_ = i
	}
	for _, c := range s.Competitors {
		for _, q := range c.Quality {
			if math.IsNaN(q) || math.IsInf(q, 0) || q < 0 {
				return errors.New("store: invalid rival quality")
			}
		}
		for _, sk := range c.Skill {
			if math.IsNaN(sk) || math.IsInf(sk, 0) {
				return errors.New("store: invalid rival skill")
			}
		}
	}
	fp := s.Progression.Frontier
	for _, v := range []float64{fp.RnDTotal, fp.RnDRemaining, fp.WorkTotal, fp.WorkRemaining, fp.RecommendedCompute} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return errors.New("store: invalid frontier project fields")
		}
	}
	return nil
}

// backupV0 writes path+".v0.bak" once. Existing backups are never overwritten.
func backupV0(path string, data []byte) error {
	bak := path + ".v0.bak"
	if _, err := os.Stat(bak); err == nil {
		return nil // already present
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.WriteFile(bak, data, 0o644)
}
