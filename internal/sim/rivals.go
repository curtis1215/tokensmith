package sim

import (
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

const (
	rivalFloorPct  = 0.85
	rivalCeilPct   = 1.15
	leaderBonusPct = 0.04
)

// ensureRivalEraState selects era leaders when the player's generation era
// changes. Leaders are persisted in Progression.Rivals so unrelated RNG use
// cannot reshuffle them. Era transitions clear rival momentum.
func ensureRivalEraState(s model.GameState, b balance.Config) model.GameState {
	era := rivalEraForState(s, b)
	if era < 1 {
		era = 1
	}
	if s.Progression.Rivals.Era == era && len(s.Progression.Rivals.Leaders) > 0 {
		return s
	}
	ns := s
	// Clear momentum on era transition (including first init from era 0).
	if s.Progression.Rivals.Era != era {
		if len(ns.Competitors) > 0 {
			comps := append([]model.Competitor(nil), ns.Competitors...)
			for i := range comps {
				comps[i].MomentumPct = [model.NumQualityDims]float64{}
				comps[i].MomentumCycles = 0
			}
			ns.Competitors = comps
		}
	}
	nLeaders := 2 + era%2
	leaders, rng := selectRivalLeaders(ns.Competitors, nLeaders, ns.Events.RandState)
	ns.Events.RandState = rng
	ns.Progression.Rivals = model.RivalEraState{Era: era, Leaders: leaders}
	return ns
}

func rivalEraForState(s model.GameState, b balance.Config) int {
	max := MaxUnlockedGen(s, b)
	era, err := balance.EraForGen(max)
	if err != nil {
		return 1
	}
	return era
}

// selectRivalLeaders picks n distinct companies without replacement, weighted
// by each company's strongest specialty (max Skill).
func selectRivalLeaders(comps []model.Competitor, n int, rng uint64) ([]string, uint64) {
	if n <= 0 || len(comps) == 0 {
		return nil, rng
	}
	type cand struct {
		name   string
		weight float64
	}
	pool := make([]cand, 0, len(comps))
	for _, c := range comps {
		w := 0.0
		for _, sk := range c.Skill {
			if sk > w {
				w = sk
			}
		}
		if w <= 0 {
			w = 1
		}
		pool = append(pool, cand{name: c.Name, weight: w})
	}
	if n > len(pool) {
		n = len(pool)
	}
	out := make([]string, 0, n)
	for len(out) < n && len(pool) > 0 {
		total := 0.0
		for _, c := range pool {
			total += c.weight
		}
		var r float64
		rng, r = nextRand(rng)
		if total <= 0 {
			// Uniform fallback.
			idx := int(r * float64(len(pool)))
			if idx >= len(pool) {
				idx = len(pool) - 1
			}
			out = append(out, pool[idx].name)
			pool = append(pool[:idx], pool[idx+1:]...)
			continue
		}
		pick := r * total
		acc := 0.0
		idx := len(pool) - 1
		for i, c := range pool {
			acc += c.weight
			if pick < acc {
				idx = i
				break
			}
		}
		out = append(out, pool[idx].name)
		pool = append(pool[:idx], pool[idx+1:]...)
	}
	return out, rng
}

func isRivalLeader(s model.GameState, name string) bool {
	for _, n := range s.Progression.Rivals.Leaders {
		if n == name {
			return true
		}
	}
	return false
}

// rivalTarget is the per-dimension quality the rival approaches this tick:
// GlobalFrontier[d] × clamp(Skill[d] + leaderBonus + MomentumPct[d], 0.85, 1.15).
func rivalTarget(s model.GameState, rival model.Competitor, b balance.Config) [model.NumQualityDims]float64 {
	gf := GlobalFrontier(s, b)
	var out [model.NumQualityDims]float64
	lead := 0.0
	if isRivalLeader(s, rival.Name) {
		lead = leaderBonusPct
	}
	for d := range model.NumQualityDims {
		pct := rival.Skill[d] + lead + rival.MomentumPct[d]
		if pct < rivalFloorPct {
			pct = rivalFloorPct
		}
		if pct > rivalCeilPct {
			pct = rivalCeilPct
		}
		out[d] = gf[d] * pct
	}
	return out
}

// clampRivalToBand enforces quality ∈ GlobalFrontier × [0.85, 1.15] per dim.
func clampRivalToBand(q float64, frontier float64) float64 {
	if frontier <= 0 {
		if q < 0 {
			return 0
		}
		return q
	}
	lo := frontier * rivalFloorPct
	hi := frontier * rivalCeilPct
	if q < lo {
		return lo
	}
	if q > hi {
		return hi
	}
	return q
}

// advanceRivalLeague rubber-bands every rival toward its bounded global-frontier
// target. Used for both campaign and non-campaign play (no Tick freeze).
func advanceRivalLeague(s model.GameState, dt float64, b balance.Config) model.GameState {
	if len(s.Competitors) == 0 {
		return s
	}
	ns := ensureRivalEraState(s, b)
	gf := GlobalFrontier(ns, b)
	factor := b.CompetitorCatchupRate * dt
	if factor > 1 {
		factor = 1
	} else if factor < 0 {
		factor = 0
	}
	comps := append([]model.Competitor(nil), ns.Competitors...)
	for i := range comps {
		target := rivalTarget(ns, comps[i], b)
		for d := range model.NumQualityDims {
			comps[i].Quality[d] += (target[d] - comps[i].Quality[d]) * factor
			comps[i].Quality[d] = clampRivalToBand(comps[i].Quality[d], gf[d])
		}
	}
	ns.Competitors = comps
	return ns
}
