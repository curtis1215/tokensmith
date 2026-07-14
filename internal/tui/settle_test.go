package tui

import (
	"strings"
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

func TestSettleGrantsOfflineRnDAndAdvances(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Research.EfficiencyMult = 1
	before := s.Resources.RnD
	ns, sum := Settle(s, b, 2*3600, 100000, 50000) // 2h offline + tokens
	if sum.RnDGained <= 0 || ns.Resources.RnD <= before {
		t.Fatalf("offline settlement granted no R&D: %+v", sum)
	}
	if ns.GameTime < 2*3600-1 {
		t.Fatalf("world did not advance: GameTime=%v", ns.GameTime)
	}
	if sum.TokensIn != 100000 || sum.TokensOut != 50000 {
		t.Fatalf("summary tokens wrong: %+v", sum)
	}
}

func TestSettleCompletesTraining(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Compute.RentedTraining = map[string]int{"N7": 1}
	s.HasTraining = true
	s.Training = model.TrainingJob{Gen: 1, Alloc: [4]float64{0.4, 0.2, 0.2, 0.2}, Price: 12, WorkRemaining: 1800}
	ns, sum := Settle(s, b, 4*3600, 0, 0) // 4h × 1 GPU = 14400 GPU·s ≫ 1800
	if !sum.TrainingCompleted || ns.HasTraining {
		t.Fatalf("training should complete offline: %+v", sum)
	}
}

func TestSettleClampsElapsed(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	_, sum := Settle(s, b, -100, 0, 0)
	if sum.SecondsSettled != 0 {
		t.Fatalf("negative elapsed should clamp to 0, got %v", sum.SecondsSettled)
	}
	_, sum2 := Settle(s, b, 999*86400, 0, 0)
	if sum2.SecondsSettled != settleMaxSec {
		t.Fatalf("huge elapsed should clamp to max, got %v", sum2.SecondsSettled)
	}
}

func TestSettleCountsEvents(t *testing.T) {
	b := balance.Default()
	b.EventHitChance = 1.0
	b.EventCheckSec = 3600 // one roll per settle chunk
	var s model.GameState
	s.Resources.Cash = 1e6
	s.Events.RandState = 42
	s.Events.NextCheckAt = 1 // pre-scheduled so rolls happen immediately
	ns, sum := Settle(s, b, 6*3600, 0, 0)
	if sum.EventsFired == 0 {
		t.Fatalf("expected events during 6h settle, got %+v", sum)
	}
	if ns.Events.FiredCount != sum.EventsFired {
		t.Fatalf("summary %d != state counter %d", sum.EventsFired, ns.Events.FiredCount)
	}
}

func TestNewAtSeedsRandState(t *testing.T) {
	m := testModel(t)
	if m.state.Events.RandState == 0 {
		t.Fatal("a fresh game must get a nonzero RNG seed")
	}
	if m.state.Campaign.RandState == 0 {
		t.Fatal("a fresh game must get a nonzero campaign RNG seed")
	}
}

func TestOfflineBannerAutoResolvedOnly(t *testing.T) {
	m := testModel(t)
	m.offlineSummary = &Summary{EventsAutoResolved: 2, SecondsSettled: 3600}
	out := renderOfflineReport(m)
	if !strings.Contains(out, "自動決議") {
		t.Fatalf("expected auto-resolve line when EventsFired==0, got %q", out)
	}
}

func TestSettleCapsIndustryFrontier(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Progression.IndustryTime = 0 // next gen boundary = 1000 days
	oneGen := sim.SecondsUntilNextTimeGeneration(s, b)
	cap8h := 8 * 3600 * balance.RealSecCompression
	// Economy may run a full multi-day window; industry must not exceed caps.
	const elapsed = 3 * 86400.0 // 3 real days offline
	ns, sum := Settle(s, b, elapsed, 0, 0)
	if sum.SecondsSettled != elapsed {
		t.Fatalf("economy settled %v, want %v", sum.SecondsSettled, elapsed)
	}
	if ns.GameTime < elapsed-1 {
		t.Fatalf("economy GameTime = %v, want ~%v", ns.GameTime, elapsed)
	}
	// Industry allowance = min(elapsed*compression, 8h*compression, oneGen)
	wantIndustry := elapsed * balance.RealSecCompression
	if cap8h < wantIndustry {
		wantIndustry = cap8h
	}
	if oneGen < wantIndustry {
		wantIndustry = oneGen
	}
	if !approxIndustry(ns.Progression.IndustryTime, wantIndustry) {
		t.Fatalf("IndustryTime = %v, want capped %v (8h=%v oneGen=%v)",
			ns.Progression.IndustryTime, wantIndustry, cap8h, oneGen)
	}
	// Must be strictly less than full uncompressed multi-day industry.
	if ns.Progression.IndustryTime >= elapsed*balance.RealSecCompression && elapsed*balance.RealSecCompression > wantIndustry {
		t.Fatal("industry was not capped")
	}
}

func TestSettleDropsIndustryBacklog(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Progression.IndustryTime = 0
	// First settle consumes the one-generation + 8h caps as applicable.
	ns1, _ := Settle(s, b, 30*86400, 0, 0) // 30 real days
	ind1 := ns1.Progression.IndustryTime
	// Second settle from same wall-clock-style huge window must not replay
	// the dropped backlog — only a fresh allowance from the new IndustryTime.
	oneGen2 := sim.SecondsUntilNextTimeGeneration(ns1, b)
	cap8h := 8 * 3600 * balance.RealSecCompression
	ns2, _ := Settle(ns1, b, 30*86400, 0, 0)
	delta := ns2.Progression.IndustryTime - ind1
	wantMax := 30 * 86400 * balance.RealSecCompression
	if cap8h < wantMax {
		wantMax = cap8h
	}
	if oneGen2 < wantMax {
		wantMax = oneGen2
	}
	if delta > wantMax+1 {
		t.Fatalf("second settle industry delta %v exceeds fresh cap %v (backlog replayed?)", delta, wantMax)
	}
	// Total industry is not "sum of full 30d compressions".
	full := 2 * 30 * 86400 * balance.RealSecCompression
	if ns2.Progression.IndustryTime >= full*0.5 {
		t.Fatalf("industry %v looks like backlog was banked/replayed (full would be %v)",
			ns2.Progression.IndustryTime, full)
	}
}

func approxIndustry(a, b float64) bool {
	if a == b {
		return true
	}
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= 1e-3*b || d < 1
}

// TestSettleIndustryCapIsResidualToNextBaseline documents that the "one
// generation" offline cap is residual-to-next-TimeBaselineDay, not the full
// current generation interval width (design §8.2 clarified).
func TestSettleIndustryCapIsResidualToNextBaseline(t *testing.T) {
	b := balance.Default()
	// Place IndustryTime just before Gen2 baseline so residual ≪ full interval.
	g2, err := balance.Generation(2)
	if err != nil {
		t.Fatal(err)
	}
	// 99% of the way from Gen1 baseline to Gen2 baseline.
	g1, _ := balance.Generation(1)
	spanDays := g2.TimeBaselineDay - g1.TimeBaselineDay
	if spanDays <= 0 {
		t.Fatalf("unexpected baselines g1=%v g2=%v", g1.TimeBaselineDay, g2.TimeBaselineDay)
	}
	s := model.GameState{}
	s.Progression.IndustryTime = (g1.TimeBaselineDay + 0.99*spanDays) * 86400
	residual := sim.SecondsUntilNextTimeGeneration(s, b)
	if residual <= 0 || residual >= spanDays*86400*0.5 {
		t.Fatalf("residual = %v days-equiv, want small fraction of span %v", residual/86400, spanDays)
	}
	// Huge offline window — industry still stops at residual (or 8h if smaller).
	const elapsed = 30 * 86400.0
	ns, _ := Settle(s, b, elapsed, 0, 0)
	delta := ns.Progression.IndustryTime - s.Progression.IndustryTime
	cap8h := 8 * 3600 * balance.RealSecCompression
	want := residual
	if cap8h < want {
		want = cap8h
	}
	if !approxIndustry(delta, want) {
		t.Fatalf("industry delta = %v, want residual/8h cap %v (residual=%v 8h=%v fullSpan=%v)",
			delta, want, residual, cap8h, spanDays*86400)
	}
	// Must not have advanced a full generation interval.
	if delta >= spanDays*86400*0.5 {
		t.Fatalf("industry advanced full-ish interval %v; cap should be residual", delta)
	}
}

func TestSettleIndustryStopsAtPlayerCap(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Progression.MaxUnlockedGen = 5
	cap := sim.IndustryTimeCapSec(s, b)
	s.Progression.IndustryTime = cap
	const elapsed = 86400.0 // 1 real day
	ns, _ := Settle(s, b, elapsed, 0, 0)
	if !approxIndustry(ns.Progression.IndustryTime, cap) {
		t.Fatalf("IndustryTime = %v, want still at cap %v", ns.Progression.IndustryTime, cap)
	}
	// Economy still advances.
	if ns.GameTime < elapsed-1 {
		t.Fatalf("GameTime = %v, want ~%v", ns.GameTime, elapsed)
	}
}
