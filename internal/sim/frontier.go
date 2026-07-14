package sim

import (
	"math"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// ModelFrontier is a stable, relative view of one stored model versus the
// current global frontier. AbsoluteQuality is the stored value and is never
// rewritten by frontier movement.
type ModelFrontier struct {
	Active           bool
	ModelGen         int
	AbsoluteQuality  [model.NumQualityDims]float64
	GlobalFrontier   [model.NumQualityDims]float64
	FrontierDeltaPct [model.NumQualityDims]float64 // absolute/global - 1; 0 if global==0
	EquivalentGen    float64                       // fractional gen from global quality scale
	GenerationGap    float64                       // EquivalentGen - ModelGen
}

// PlayerFrontier is the best online player-model quality per dimension.
// Drafts and offline models are ignored. Pure: does not mutate s.
func PlayerFrontier(s model.GameState) [model.NumQualityDims]float64 {
	return playerFrontier(s)
}

// TimeFrontier interpolates industry progress along generation baseline days
// and scales each dimension from CompetitorBaseQuality / Gen1.QualityScale.
// Uses EffectiveIndustryDay (IndustryTime clamped to MaxUnlockedGen+Lead),
// not raw GameTime, so AFK cannot push the industry scale unbounded past the player.
func TimeFrontier(s model.GameState, b balance.Config) [model.NumQualityDims]float64 {
	var out [model.NumQualityDims]float64
	g1, err := balance.Generation(1)
	if err != nil || g1.QualityScale <= 0 {
		return out
	}
	scale := interpolatedQualityScale(EffectiveIndustryDay(s, b))
	val := b.CompetitorBaseQuality * scale / g1.QualityScale
	if math.IsNaN(val) || math.IsInf(val, 0) || val < 0 {
		val = 0
	}
	for d := range model.NumQualityDims {
		out[d] = val
	}
	return out
}

// SecondsUntilNextTimeGeneration is game-seconds of industry clock until the
// next generation TimeBaselineDay is reached. Offline industry allowance uses
// this residual (not the full current generation interval width), so a settle
// never crosses the next baseline in one go — a conservative reading of
// design §8.2 "at most one generation of time-frontier movement".
// Returns 0 when no further baseline is resolvable.
func SecondsUntilNextTimeGeneration(s model.GameState, _ balance.Config) float64 {
	day := s.Progression.IndustryTime / 86400
	if day < 0 {
		day = 0
	}
	for gen := 1; gen <= 500; gen++ {
		g, err := balance.Generation(gen)
		if err != nil {
			return 0
		}
		if g.TimeBaselineDay > day+1e-12 {
			return (g.TimeBaselineDay - day) * 86400
		}
	}
	return 0
}

// GlobalFrontier is the per-dimension max of player and time frontiers.
func GlobalFrontier(s model.GameState, b balance.Config) [model.NumQualityDims]float64 {
	p := PlayerFrontier(s)
	t := TimeFrontier(s, b)
	var out [model.NumQualityDims]float64
	for d := range model.NumQualityDims {
		out[d] = p[d]
		if t[d] > out[d] {
			out[d] = t[d]
		}
	}
	return out
}

// ModelFrontierView projects model index against the global frontier.
// Invalid indices return a zero view with Active=false.
func ModelFrontierView(s model.GameState, index int, b balance.Config) ModelFrontier {
	if index < 0 || index >= len(s.Models) {
		return ModelFrontier{}
	}
	m := s.Models[index]
	gf := GlobalFrontier(s, b)
	v := ModelFrontier{
		Active:          true,
		ModelGen:        m.Gen,
		AbsoluteQuality: m.Quality,
		GlobalFrontier:  gf,
	}
	for d := range model.NumQualityDims {
		if gf[d] > simEpsilon {
			v.FrontierDeltaPct[d] = m.Quality[d]/gf[d] - 1
		} else {
			v.FrontierDeltaPct[d] = 0
		}
	}
	v.EquivalentGen = equivalentGenFromFrontier(gf, b)
	v.GenerationGap = v.EquivalentGen - float64(m.Gen)
	return v
}

// interpolatedQualityScale returns the catalog QualityScale at industryDay
// (game days), linearly interpolating between generation baseline days.
func interpolatedQualityScale(industryDay float64) float64 {
	if industryDay < 0 {
		industryDay = 0
	}
	prev, err := balance.Generation(1)
	if err != nil {
		return 0
	}
	if industryDay <= prev.TimeBaselineDay {
		return prev.QualityScale
	}
	// Walk generations until baseline exceeds industryDay (catalog has no max).
	for gen := 2; gen <= 500; gen++ {
		cur, err := balance.Generation(gen)
		if err != nil {
			return prev.QualityScale
		}
		if industryDay <= cur.TimeBaselineDay {
			span := cur.TimeBaselineDay - prev.TimeBaselineDay
			if span <= simEpsilon {
				return cur.QualityScale
			}
			t := (industryDay - prev.TimeBaselineDay) / span
			return prev.QualityScale + t*(cur.QualityScale-prev.QualityScale)
		}
		prev = cur
	}
	return prev.QualityScale
}

// equivalentGenFromFrontier reverse-maps the global frontier's capability
// (time-scaled units) onto a fractional generation via catalog QualityScale.
func equivalentGenFromFrontier(gf [model.NumQualityDims]float64, b balance.Config) float64 {
	g1, err := balance.Generation(1)
	if err != nil || g1.QualityScale <= 0 || b.CompetitorBaseQuality <= simEpsilon {
		return 1
	}
	// Prefer the strongest frontier dim for explanatory generation.
	front := 0.0
	for d := range model.NumQualityDims {
		if gf[d] > front {
			front = gf[d]
		}
	}
	if front <= simEpsilon {
		return 1
	}
	scale := front * g1.QualityScale / b.CompetitorBaseQuality
	return equivalentGenFromQualityScale(scale)
}

func equivalentGenFromQualityScale(scale float64) float64 {
	if scale <= 0 {
		return 1
	}
	prev, err := balance.Generation(1)
	if err != nil {
		return 1
	}
	if scale <= prev.QualityScale {
		return 1
	}
	for gen := 2; gen <= 500; gen++ {
		cur, err := balance.Generation(gen)
		if err != nil {
			return float64(gen - 1)
		}
		if scale <= cur.QualityScale {
			span := cur.QualityScale - prev.QualityScale
			if span <= simEpsilon {
				return float64(gen)
			}
			t := (scale - prev.QualityScale) / span
			return float64(gen-1) + t
		}
		prev = cur
	}
	return 500
}
