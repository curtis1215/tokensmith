package sim

import (
	"errors"
	"math"
	"strings"
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestApplyRentTrainingComputeAdds(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Compute.RentedTraining = map[string]int{"N7": 2}
	ns, err := Apply(s, model.RentCompute{Process: "N7", Pool: model.PoolTraining, Delta: 3}, b)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ns.Compute.RentedTraining["N7"] != 5 {
		t.Fatalf("capacity = %v, want 5", ns.Compute.RentedTraining["N7"])
	}
	// input not mutated
	if s.Compute.RentedTraining["N7"] != 2 {
		t.Fatalf("Apply mutated input: %v", s.Compute.RentedTraining["N7"])
	}
}

func TestApplyRentTrainingComputeFloorsAtZero(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Compute.RentedTraining = map[string]int{"N7": 2}
	ns, _ := Apply(s, model.RentCompute{Process: "N7", Pool: model.PoolTraining, Delta: -5}, b)
	if ns.Compute.RentedTraining["N7"] != 0 {
		t.Fatalf("capacity = %v, want 0", ns.Compute.RentedTraining["N7"])
	}
}

func validAlloc() [model.NumQualityDims]float64 {
	return [model.NumQualityDims]float64{0.4, 0.2, 0.2, 0.2}
}

func TestApplyStartTrainingSuccess(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Resources.RnD = 50000 // > Gen1 cost 20000
	cmd := model.StartTraining{Gen: 1, Alloc: validAlloc(), Price: 12}
	ns, err := Apply(s, cmd, b)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ns.Resources.RnD != 30000 { // 50000 - 20000
		t.Errorf("RnD = %v, want 30000", ns.Resources.RnD)
	}
	if !ns.HasTraining || ns.Training.Gen != 1 || ns.Training.Price != 12 {
		t.Errorf("training not set: %+v", ns.Training)
	}
	if ns.Training.WorkRemaining != 900000 {
		t.Errorf("WorkRemaining = %v, want 900000", ns.Training.WorkRemaining)
	}
	if s.HasTraining {
		t.Errorf("Apply mutated input")
	}
}

func TestApplyStartTrainingErrors(t *testing.T) {
	b := balance.Default()
	base := model.GameState{}
	base.Resources.RnD = 50000

	// already training
	busy := base
	busy.HasTraining = true
	if _, err := Apply(busy, model.StartTraining{Gen: 1, Alloc: validAlloc()}, b); err != ErrTrainingInProgress {
		t.Errorf("busy: err = %v, want ErrTrainingInProgress", err)
	}
	// invalid gen (catalog rejects gen < 1)
	if _, err := Apply(base, model.StartTraining{Gen: 0, Alloc: validAlloc()}, b); !errors.Is(err, balance.ErrInvalidGenerationSpec) {
		t.Errorf("gen: err = %v, want ErrInvalidGenerationSpec", err)
	}
	// Gen9 is a valid catalog row but still locked without progression unlock.
	if _, err := Apply(base, model.StartTraining{Gen: 9, Alloc: validAlloc(), Price: 12}, b); err != ErrGenLocked {
		t.Errorf("gen9 locked: err = %v, want ErrGenLocked", err)
	}
	// bad alloc (sums to 0.8)
	bad := [model.NumQualityDims]float64{0.4, 0.2, 0.1, 0.1}
	if _, err := Apply(base, model.StartTraining{Gen: 1, Alloc: bad}, b); err != ErrInvalidAlloc {
		t.Errorf("alloc: err = %v, want ErrInvalidAlloc", err)
	}
	// non-positive price (validated before the R&D check)
	if _, err := Apply(base, model.StartTraining{Gen: 1, Alloc: validAlloc(), Price: 0}, b); err != ErrInvalidPrice {
		t.Errorf("price: err = %v, want ErrInvalidPrice", err)
	}
	// insufficient R&D
	poor := model.GameState{}
	poor.Resources.RnD = 100
	if _, err := Apply(poor, model.StartTraining{Gen: 1, Alloc: validAlloc(), Price: 12}, b); err != ErrInsufficientRnD {
		t.Errorf("poor: err = %v, want ErrInsufficientRnD", err)
	}
}

func TestApplyStartTrainingWithBoostsChargesCashAndFreezesBonus(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Resources.RnD = 50_000
	s.Resources.Cash = 1e9
	var boosts [model.NumQualityDims]bool
	boosts[model.DimSafety] = true
	cost, err := QuoteTrainBoostCost(s, 1, boosts, b)
	if err != nil {
		t.Fatal(err)
	}
	ns, err := Apply(s, model.StartTraining{
		Gen: 1, Alloc: validAlloc(), Price: 12, Boosts: boosts,
	}, b)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(ns.Resources.Cash-(1e9-cost)) > 1e-6 {
		t.Fatalf("cash = %v, want %v", ns.Resources.Cash, 1e9-cost)
	}
	if !ns.Training.Boosts[model.DimSafety] {
		t.Fatal("boost not frozen on job")
	}
	wantBonus := b.TrainBoostBeta * 25 // Gen1 scale
	if math.Abs(ns.Training.CashBonus[model.DimSafety]-wantBonus) > 1e-9 {
		t.Fatalf("CashBonus = %v", ns.Training.CashBonus)
	}
	if math.Abs(ns.Training.BoostCashPaid-cost) > 1e-6 {
		t.Fatalf("BoostCashPaid = %v, want %v", ns.Training.BoostCashPaid, cost)
	}
}

func TestApplyStartTrainingInsufficientCashForBoost(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Resources.RnD = 50_000
	s.Resources.Cash = 1 // too low for any boost at floor
	var boosts [model.NumQualityDims]bool
	boosts[0] = true
	out, err := Apply(s, model.StartTraining{Gen: 1, Alloc: validAlloc(), Price: 12, Boosts: boosts}, b)
	if err != ErrInsufficientCash {
		t.Fatalf("err = %v, want ErrInsufficientCash", err)
	}
	if out.HasTraining || out.Resources.RnD != 50_000 || out.Resources.Cash != 1 {
		t.Fatalf("state mutated on failure: %+v", out.Resources)
	}
}

func TestApplyStartTrainingRejectsNegativeRoleWeightConfig(t *testing.T) {
	b := balance.Default()
	for i := range b.TrainBoosts {
		if b.TrainBoosts[i].Dim == model.DimCapability {
			b.TrainBoosts[i].RoleWeight = -0.1
		}
	}
	s := model.GameState{}
	s.Resources.RnD = 50_000
	s.Resources.Cash = 0
	var boosts [model.NumQualityDims]bool
	boosts[model.DimCapability] = true
	out, err := Apply(s, model.StartTraining{Gen: 1, Alloc: validAlloc(), Price: 12, Boosts: boosts}, b)
	if !errors.Is(err, balance.ErrInvalidTrainBoostConfig) {
		t.Fatalf("err = %v, want ErrInvalidTrainBoostConfig", err)
	}
	if out.HasTraining || out.Resources.Cash != 0 || out.Resources.RnD != 50_000 {
		t.Fatalf("must not mint cash or start training: cash=%v rnd=%v training=%v",
			out.Resources.Cash, out.Resources.RnD, out.HasTraining)
	}
}

func TestApplyStartTrainingRejectsNaNPainMult(t *testing.T) {
	b := balance.Default()
	b.TrainBoostPainMult = math.NaN()
	s := model.GameState{}
	s.Resources.RnD = 50_000
	s.Resources.Cash = 1e9
	var boosts [model.NumQualityDims]bool
	boosts[0] = true
	out, err := Apply(s, model.StartTraining{Gen: 1, Alloc: validAlloc(), Price: 12, Boosts: boosts}, b)
	if !errors.Is(err, balance.ErrInvalidTrainBoostConfig) {
		t.Fatalf("err = %v, want ErrInvalidTrainBoostConfig", err)
	}
	if out.HasTraining || math.IsNaN(out.Resources.Cash) {
		t.Fatalf("must not train or write NaN cash: %+v", out.Resources)
	}
}

func TestAdvanceTrainingAppliesFrozenCashBonus(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.HasTraining = true
	s.Training = model.TrainingJob{
		Gen: 1, Alloc: validAlloc(), Price: 12, WorkRemaining: 1,
		Boosts:    [model.NumQualityDims]bool{false, false, true, false},
		CashBonus: [model.NumQualityDims]float64{0, 0, 3.75, 0},
	}
	s.Compute.RentedTraining = map[string]int{"N7": 100}
	ns := advanceTraining(s, 1e12, 1e6, b)
	if len(ns.Models) == 0 {
		t.Fatal("no draft model")
	}
	m := ns.Models[len(ns.Models)-1]
	// safety = 0.2*25 + 3.75 = 8.75 (no tech/star)
	if math.Abs(m.Quality[model.DimSafety]-8.75) > 1e-6 {
		t.Fatalf("safety = %v, want 8.75", m.Quality[model.DimSafety])
	}
	// capability unchanged path: 0.4*25 = 10
	if math.Abs(m.Quality[model.DimCapability]-10) > 1e-6 {
		t.Fatalf("cap = %v, want 10", m.Quality[model.DimCapability])
	}
}

func TestStartTrainingCarriesSegment(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Resources.RnD = 50000
	s.Compute.RentedTraining = map[string]int{"N7": 100} // finish fast
	cmd := model.StartTraining{Gen: 1, Segment: model.SegEnterprise, Alloc: validAlloc(), Price: 180}
	ns, err := Apply(s, cmd, b)
	if err != nil {
		t.Fatal(err)
	}
	if ns.Training.Segment != model.SegEnterprise {
		t.Fatalf("job segment = %v, want Enterprise", ns.Training.Segment)
	}
	// tick to completion; the online model must keep the segment
	for i := 0; i < 100 && ns.HasTraining; i++ {
		ns = Tick(ns, 3600, nil, b)
	}
	if len(ns.Models) == 0 || ns.Models[len(ns.Models)-1].Segment != model.SegEnterprise {
		t.Fatalf("completed model segment wrong: %+v", ns.Models)
	}
}

func TestMaxUnlockedGen(t *testing.T) {
	b := balance.Default()
	// Zero-value legacy state still allows Gen1.
	s := model.GameState{}
	if MaxUnlockedGen(s, b) != 1 {
		t.Fatalf("fresh game should allow only Gen1, got %d", MaxUnlockedGen(s, b))
	}
	// Contiguous legacy model-gen-2..5 nodes raise the gate.
	s.UnlockedTech = []string{balance.GenUnlockNodeID(2), balance.GenUnlockNodeID(3)}
	if MaxUnlockedGen(s, b) != 3 {
		t.Fatalf("gen-2+gen-3 unlocked → max 3, got %d", MaxUnlockedGen(s, b))
	}
	// chain gap: gen-3 unlocked but not gen-2 → still 1
	s.UnlockedTech = []string{balance.GenUnlockNodeID(3)}
	if MaxUnlockedGen(s, b) != 1 {
		t.Fatalf("gen-3 without gen-2 → max 1, got %d", MaxUnlockedGen(s, b))
	}
	// Full Gen2–5 chain.
	s.UnlockedTech = []string{
		balance.GenUnlockNodeID(2), balance.GenUnlockNodeID(3),
		balance.GenUnlockNodeID(4), balance.GenUnlockNodeID(5),
	}
	if MaxUnlockedGen(s, b) != 5 {
		t.Fatalf("gen-2..5 → max 5, got %d", MaxUnlockedGen(s, b))
	}
	// Progression.MaxUnlockedGen alone can open Gen6+ (frontier unlock path).
	s = model.GameState{}
	s.Progression.MaxUnlockedGen = 6
	if MaxUnlockedGen(s, b) != 6 {
		t.Fatalf("Progression.MaxUnlockedGen=6 → max 6, got %d", MaxUnlockedGen(s, b))
	}
	// Progression and legacy combine via max.
	s.UnlockedTech = []string{balance.GenUnlockNodeID(2), balance.GenUnlockNodeID(3)}
	s.Progression.MaxUnlockedGen = 1
	if MaxUnlockedGen(s, b) != 3 {
		t.Fatalf("legacy gen-3 with Progression=1 → max 3, got %d", MaxUnlockedGen(s, b))
	}
}

func TestUnlockTechUpdatesProgression(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Resources.RnD = 1e12
	s.Progression.MaxUnlockedGen = 1
	// Unlock Gen2 node → Progression advances atomically.
	ns, err := Apply(s, model.UnlockTech{NodeID: balance.GenUnlockNodeID(2)}, b)
	if err != nil {
		t.Fatalf("unlock gen2: %v", err)
	}
	if ns.Progression.MaxUnlockedGen != 2 {
		t.Fatalf("after gen2 unlock Progression.MaxUnlockedGen = %d, want 2", ns.Progression.MaxUnlockedGen)
	}
	if s.Progression.MaxUnlockedGen != 1 {
		t.Fatalf("Apply mutated input Progression: %d", s.Progression.MaxUnlockedGen)
	}
	// Contiguous Gen3.
	ns, err = Apply(ns, model.UnlockTech{NodeID: balance.GenUnlockNodeID(3)}, b)
	if err != nil {
		t.Fatalf("unlock gen3: %v", err)
	}
	if ns.Progression.MaxUnlockedGen != 3 {
		t.Fatalf("after gen3 unlock Progression.MaxUnlockedGen = %d, want 3", ns.Progression.MaxUnlockedGen)
	}
	// Non-generation tech must not change MaxUnlockedGen.
	before := ns.Progression.MaxUnlockedGen
	ns2, err := Apply(ns, model.UnlockTech{NodeID: "algo-cap-1"}, b)
	if err != nil {
		t.Fatalf("unlock algo: %v", err)
	}
	if ns2.Progression.MaxUnlockedGen != before {
		t.Fatalf("non-gen tech changed MaxUnlockedGen: %d → %d", before, ns2.Progression.MaxUnlockedGen)
	}
}

func TestStartTrainingGen6(t *testing.T) {
	b := balance.Default()
	spec, err := balance.Generation(6)
	if err != nil {
		t.Fatal(err)
	}
	s := model.GameState{}
	s.Resources.RnD = spec.TrainRnD * 2
	s.Progression.MaxUnlockedGen = 6
	before := s
	ns, err := Apply(s, model.StartTraining{Gen: 6, Alloc: validAlloc(), Price: 12}, b)
	if err != nil {
		t.Fatalf("Gen6 train: %v", err)
	}
	if !approx(ns.Resources.RnD, before.Resources.RnD-spec.TrainRnD) {
		t.Errorf("RnD = %v, want %v", ns.Resources.RnD, before.Resources.RnD-spec.TrainRnD)
	}
	if !ns.HasTraining || ns.Training.Gen != 6 {
		t.Fatalf("training job wrong: %+v", ns.Training)
	}
	if !approx(ns.Training.WorkRemaining, spec.TrainWork) {
		t.Errorf("WorkRemaining = %v, want %v", ns.Training.WorkRemaining, spec.TrainWork)
	}
	// Locked without progression/legacy unlock.
	locked := model.GameState{Resources: model.Resources{RnD: 1e15}}
	if _, err := Apply(locked, model.StartTraining{Gen: 6, Alloc: validAlloc(), Price: 12}, b); err != ErrGenLocked {
		t.Fatalf("Gen6 without unlock: err = %v, want ErrGenLocked", err)
	}
	// Invalid generation returns typed catalog error without mutation.
	bad := model.GameState{Resources: model.Resources{RnD: 1e15}}
	bad.Progression.MaxUnlockedGen = 99
	out, err := Apply(bad, model.StartTraining{Gen: 0, Alloc: validAlloc(), Price: 12}, b)
	if !errors.Is(err, balance.ErrInvalidGenerationSpec) {
		t.Fatalf("gen 0: err = %v, want ErrInvalidGenerationSpec", err)
	}
	if out.HasTraining || out.Resources.RnD != bad.Resources.RnD {
		t.Fatalf("invalid gen mutated state: %+v", out)
	}
}

func TestTrainingGen6Quality(t *testing.T) {
	b := balance.Default()
	spec, err := balance.Generation(6)
	if err != nil {
		t.Fatal(err)
	}
	s := model.GameState{HasTraining: true}
	s.Compute.RentedTraining = map[string]int{"N7": 10000}
	s.Training = model.TrainingJob{
		Gen: 6, Alloc: validAlloc(), Price: 12, WorkRemaining: 1,
	}
	ns := Tick(s, 1, nil, b)
	if len(ns.Models) != 1 {
		t.Fatalf("expected completed Gen6 draft, models=%d", len(ns.Models))
	}
	// alloc 0.4 * QualityScale 120 = 48 on capability
	want := 0.4 * spec.QualityScale
	if !approx(ns.Models[0].Quality[model.DimCapability], want) {
		t.Fatalf("capability = %v, want %v (scale %v)", ns.Models[0].Quality[model.DimCapability], want, spec.QualityScale)
	}
	if ns.Models[0].Gen != 6 {
		t.Fatalf("model gen = %d, want 6", ns.Models[0].Gen)
	}
}

func TestStartTrainingGenLocked(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Resources.RnD = 1e9
	if _, err := Apply(s, model.StartTraining{Gen: 2, Alloc: validAlloc(), Price: 12}, b); err != ErrGenLocked {
		t.Fatalf("Gen2 without unlock: err = %v, want ErrGenLocked", err)
	}
	s.UnlockedTech = []string{balance.GenUnlockNodeID(2)}
	if _, err := Apply(s, model.StartTraining{Gen: 2, Alloc: validAlloc(), Price: 12}, b); err != nil {
		t.Fatalf("Gen2 after unlock should succeed, got %v", err)
	}
}

func TestApplySetPriceSuccess(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Models: []model.Model{{Online: true, Price: 12}}}
	ns, err := Apply(s, model.SetPrice{ModelIndex: 0, Price: 20}, b)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ns.Models[0].Price != 20 {
		t.Errorf("price = %v, want 20", ns.Models[0].Price)
	}
	if s.Models[0].Price != 12 {
		t.Errorf("Apply mutated input Models (price = %v)", s.Models[0].Price)
	}
}

