package balance

import (
	"errors"
	"math"
)

// ErrInvalidGenerationSpec is returned for gen/era < 1 or non-finite catalog values.
var ErrInvalidGenerationSpec = errors.New("balance: invalid generation spec")

// GenerationSpec is the resolved balance row for one model generation.
// There is no configured maximum generation; Gen11+ are produced by formula.
type GenerationSpec struct {
	Gen                int
	Era                int
	FrontierRnD        float64
	FrontierWork       float64
	TrainRnD           float64
	TrainWork          float64
	QualityScale       float64
	RecommendedCompute float64
	TimeBaselineDay    float64
}

// EraForGen returns the 1-based era for a generation.
// I:1–2, II:3–4, III:5–7, IV:8–10; Gen11+ uses 5 + floor((gen-11)/3).
func EraForGen(gen int) (int, error) {
	if gen < 1 {
		return 0, ErrInvalidGenerationSpec
	}
	switch {
	case gen <= 2:
		return 1, nil
	case gen <= 4:
		return 2, nil
	case gen <= 7:
		return 3, nil
	case gen <= 10:
		return 4, nil
	default:
		return 5 + (gen-11)/3, nil
	}
}

// EraStartGen returns the first generation of the given 1-based era.
func EraStartGen(era int) (int, error) {
	if era < 1 {
		return 0, ErrInvalidGenerationSpec
	}
	switch era {
	case 1:
		return 1, nil
	case 2:
		return 3, nil
	case 3:
		return 5, nil
	case 4:
		return 8, nil
	default:
		return 11 + (era-5)*3, nil
	}
}

// EraEndGen returns the last generation of the given 1-based era.
func EraEndGen(era int) (int, error) {
	start, err := EraStartGen(era)
	if err != nil {
		return 0, err
	}
	switch era {
	case 1, 2:
		return start + 1, nil // two gens
	case 3, 4:
		return start + 2, nil // three gens
	default:
		return start + 2, nil // three gens from Era V onward
	}
}

// Generation resolves the catalog row for gen. gen < 1 returns ErrInvalidGenerationSpec.
func Generation(gen int) (GenerationSpec, error) {
	if gen < 1 {
		return GenerationSpec{}, ErrInvalidGenerationSpec
	}
	era, err := EraForGen(gen)
	if err != nil {
		return GenerationSpec{}, err
	}
	var spec GenerationSpec
	switch {
	case gen <= 5:
		spec = gen1to5(gen)
	case gen <= 10:
		spec = gen6to10(gen)
	default:
		spec, err = gen11plus(gen)
		if err != nil {
			return GenerationSpec{}, err
		}
	}
	spec.Gen = gen
	spec.Era = era
	if err := validateSpec(spec); err != nil {
		return GenerationSpec{}, err
	}
	return spec, nil
}

// gen1–5 training values mirror Default()'s legacy arrays (retired in Task 3).
// Frontier fields are unused before Gen6 frontier projects; TimeBaselineDay is
// the approved industry clock ladder.
func gen1to5(gen int) GenerationSpec {
	// index by gen 1..5
	trainRnD := [6]float64{0, 20000, 150000, 1000000, 6000000, 40000000}
	trainWork := [6]float64{0, 900000, 3600000, 14400000, 57600000, 230400000}
	quality := [6]float64{0, 25, 45, 65, 82, 100}
	baseline := [6]float64{0, 0, 1000, 2500, 4500, 7000}
	return GenerationSpec{
		TrainRnD:        trainRnD[gen],
		TrainWork:       trainWork[gen],
		QualityScale:    quality[gen],
		TimeBaselineDay: baseline[gen],
	}
}

