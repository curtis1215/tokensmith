package sim

import (
	"errors"
	"math"
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

func TestDiminishedFrontierCompute(t *testing.T) {
	// Linear at and below recommendation.
	if got := diminishedFrontierCompute(0, 100); got != 0 {
		t.Fatalf("0 alloc = %v, want 0", got)
	}
	if got := diminishedFrontierCompute(50, 100); !approx(got, 50) {
		t.Fatalf("below rec = %v, want 50", got)
	}
	if got := diminishedFrontierCompute(100, 100); !approx(got, 100) {
		t.Fatalf("at rec = %v, want 100", got)
	}
	// Above recommendation: R + sqrt((A-R)*R)
	got := diminishedFrontierCompute(400, 100)
	want := 100 + math.Sqrt((400-100)*100) // 100 + sqrt(30000) ≈ 273.205
	if !approx(got, want) {
		t.Fatalf("above rec = %v, want %v", got, want)
	}
	// Extra compute still helps (strictly increasing past R).
	if diminishedFrontierCompute(401, 100) <= got {
		t.Fatal("diminishing must remain strictly increasing")
	}
}

func TestTickSharesTrainingCompute(t *testing.T) {
	b := balance.Default()
	// 100 compute, 60% frontier / 40% training.
	s := model.GameState{HasTraining: true}
	s.Servers = []model.Server{{Pool: model.PoolTraining, Compute: 100}}
	s.Training = model.TrainingJob{Gen: 1, WorkRemaining: 1e9}
	s.Progression.MaxUnlockedGen = 5
	s.Progression.Frontier = model.FrontierProject{
		Active:             true,
		TargetGen:          6,
		RnDTotal:           1e12,
		RnDRemaining:       1e12,
		WorkTotal:          1e12,
		WorkRemaining:      1e12,
		RecommendedCompute: 1e9, // linear regime for allocated 60
		AllocationPct:      60,
	}
	s.Resources.RnD = 1e18 // plenty so frontier is not R&D-gated
	const dt = 10.0
	ns := Tick(s, dt, nil, b)
	// Training gets 40 compute × 10s = 400 work.
	if !approx(ns.Training.WorkRemaining, 1e9-400) {
		t.Fatalf("training work = %v, want %v", ns.Training.WorkRemaining, 1e9-400)
	}
	// Frontier gets 60 compute × 10s = 600 work (linear).
	if !approx(ns.Progression.Frontier.WorkRemaining, 1e12-600) {
		t.Fatalf("frontier work = %v, want %v", ns.Progression.Frontier.WorkRemaining, 1e12-600)
	}
	// Idle shares: 100% frontier, no training — training job absent leaves model share idle.
	onlyFront := s
	onlyFront.HasTraining = false
	onlyFront.Training = model.TrainingJob{}
	onlyFront.Progression.Frontier.AllocationPct = 40
	nf := Tick(onlyFront, dt, nil, b)
	// Only 40% of compute goes to frontier; 60% stays idle (not redirected).
	if !approx(nf.Progression.Frontier.WorkRemaining, 1e12-400) {
		t.Fatalf("idle model share must not redirect: frontier work = %v, want %v",
			nf.Progression.Frontier.WorkRemaining, 1e12-400)
	}
}

func TestFrontierProjectRnDStall(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Servers = []model.Server{{Pool: model.PoolTraining, Compute: 100}}
	s.Resources.RnD = 0 // stall
	s.Progression.MaxUnlockedGen = 5
	s.Progression.Frontier = model.FrontierProject{
		Active:             true,
		TargetGen:          6,
		RnDTotal:           1000,
		RnDRemaining:       1000,
		WorkTotal:          500,
		WorkRemaining:      500,
		RecommendedCompute: 50,
		AllocationPct:      100,
	}
	before := s.Progression.Frontier
	ns := Tick(s, 10, nil, b)
	if ns.Resources.RnD != 0 {
		t.Fatalf("RnD should stay 0 without staff, got %v", ns.Resources.RnD)
	}
	if !approx(ns.Progression.Frontier.WorkRemaining, before.WorkRemaining) ||
		!approx(ns.Progression.Frontier.RnDRemaining, before.RnDRemaining) {
		t.Fatalf("stall advanced frontier: %+v", ns.Progression.Frontier)
	}
	if !ns.Progression.Frontier.Active {
		t.Fatal("stall must not complete the project")
	}
}

func TestFrontierProjectProportionalSpend(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Servers = []model.Server{{Pool: model.PoolTraining, Compute: 10}}
	s.Resources.RnD = 1e9
	s.Progression.MaxUnlockedGen = 5
	// Ratio RnD/Work = 2
	s.Progression.Frontier = model.FrontierProject{
		Active:             true,
		TargetGen:          6,
		RnDTotal:           200,
		RnDRemaining:       200,
		WorkTotal:          100,
		WorkRemaining:      100,
		RecommendedCompute: 1000, // linear: allocated 10
		AllocationPct:      100,
	}
	const dt = 2.0 // workByCompute = 10*2 = 20
	ns := Tick(s, dt, nil, b)
	// workDone=20, rndPerWork=2, spend=40
	if !approx(ns.Progression.Frontier.WorkRemaining, 80) {
		t.Fatalf("work rem = %v, want 80", ns.Progression.Frontier.WorkRemaining)
	}
	if !approx(ns.Progression.Frontier.RnDRemaining, 160) {
		t.Fatalf("rnd rem = %v, want 160", ns.Progression.Frontier.RnDRemaining)
	}
	if !approx(ns.Resources.RnD, 1e9-40) {
		t.Fatalf("wallet RnD = %v, want %v", ns.Resources.RnD, 1e9-40)
	}
	// Remaining ratio preserved: 160/80 = 2
	ratio := ns.Progression.Frontier.RnDRemaining / ns.Progression.Frontier.WorkRemaining
	if !approx(ratio, 2) {
		t.Fatalf("remaining ratio = %v, want 2", ratio)
	}
}

func TestFrontierProjectCompletion(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Servers = []model.Server{{Pool: model.PoolTraining, Compute: 1000}}
	s.Resources.RnD = 1e18
	s.Progression.MaxUnlockedGen = 5
	s.Progression.Frontier = model.FrontierProject{
		Active:             true,
		TargetGen:          6,
		RnDTotal:           100,
		RnDRemaining:       10,
		WorkTotal:          100,
		WorkRemaining:      10,
		RecommendedCompute: 100,
		AllocationPct:      100,
	}
	ns := Tick(s, 1, nil, b) // 1000 compute >> 10 work
	if ns.Progression.Frontier.Active {
		t.Fatalf("project should clear on completion: %+v", ns.Progression.Frontier)
	}
	if ns.Progression.MaxUnlockedGen != 6 {
		t.Fatalf("MaxUnlockedGen = %d, want 6", ns.Progression.MaxUnlockedGen)
	}
	// Totals zeroed.
	if ns.Progression.Frontier != (model.FrontierProject{}) {
		t.Fatalf("frontier not zeroed: %+v", ns.Progression.Frontier)
	}
	// Second tick never unlocks twice / does not regress.
	ns2 := Tick(ns, 1, nil, b)
	if ns2.Progression.MaxUnlockedGen != 6 {
		t.Fatalf("second tick changed max gen: %d", ns2.Progression.MaxUnlockedGen)
	}
}

func TestFrontierProjectAllocationPreservesTotals(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Progression.MaxUnlockedGen = 5
	s.Progression.Frontier = model.FrontierProject{
		Active:             true,
		TargetGen:          6,
		RnDTotal:           500,
		RnDRemaining:       123,
		WorkTotal:          800,
		WorkRemaining:      456,
		RecommendedCompute: 500,
		AllocationPct:      100,
	}
	ns, err := Apply(s, model.SetFrontierAllocation{Percent: 30}, b)
	if err != nil {
		t.Fatal(err)
	}
	if ns.Progression.Frontier.AllocationPct != 30 {
		t.Fatalf("pct = %d", ns.Progression.Frontier.AllocationPct)
	}
	if ns.Progression.Frontier.RnDTotal != 500 || ns.Progression.Frontier.WorkTotal != 800 ||
		ns.Progression.Frontier.RnDRemaining != 123 || ns.Progression.Frontier.WorkRemaining != 456 {
		t.Fatalf("totals/remaining rewritten: %+v", ns.Progression.Frontier)
	}
}

func TestTickFrontierPurityNestedInputs(t *testing.T) {
	b := balance.Default()
	s := model.GameState{HasTraining: true}
	s.Compute.RentedTraining = map[string]int{"N7": 4}
	s.Servers = []model.Server{{Pool: model.PoolTraining, Compute: 10}}
	s.Models = []model.Model{{Gen: 1, Online: false}}
	s.Training = model.TrainingJob{Gen: 1, WorkRemaining: 1e6}
	s.Resources.RnD = 1e12
	s.Progression.MaxUnlockedGen = 5
	s.Progression.Frontier = model.FrontierProject{
		Active: true, TargetGen: 6,
		RnDTotal: 1e6, RnDRemaining: 1e6,
		WorkTotal: 1e6, WorkRemaining: 1e6,
		RecommendedCompute: 100, AllocationPct: 50,
	}
	s.Progression.Eras = []model.EraProgress{{Era: 3, UnlockedMask: 1}}
	// Snapshot nested identities.
	trainMap := s.Compute.RentedTraining
	servers := s.Servers
	models := s.Models
	eras := s.Progression.Eras
	_ = Tick(s, 1, nil, b)
	if trainMap["N7"] != 4 || s.Compute.RentedTraining["N7"] != 4 {
		t.Fatal("rented map mutated")
	}
	if &servers[0] != &s.Servers[0] {
		// header may differ but element identity for unused clone paths
	}
	if servers[0].Compute != 10 || s.Servers[0].Compute != 10 {
		t.Fatal("servers mutated")
	}
	if models[0].Gen != 1 || s.Models[0].Gen != 1 {
		t.Fatal("models mutated")
	}
	if eras[0].Era != 3 || s.Progression.Eras[0].Era != 3 {
		t.Fatal("eras mutated")
	}
	if !s.Progression.Frontier.Active || s.Progression.Frontier.WorkRemaining != 1e6 {
		t.Fatalf("input frontier mutated: %+v", s.Progression.Frontier)
	}
}