func TestApplySetPriceErrors(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Models: []model.Model{{Online: true, Price: 12}}}
	if _, err := Apply(s, model.SetPrice{ModelIndex: 5, Price: 20}, b); err != ErrInvalidModelIndex {
		t.Errorf("index: err = %v, want ErrInvalidModelIndex", err)
	}
	if _, err := Apply(s, model.SetPrice{ModelIndex: 0, Price: 0}, b); err != ErrInvalidPrice {
		t.Errorf("price: err = %v, want ErrInvalidPrice", err)
	}
}

func TestApplyRentInferenceCompute(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Compute.RentedInference = map[string]int{"N7": 2}
	ns, err := Apply(s, model.RentCompute{Process: "N7", Pool: model.PoolInference, Delta: 3}, b)
	if err != nil || ns.Compute.RentedInference["N7"] != 5 {
		t.Fatalf("capacity = %v err=%v, want 5", ns.Compute.RentedInference["N7"], err)
	}
	ns2, _ := Apply(s, model.RentCompute{Process: "N7", Pool: model.PoolInference, Delta: -10}, b)
	if ns2.Compute.RentedInference["N7"] != 0 {
		t.Fatalf("should floor at 0, got %v", ns2.Compute.RentedInference["N7"])
	}
	if s.Compute.RentedInference["N7"] != 2 {
		t.Fatalf("Apply mutated input")
	}
}

