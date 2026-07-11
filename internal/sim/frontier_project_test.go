package sim

import (
	"errors"
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestApplyStartFrontierProjectGen5StartsGen6(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Progression.MaxUnlockedGen = 5
	spec, err := balance.Generation(6)
	if err != nil {
		t.Fatal(err)
	}

	ns, err := Apply(s, model.StartFrontierProject{TargetGen: 6}, b)
	if err != nil {
		t.Fatalf("start Gen6: %v", err)
	}
	fp := ns.Progression.Frontier
	if !fp.Active || fp.TargetGen != 6 {
		t.Fatalf("frontier not active for Gen6: %+v", fp)
	}
	if !approx(fp.RnDTotal, spec.FrontierRnD) || !approx(fp.RnDRemaining, spec.FrontierRnD) {
		t.Fatalf("RnD snapshot wrong: total=%v rem=%v want %v", fp.RnDTotal, fp.RnDRemaining, spec.FrontierRnD)
	}
	if !approx(fp.WorkTotal, spec.FrontierWork) || !approx(fp.WorkRemaining, spec.FrontierWork) {
		t.Fatalf("work snapshot wrong: total=%v rem=%v want %v", fp.WorkTotal, fp.WorkRemaining, spec.FrontierWork)
	}
	if !approx(fp.RecommendedCompute, spec.RecommendedCompute) {
		t.Fatalf("recommended = %v, want %v", fp.RecommendedCompute, spec.RecommendedCompute)
	}
	if fp.AllocationPct != 100 {
		t.Fatalf("default AllocationPct = %d, want 100", fp.AllocationPct)
	}
	// Input purity.
	if s.Progression.Frontier.Active {
		t.Fatal("Apply mutated input frontier")
	}
}

func TestApplyStartFrontierProjectWrongTargetAndInvalid(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Progression.MaxUnlockedGen = 5

	// Only MaxUnlockedGen+1 (Gen6) is allowed — Gen7 is wrong target.
	out, err := Apply(s, model.StartFrontierProject{TargetGen: 7}, b)
	if err != ErrInvalidFrontierTarget {
		t.Fatalf("Gen7 from Gen5: err = %v, want ErrInvalidFrontierTarget", err)
	}
	if out.Progression.Frontier.Active {
		t.Fatal("wrong target mutated state")
	}
	// Gen5 training-style unlock path is not a frontier project.
	if _, err := Apply(s, model.StartFrontierProject{TargetGen: 5}, b); err != ErrInvalidFrontierTarget {
		t.Fatalf("Gen5 frontier: err = %v, want ErrInvalidFrontierTarget", err)
	}
	// Invalid generation (catalog).
	out, err = Apply(s, model.StartFrontierProject{TargetGen: 0}, b)
	if !errors.Is(err, balance.ErrInvalidGenerationSpec) {
		t.Fatalf("gen0: err = %v, want ErrInvalidGenerationSpec", err)
	}
	if out.Progression.Frontier.Active {
		t.Fatal("invalid gen mutated state")
	}
}

func TestApplyStartFrontierProjectSecondProjectFails(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Progression.MaxUnlockedGen = 5
	ns, err := Apply(s, model.StartFrontierProject{TargetGen: 6}, b)
	if err != nil {
		t.Fatal(err)
	}
	// Mutate remaining to prove purity on failed second start.
	ns.Progression.Frontier.WorkRemaining = 123
	before := ns.Progression.Frontier
	out, err := Apply(ns, model.StartFrontierProject{TargetGen: 6}, b)
	if err != ErrFrontierActive {
		t.Fatalf("second project: err = %v, want ErrFrontierActive", err)
	}
	if out.Progression.Frontier != before {
		t.Fatalf("second start mutated frontier: %+v", out.Progression.Frontier)
	}
}

func TestApplyStartFrontierProjectEraGates(t *testing.T) {
	b := balance.Default()
	// Gen7 unlocked → next frontier target is Gen8 (Era IV).
	s := model.GameState{}
	s.Progression.MaxUnlockedGen = 7
	// Era IV closed without two Era III breakthroughs.
	if _, err := Apply(s, model.StartFrontierProject{TargetGen: 8}, b); err != ErrEraNotOpen {
		t.Fatalf("Gen8 without era gate: err = %v, want ErrEraNotOpen", err)
	}
	if s.Progression.Frontier.Active {
		t.Fatal("era gate failure mutated input")
	}
	// Open Era IV: two Era III breakthroughs already present with Gen7.
	s.Progression.Eras = []model.EraProgress{{
		Era: 3, HasPrimary: true, Primary: model.BranchAlgo,
		UnlockedMask: (1 << model.BranchAlgo) | (1 << model.BranchInfra),
	}}
	ns, err := Apply(s, model.StartFrontierProject{TargetGen: 8}, b)
	if err != nil {
		t.Fatalf("Gen8 with era open: %v", err)
	}
	spec, _ := balance.Generation(8)
	if !ns.Progression.Frontier.Active || ns.Progression.Frontier.TargetGen != 8 {
		t.Fatalf("frontier: %+v", ns.Progression.Frontier)
	}
	if !approx(ns.Progression.Frontier.RnDTotal, spec.FrontierRnD) {
		t.Fatalf("Gen8 RnD snapshot = %v, want %v", ns.Progression.Frontier.RnDTotal, spec.FrontierRnD)
	}
}

func TestApplySetFrontierAllocation(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Progression.MaxUnlockedGen = 5
	ns, err := Apply(s, model.StartFrontierProject{TargetGen: 6}, b)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate partial progress that allocation must preserve.
	ns.Progression.Frontier.RnDRemaining = ns.Progression.Frontier.RnDTotal * 0.4
	ns.Progression.Frontier.WorkRemaining = ns.Progression.Frontier.WorkTotal * 0.4

	for _, pct := range []int{0, 10, 100} {
		next, err := Apply(ns, model.SetFrontierAllocation{Percent: pct}, b)
		if err != nil {
			t.Fatalf("alloc %d: %v", pct, err)
		}
		if next.Progression.Frontier.AllocationPct != pct {
			t.Fatalf("AllocationPct = %d, want %d", next.Progression.Frontier.AllocationPct, pct)
		}
		if !approx(next.Progression.Frontier.RnDRemaining, ns.Progression.Frontier.RnDRemaining) ||
			!approx(next.Progression.Frontier.WorkRemaining, ns.Progression.Frontier.WorkRemaining) ||
			!approx(next.Progression.Frontier.RnDTotal, ns.Progression.Frontier.RnDTotal) ||
			!approx(next.Progression.Frontier.WorkTotal, ns.Progression.Frontier.WorkTotal) {
			t.Fatalf("alloc %d rewrote progress: %+v", pct, next.Progression.Frontier)
		}
		// Keep using updated allocation for next iteration base? Use original ns
		// with preserved progress each time.
	}

	// Reject out of range without mutation.
	for _, bad := range []int{-1, 101} {
		out, err := Apply(ns, model.SetFrontierAllocation{Percent: bad}, b)
		if err != ErrInvalidFrontierAllocation {
			t.Fatalf("alloc %d: err = %v, want ErrInvalidFrontierAllocation", bad, err)
		}
		if out.Progression.Frontier.AllocationPct != ns.Progression.Frontier.AllocationPct {
			t.Fatalf("bad alloc %d mutated percent", bad)
		}
	}
}

func TestApplySetFrontierAllocationRequiresProject(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	out, err := Apply(s, model.SetFrontierAllocation{Percent: 50}, b)
	if err != ErrNoFrontierProject {
		t.Fatalf("err = %v, want ErrNoFrontierProject", err)
	}
	if out.Progression.Frontier.AllocationPct != 0 {
		t.Fatalf("no-project alloc mutated state")
	}
}