// gen6–10 use the design table; work = recommended × realSeconds × RealSecCompression.
func gen6to10(gen int) GenerationSpec {
	type row struct {
		frontierRnD, frontierSec float64
		trainRnD, trainSec       float64
		quality, recommended     float64
		baseline                 float64
	}
	// keyed by gen
	table := map[int]row{
		6:  {180e6, 2 * 3600, 120e6, 0.5 * 3600, 120, 500, 10000},
		7:  {500e6, 4 * 3600, 350e6, 1 * 3600, 142, 1200, 14000},
		8:  {1.5e9, 6 * 3600, 1e9, 1.5 * 3600, 166, 3000, 20000},
		9:  {4.5e9, 8 * 3600, 3e9, 2 * 3600, 192, 7500, 30000},
		10: {13.5e9, 12 * 3600, 9e9, 3 * 3600, 220, 18000, 40000},
	}
	r := table[gen]
	return GenerationSpec{
		FrontierRnD:        r.frontierRnD,
		FrontierWork:       r.recommended * r.frontierSec * RealSecCompression,
		TrainRnD:           r.trainRnD,
		TrainWork:          r.recommended * r.trainSec * RealSecCompression,
		QualityScale:       r.quality,
		RecommendedCompute: r.recommended,
		TimeBaselineDay:    r.baseline,
	}
}

func gen11plus(gen int) (GenerationSpec, error) {
	g10 := gen6to10(10)
	k := float64(gen - 10)

	fr, err := checkedPow(2.4, k)
	if err != nil {
		return GenerationSpec{}, err
	}
	fw, err := checkedPow(2.6, k)
	if err != nil {
		return GenerationSpec{}, err
	}
	tr, err := checkedPow(2.2, k)
	if err != nil {
		return GenerationSpec{}, err
	}
	tw, err := checkedPow(2.8, k)
	if err != nil {
		return GenerationSpec{}, err
	}
	rc, err := checkedPow(1.75, k)
	if err != nil {
		return GenerationSpec{}, err
	}

	baseline, err := timeBaselineDay(gen)
	if err != nil {
		return GenerationSpec{}, err
	}

	return GenerationSpec{
		FrontierRnD:        g10.FrontierRnD * fr,
		FrontierWork:       g10.FrontierWork * fw,
		TrainRnD:           g10.TrainRnD * tr,
		TrainWork:          g10.TrainWork * tw,
		QualityScale:       220 + 28*k,
		RecommendedCompute: 18000 * rc,
		TimeBaselineDay:    baseline,
	}, nil
}

// timeBaselineDay for gen>=10. Gen9 baseline is 30000; Gen10 is 40000.
// interval(gen) grows ×1.35 from interval(Gen10)=10000.
func timeBaselineDay(gen int) (float64, error) {
	if gen < 10 {
		return 0, ErrInvalidGenerationSpec
	}
	const gen9Baseline = 30000.0
	const gen10Baseline = 40000.0
	if gen == 10 {
		return gen10Baseline, nil
	}
	interval := gen10Baseline - gen9Baseline // 10000
	baseline := gen10Baseline
	for g := 11; g <= gen; g++ {
		next, err := checkedMul(interval, 1.35)
		if err != nil {
			return 0, err
		}
		interval = next
		baseline, err = checkedAdd(baseline, interval)
		if err != nil {
			return 0, err
		}
	}
	return baseline, nil
}

func checkedPow(base, exp float64) (float64, error) {
	v := math.Pow(base, exp)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, ErrInvalidGenerationSpec
	}
	return v, nil
}

func checkedMul(a, b float64) (float64, error) {
	v := a * b
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, ErrInvalidGenerationSpec
	}
	return v, nil
}

func checkedAdd(a, b float64) (float64, error) {
	v := a + b
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, ErrInvalidGenerationSpec
	}
	return v, nil
}

func validateSpec(s GenerationSpec) error {
	// Gen1–5 have zero frontier / recommended (no frontier projects yet).
	// Training and quality must still be positive; Gen1 TimeBaselineDay is 0.
	if s.TrainRnD <= 0 || s.TrainWork <= 0 || s.QualityScale <= 0 {
		return ErrInvalidGenerationSpec
	}
	if s.TimeBaselineDay < 0 || math.IsNaN(s.TimeBaselineDay) || math.IsInf(s.TimeBaselineDay, 0) {
		return ErrInvalidGenerationSpec
	}
	if s.Gen >= 6 {
		fields := []float64{
			s.FrontierRnD, s.FrontierWork, s.RecommendedCompute,
		}
		for _, v := range fields {
			if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
				return ErrInvalidGenerationSpec
			}
		}
	}
	for _, v := range []float64{s.TrainRnD, s.TrainWork, s.QualityScale} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return ErrInvalidGenerationSpec
		}
	}
	return nil
}