func TestApplyExpandDatacenter(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Resources.Cash = 1_000_000
	ns, err := Apply(s, model.ExpandDatacenter{PowerDelta: 800, SlotDelta: 20}, b)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ns.Datacenter.PowerCapacity != 800 || ns.Datacenter.SlotCapacity != 20 {
		t.Errorf("capacity wrong: %+v", ns.Datacenter)
	}
	wantCost := 800*b.PowerCostPerKW + 20*b.SlotCost
	if !approx(ns.Resources.Cash, 1_000_000-wantCost) {
		t.Errorf("cash = %v, want %v", ns.Resources.Cash, 1_000_000-wantCost)
	}
	if s.Datacenter.PowerCapacity != 0 {
		t.Errorf("Apply mutated input")
	}
}

func TestApplyExpandDatacenterInsufficientCash(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Resources.Cash = 100
	if _, err := Apply(s, model.ExpandDatacenter{PowerDelta: 800, SlotDelta: 20}, b); err != ErrInsufficientCash {
		t.Fatalf("err = %v, want ErrInsufficientCash", err)
	}
}

func dcState(cash, power, slots float64) model.GameState {
	s := model.GameState{}
	s.Resources.Cash = cash
	s.Datacenter = model.Datacenter{PowerCapacity: power, SlotCapacity: slots}
	return s
}

func TestApplyBuildServerSuccess(t *testing.T) {
	b := balance.Default()
	n7, _ := balance.ProcessByID(b.Processes, "N7")
	s := dcState(1_000_000, 800, 20)
	ns, err := Apply(s, model.BuildServer{Process: "N7", Pool: model.PoolTraining}, b)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(ns.Servers) != 1 {
		t.Fatalf("servers = %d, want 1", len(ns.Servers))
	}
	sv := ns.Servers[0]
	if sv.Pool != model.PoolTraining || sv.Compute != n7.Compute || sv.PowerKW != n7.PowerKW || sv.Slots != 1 {
		t.Errorf("server wrong: %+v", sv)
	}
	wantCapex := n7.BuyPrice + b.ChassisCost
	if !approx(ns.Resources.Cash, 1_000_000-wantCapex) {
		t.Errorf("cash = %v, want %v", ns.Resources.Cash, 1_000_000-wantCapex)
	}
	if len(s.Servers) != 0 {
		t.Errorf("Apply mutated input Servers")
	}
}

func TestApplyBuildServerErrors(t *testing.T) {
	b := balance.Default()
	// unknown process
	if _, err := Apply(dcState(1e9, 1e9, 1e9), model.BuildServer{Process: "nope", Pool: model.PoolTraining}, b); err != ErrInvalidProcess {
		t.Errorf("process: err = %v, want ErrInvalidProcess", err)
	}
	// insufficient cash
	if _, err := Apply(dcState(100, 1e9, 1e9), model.BuildServer{Process: "N7", Pool: model.PoolTraining}, b); err != ErrInsufficientCash {
		t.Errorf("cash: err = %v, want ErrInsufficientCash", err)
	}
	// insufficient power (N7 server draws 2kW; capacity 1)
	if _, err := Apply(dcState(1e9, 1, 1e9), model.BuildServer{Process: "N7", Pool: model.PoolTraining}, b); err != ErrInsufficientPower {
		t.Errorf("power: err = %v, want ErrInsufficientPower", err)
	}
	// insufficient space (slots 0)
	if _, err := Apply(dcState(1e9, 1e9, 0), model.BuildServer{Process: "N7", Pool: model.PoolTraining}, b); err != ErrInsufficientSpace {
		t.Errorf("space: err = %v, want ErrInsufficientSpace", err)
	}
	// locked process (N5 needs process-N5 tech)
	if _, err := Apply(dcState(1e9, 1e9, 1e9), model.BuildServer{Process: "N5", Pool: model.PoolTraining}, b); err != ErrProcessLocked {
		t.Errorf("locked: err = %v, want ErrProcessLocked", err)
	}
}

func TestApplyBuildServerIntoInferencePool(t *testing.T) {
	b := balance.Default()
	s := dcState(1_000_000, 800, 20)
	ns, err := Apply(s, model.BuildServer{Process: "N7", Pool: model.PoolInference}, b)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(ns.Servers) != 1 || ns.Servers[0].Pool != model.PoolInference {
		t.Fatalf("expected 1 server in inference pool, got %+v", ns.Servers)
	}
}

func TestApplyHireStaff(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Resources.Cash = 100000
	// hire 2 T2 researchers
	ns, err := Apply(s, model.HireStaff{Role: model.RoleResearcher, Tier: model.Tier2, Count: 2}, b)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ns.Research.Researchers[model.Tier2] != 2 {
		t.Errorf("researchers = %d, want 2", ns.Research.Researchers[model.Tier2])
	}
	if !approx(ns.Resources.Cash, 100000-2*b.ResearcherHireCost[model.Tier2]) {
		t.Errorf("cash wrong: %v", ns.Resources.Cash)
	}
	// hire 3 engineers
	ns2, _ := Apply(ns, model.HireStaff{Role: model.RoleEngineer, Count: 3}, b)
	if ns2.Engineers != 3 {
		t.Errorf("engineers = %d, want 3", ns2.Engineers)
	}
	// purity
	if s.Research.Researchers[model.Tier2] != 0 {
		t.Errorf("Apply mutated input")
	}
}

func TestApplyHireStaffErrors(t *testing.T) {
	b := balance.Default()
	rich := model.GameState{}
	rich.Resources.Cash = 1e9
	if _, err := Apply(rich, model.HireStaff{Role: model.RoleResearcher, Tier: model.Tier2, Count: 0}, b); err != ErrInvalidCount {
		t.Errorf("count: err = %v, want ErrInvalidCount", err)
	}
	if _, err := Apply(rich, model.HireStaff{Role: model.RoleResearcher, Tier: model.TierNone, Count: 1}, b); err != ErrInvalidTier {
		t.Errorf("tier: err = %v, want ErrInvalidTier", err)
	}
	poor := model.GameState{}
	poor.Resources.Cash = 10
	if _, err := Apply(poor, model.HireStaff{Role: model.RoleEngineer, Count: 1}, b); err != ErrInsufficientCash {
		t.Errorf("cash: err = %v, want ErrInsufficientCash", err)
	}
}

func TestApplyFireStaffFloorsAtZero(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Engineers: 2}
	ns, err := Apply(s, model.FireStaff{Role: model.RoleEngineer, Count: 5}, b)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ns.Engineers != 0 {
		t.Fatalf("engineers = %d, want 0", ns.Engineers)
	}
}

func TestApplyUnlockTech(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Resources.RnD = 100000
	ns, err := Apply(s, model.UnlockTech{NodeID: "algo-cap-1"}, b) // cost 15000
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !approx(ns.Resources.RnD, 85000) {
		t.Errorf("RnD = %v, want 85000", ns.Resources.RnD)
	}
	if len(ns.UnlockedTech) != 1 || ns.UnlockedTech[0] != "algo-cap-1" {
		t.Errorf("not unlocked: %+v", ns.UnlockedTech)
	}
	if len(s.UnlockedTech) != 0 {
		t.Errorf("Apply mutated input")
	}
}

func TestApplyUnlockTechErrors(t *testing.T) {
	b := balance.Default()
	rich := model.GameState{}
	rich.Resources.RnD = 1e9
	if _, err := Apply(rich, model.UnlockTech{NodeID: "nope"}, b); err != ErrInvalidTech {
		t.Errorf("invalid: err = %v, want ErrInvalidTech", err)
	}
	if _, err := Apply(rich, model.UnlockTech{NodeID: "infra-density-1"}, b); err != ErrPrereqNotMet {
		t.Errorf("prereq: err = %v, want ErrPrereqNotMet", err)
	}
	already := model.GameState{UnlockedTech: []string{"algo-cap-1"}}
	already.Resources.RnD = 1e9
	if _, err := Apply(already, model.UnlockTech{NodeID: "algo-cap-1"}, b); err != ErrAlreadyUnlocked {
		t.Errorf("already: err = %v, want ErrAlreadyUnlocked", err)
	}
	poor := model.GameState{}
	poor.Resources.RnD = 100
	if _, err := Apply(poor, model.UnlockTech{NodeID: "algo-cap-1"}, b); err != ErrInsufficientRnD {
		t.Errorf("rnd: err = %v, want ErrInsufficientRnD", err)
	}
}

func TestApplyBuyPrestigeNode(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Prestige.Patents = 5
	ns, err := Apply(s, model.BuyPrestigeNode{NodeID: "start-cash-1"}, b) // cost 1
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ns.Prestige.Patents != 4 {
		t.Errorf("patents = %v, want 4", ns.Prestige.Patents)
	}
	if len(ns.Prestige.UnlockedPrestige) != 1 || ns.Prestige.UnlockedPrestige[0] != "start-cash-1" {
		t.Errorf("not unlocked: %+v", ns.Prestige.UnlockedPrestige)
	}
	if len(s.Prestige.UnlockedPrestige) != 0 {
		t.Errorf("Apply mutated input")
	}
}

func TestApplyBuyPrestigeErrors(t *testing.T) {
	b := balance.Default()
	rich := model.GameState{}
	rich.Prestige.Patents = 100
	if _, err := Apply(rich, model.BuyPrestigeNode{NodeID: "nope"}, b); err != ErrInvalidPrestigeNode {
		t.Errorf("invalid: err = %v, want ErrInvalidPrestigeNode", err)
	}
	if _, err := Apply(rich, model.BuyPrestigeNode{NodeID: "start-cash-1"}, b); err != nil {
		t.Errorf("rich buy should succeed: %v", err)
	}
	already := model.GameState{}
	already.Prestige.Patents = 100
	already.Prestige.UnlockedPrestige = []string{"start-cash-1"}
	if _, err := Apply(already, model.BuyPrestigeNode{NodeID: "start-cash-1"}, b); err != ErrAlreadyUnlocked {
		t.Errorf("already: err = %v, want ErrAlreadyUnlocked", err)
	}
	poor := model.GameState{}
	poor.Prestige.Patents = 0
	if _, err := Apply(poor, model.BuyPrestigeNode{NodeID: "start-cash-1"}, b); err != ErrInsufficientPatents {
		t.Errorf("patents: err = %v, want ErrInsufficientPatents", err)
	}
}

func TestApplyPrestigeReset(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.PeakValuation = 1e9 // patents = floor(sqrt(1e9/1e8)) = 3
	s.Resources.Cash = 5e6
	s.Resources.RnD = 1e6
	s.Models = []model.Model{{Online: true}}
	s.Engineers = 5
	s.Prestige.Patents = 1
	ns, err := Apply(s, model.PrestigeReset{}, b)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ns.Prestige.Patents != 4 { // 1 existing + 3 gained
		t.Errorf("patents = %v, want 4", ns.Prestige.Patents)
	}
	if len(ns.Models) != 0 || ns.Engineers != 0 || ns.PeakValuation != 0 {
		t.Errorf("run state not reset: %+v", ns)
	}
	if !approx(ns.Resources.Cash, b.StartingCash) {
		t.Errorf("cash not reset to starting: %v", ns.Resources.Cash)
	}
	if len(ns.Competitors) != 7 {
		t.Errorf("competitors not re-seeded")
	}
}

func TestApplyPrestigeLocked(t *testing.T) {
	b := balance.Default()
	s := model.GameState{} // peak 0 < 1e9
	if _, err := Apply(s, model.PrestigeReset{}, b); err != ErrPrestigeLocked {
		t.Fatalf("err = %v, want ErrPrestigeLocked", err)
	}
}

func TestApplySignStar(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Resources.Cash = 1_000_000
	ns, err := Apply(s, model.SignStar{StarID: "aria-chen"}, b) // signing 600000
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !approx(ns.Resources.Cash, 400000) {
		t.Errorf("cash = %v, want 400000", ns.Resources.Cash)
	}
	if len(ns.HiredStars) != 1 || ns.HiredStars[0] != "aria-chen" {
		t.Errorf("not hired: %+v", ns.HiredStars)
	}
	if len(s.HiredStars) != 0 {
		t.Errorf("Apply mutated input")
	}
}

func TestApplySignStarErrors(t *testing.T) {
	b := balance.Default()
	rich := model.GameState{}
	rich.Resources.Cash = 1e9
	if _, err := Apply(rich, model.SignStar{StarID: "nope"}, b); err != ErrInvalidStar {
		t.Errorf("invalid: err = %v, want ErrInvalidStar", err)
	}
	already := model.GameState{HiredStars: []string{"aria-chen"}}
	already.Resources.Cash = 1e9
	if _, err := Apply(already, model.SignStar{StarID: "aria-chen"}, b); err != ErrAlreadyHired {
		t.Errorf("already: err = %v, want ErrAlreadyHired", err)
	}
	poor := model.GameState{}
	poor.Resources.Cash = 100
	if _, err := Apply(poor, model.SignStar{StarID: "aria-chen"}, b); err != ErrInsufficientCash {
		t.Errorf("cash: err = %v, want ErrInsufficientCash", err)
	}
}

func TestApplyPublishModel(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		Models: []model.Model{{
			Gen: 1, Segment: model.SegConsumer, Price: 12,
			Online: false, Users: 0,
			Quality: [model.NumQualityDims]float64{25, 0, 0, 0},
		}},
	}
	ns, err := Apply(s, model.PublishModel{ModelIndex: 0, Name: "  Nova-1  ", Price: 9}, b)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if !ns.Models[0].Online || ns.Models[0].Name != "Nova-1" || ns.Models[0].Price != 9 {
		t.Fatalf("published model wrong: %+v", ns.Models[0])
	}
	// purity
	if s.Models[0].Online || s.Models[0].Name != "" {
		t.Fatal("Apply mutated input")
	}
}

func TestApplyPublishModelRejects(t *testing.T) {
	b := balance.Default()
	draft := model.Model{Online: false, Users: 0}
	live := model.Model{Online: true, Users: 0, Name: "x"}
	used := model.Model{Online: false, Users: 10} // not a draft

	s := model.GameState{Models: []model.Model{draft, live, used}}
	cases := []struct {
		cmd model.PublishModel
		err error
	}{
		{model.PublishModel{ModelIndex: -1, Name: "a", Price: 1}, ErrInvalidModelIndex},
		{model.PublishModel{ModelIndex: 99, Name: "a", Price: 1}, ErrInvalidModelIndex},
		{model.PublishModel{ModelIndex: 1, Name: "a", Price: 1}, ErrNotDraft},
		{model.PublishModel{ModelIndex: 2, Name: "a", Price: 1}, ErrNotDraft},
		{model.PublishModel{ModelIndex: 0, Name: "   ", Price: 1}, ErrInvalidName},
		{model.PublishModel{ModelIndex: 0, Name: strings.Repeat("字", 25), Price: 1}, ErrInvalidName},
		{model.PublishModel{ModelIndex: 0, Name: "ok", Price: 0}, ErrInvalidPrice},
		{model.PublishModel{ModelIndex: 0, Name: "ok", Price: -1}, ErrInvalidPrice},
	}
	for _, tc := range cases {
		if _, err := Apply(s, tc.cmd, b); !errors.Is(err, tc.err) {
			t.Errorf("cmd=%+v err=%v want %v", tc.cmd, err, tc.err)
		}
	}
}

func TestApplyResolveEvent(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	spec, _ := balance.EventByID(b.Events, balance.EvChipShortage)
	s = fireEvent(s, spec, b)
	ns, err := Apply(s, model.ResolveEvent{PendingIndex: 0, Choice: 1}, b)
	if err != nil {
		t.Fatalf("Apply(ResolveEvent): %v", err)
	}
	if len(ns.Events.Pending) != 0 || len(ns.Events.Log) != 1 {
		t.Fatalf("resolution not applied: %+v", ns.Events)
	}
	if ns.Events.Log[0].Auto {
		t.Fatal("player resolution must not be marked auto")
	}
	if _, err := Apply(s, model.ResolveEvent{PendingIndex: 9, Choice: 1}, b); err != ErrInvalidEventIndex {
		t.Fatalf("bad index err = %v, want ErrInvalidEventIndex", err)
	}
}
