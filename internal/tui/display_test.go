package tui

import (
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tokensmith/internal/balance"
	"tokensmith/internal/ledger"
	"tokensmith/internal/metrics"
	"tokensmith/internal/model"
	"tokensmith/internal/sim"
	"tokensmith/internal/store"
)

func TestLerpApproaches(t *testing.T) {
	x := 0.0
	for i := 0; i < 30; i++ {
		x = lerp(x, 100, 0.3)
	}
	if x < 99 {
		t.Fatalf("x=%v want ~100", x)
	}
}

func TestDisplaySnap(t *testing.T) {
	var d displayState
	d.snap(displayState{Cash: 50})
	if d.Cash != 50 {
		t.Fatal(d.Cash)
	}
}

func TestDisplayApproachTowardTruth(t *testing.T) {
	var d displayState
	truth := displayState{Cash: 1000, RnD: 500, Valuation: 2e6, TotalUsers: 100, TrainUtil: 1, InfUtil: 0.5}
	for i := 0; i < 40; i++ {
		d.approach(truth, 0.3)
	}
	if math.Abs(d.Cash-truth.Cash) > 0.02 {
		t.Fatalf("Cash=%v want %v", d.Cash, truth.Cash)
	}
	if math.Abs(d.InfUtil-truth.InfUtil) > 1e-3 {
		t.Fatalf("InfUtil=%v want %v", d.InfUtil, truth.InfUtil)
	}
}

func TestDisplaySnapAfterRestartReady(t *testing.T) {
	m := testModel(t)
	m.disp.Cash = 99999
	m.dispReady = true
	m.snapDisplay()
	if m.disp.Cash != m.state.Resources.Cash {
		t.Fatalf("snap Cash=%v want %v", m.disp.Cash, m.state.Resources.Cash)
	}
	if !m.dispReady {
		t.Fatal("dispReady should be true after snap")
	}
}

func TestAdvanceDisplayFirstTickSnaps(t *testing.T) {
	m := testModel(t)
	m.state.Resources.Cash = 1234
	m.dispReady = false
	m.advanceDisplay()
	if m.disp.Cash != 1234 {
		t.Fatalf("first advance should snap: got %v", m.disp.Cash)
	}
	if !m.dispReady {
		t.Fatal("dispReady should become true")
	}
}

func TestPulseTokenOnTokens(t *testing.T) {
	m := testModel(t)
	m.dispReady = true
	m.disp.snap(truthDisplay(m))
	m.tokensThisTick = true
	m.advanceDisplay()
	if m.disp.PulseToken != tokenPulseTicks {
		t.Fatalf("PulseToken=%d want %d", m.disp.PulseToken, tokenPulseTicks)
	}
	m.tokensThisTick = false
	m.advanceDisplay()
	if m.disp.PulseToken != tokenPulseTicks-1 {
		t.Fatalf("PulseToken should decay to %d, got %d", tokenPulseTicks-1, m.disp.PulseToken)
	}
}

func TestRenderResourceBarShowsPerSourceRnD(t *testing.T) {
	m := testModel(t)
	m.lastTokenRnD = map[string]float64{"claude-code": 842, "codex": 15}
	m.disp.PulseToken = 5
	bar := renderResourceBar(m)
	if !strings.Contains(bar, "Claude Code +842 R&D") {
		t.Fatalf("expected Claude Code R&D segment, got:\n%s", bar)
	}
	if !strings.Contains(bar, "Codex +15 R&D") {
		t.Fatalf("expected Codex R&D segment, got:\n%s", bar)
	}
}

func TestRenderResourceBarShowsGrokEstimateAndOpenCode(t *testing.T) {
	m := testModel(t)
	m.lastTokenRnD = map[string]float64{"opencode": 955, "grok": 730}
	m.disp.PulseToken = 5
	bar := renderResourceBar(m)
	if !strings.Contains(bar, "Grok（估算） +730 R&D") {
		t.Fatalf("expected estimated Grok segment, got:\n%s", bar)
	}
	if !strings.Contains(bar, "OpenCode +955 R&D") {
		t.Fatalf("expected OpenCode segment, got:\n%s", bar)
	}
	if strings.Index(bar, "Grok（估算）") > strings.Index(bar, "OpenCode") {
		t.Fatalf("known sources should use stable order, got:\n%s", bar)
	}
}

// TestPerSourceRnDDisplayIncludesPrestigeMult proves the status-bar per-source
// R&D includes pe.RnDMult (e.g. rnd-mult-1 → 1.1x), matching what sim.Tick
// actually books — without this, prestiging permanently under-reports the bar.
// Also multiplies by HQ OfficeTokenRnDMult (default Office.Level 0/1 → 1.0)
// and TokenSkillRnDMult (1 with empty roster).
func TestPerSourceRnDDisplayIncludesPrestigeMult(t *testing.T) {
	dir := t.TempDir()
	lp := filepath.Join(dir, "ledger.json")
	mp := filepath.Join(dir, "meta.json")
	// raw = (1000*1 + 500*2) / 10 = 200
	ledger.Save(lp, ledger.Ledger{
		Sources:   map[string]model.SourceTotals{"claude-code": {In: 1000, Out: 500}},
		UpdatedAt: 9_000_000_000,
	})
	store.SaveMeta(mp, store.Meta{LastRealUnix: 9_000_000_000})

	m := newAtPaths(filepath.Join(dir, "s.json"), lp, mp)
	m.daemonMode = true
	m.state.Prestige.UnlockedPrestige = []string{"rnd-mult-1"}

	nm, _ := m.Update(tickMsg(time.Unix(0, 0)))
	got := nm.(Model)

	raw := sim.TokenRawRnD([]model.TokenEvent{{
		Source: "claude-code", InputTokens: 1000, OutputTokens: 500,
	}}, got.cfg)
	pe := sim.PrestigeEffects(got.state.Prestige.UnlockedPrestige, got.cfg)
	hq := balance.OfficeTokenRnDMultAt(got.state.Office.Level, got.cfg)
	sk := sim.TokenSkillRnDMult(got.state, got.cfg)
	// First active day sets streakDays=1 → StreakMult 1.06; RnDMult 1.1 from rnd-mult-1; hq L1=1.0; sk=1.
	want := raw * got.currentStreakMult() * pe.RnDMult * sk * hq // 200 * 1.06 * 1.1 * 1 * 1 = 233.2
	if math.Abs(got.lastTokenRnD["claude-code"]-want) > 1e-9 {
		t.Fatalf("lastTokenRnD[claude-code]=%v want %v (raw*Streak*RnDMult*skill*hq)",
			got.lastTokenRnD["claude-code"], want)
	}
	if pe.RnDMult != 1.1 {
		t.Fatalf("RnDMult=%v want 1.1 from rnd-mult-1", pe.RnDMult)
	}
	// Without prestige the same tick would show raw*streak*skill*hq only (~212); assert the 1.1x gap.
	withoutPrestige := raw * got.currentStreakMult() * sk * hq
	if math.Abs(got.lastTokenRnD["claude-code"]-withoutPrestige*1.1) > 1e-9 {
		t.Fatalf("displayed R&D should be 1.1× the non-prestige amount: got %v vs base %v",
			got.lastTokenRnD["claude-code"], withoutPrestige)
	}

	got.disp.PulseToken = 5
	bar := renderResourceBar(got)
	wantSeg := "Claude Code +" + human(want) + " R&D"
	if !strings.Contains(bar, wantSeg) {
		t.Fatalf("expected prestige-multiplied segment %q in status bar, got:\n%s", wantSeg, bar)
	}
}

func TestPerSourceRnDDisplayIncludesOfficeMult(t *testing.T) {
	dir := t.TempDir()
	lp := filepath.Join(dir, "ledger.json")
	mp := filepath.Join(dir, "meta.json")
	ledger.Save(lp, ledger.Ledger{
		Sources:   map[string]model.SourceTotals{"claude-code": {In: 1000, Out: 500}},
		UpdatedAt: 9_000_000_000,
	})
	store.SaveMeta(mp, store.Meta{LastRealUnix: 9_000_000_000})

	m := newAtPaths(filepath.Join(dir, "s.json"), lp, mp)
	m.daemonMode = true
	m.state.Office.Level = 6 // mult 3.5

	nm, _ := m.Update(tickMsg(time.Unix(0, 0)))
	got := nm.(Model)

	raw := sim.TokenRawRnD([]model.TokenEvent{{
		Source: "claude-code", InputTokens: 1000, OutputTokens: 500,
	}}, got.cfg)
	pe := sim.PrestigeEffects(got.state.Prestige.UnlockedPrestige, got.cfg)
	hq := balance.OfficeTokenRnDMultAt(got.state.Office.Level, got.cfg)
	sk := sim.TokenSkillRnDMult(got.state, got.cfg)
	want := raw * got.currentStreakMult() * pe.RnDMult * sk * hq
	if math.Abs(got.lastTokenRnD["claude-code"]-want) > 1e-9 {
		t.Fatalf("lastTokenRnD=%v want %v (raw*streak*prestige*skill*hq)",
			got.lastTokenRnD["claude-code"], want)
	}

	got.disp.PulseToken = 5
	bar := renderResourceBar(got)
	if !strings.Contains(bar, "總部 ×3.50") {
		t.Fatalf("expected HQ mult badge in bar, got:\n%s", bar)
	}
	wantSeg := "Claude Code +" + human(want) + " R&D"
	if !strings.Contains(bar, wantSeg) {
		t.Fatalf("expected HQ-scaled segment %q in bar, got:\n%s", wantSeg, bar)
	}
}

// TestPerSourceRnDDisplayIncludesSkillTokenMult proves lastTokenRnD (and thus
// dashboard inflow) multiplies sim.TokenSkillRnDMult, matching sim.Tick.
func TestPerSourceRnDDisplayIncludesSkillTokenMult(t *testing.T) {
	dir := t.TempDir()
	lp := filepath.Join(dir, "ledger.json")
	mp := filepath.Join(dir, "meta.json")
	ledger.Save(lp, ledger.Ledger{
		Sources:   map[string]model.SourceTotals{"claude-code": {In: 1000, Out: 500}},
		UpdatedAt: 9_000_000_000,
	})
	store.SaveMeta(mp, store.Meta{LastRealUnix: 9_000_000_000})

	m := newAtPaths(filepath.Join(dir, "s.json"), lp, mp)
	m.daemonMode = true
	// m-pipeline: TokenRnDMult 1.02
	m.state.Employees = []model.Employee{{
		PrimaryRole: model.RoleResearcher,
		SkillIDs:    []string{"m-pipeline"},
	}}

	nm, _ := m.Update(tickMsg(time.Unix(0, 0)))
	got := nm.(Model)

	raw := sim.TokenRawRnD([]model.TokenEvent{{
		Source: "claude-code", InputTokens: 1000, OutputTokens: 500,
	}}, got.cfg)
	pe := sim.PrestigeEffects(got.state.Prestige.UnlockedPrestige, got.cfg)
	hq := balance.OfficeTokenRnDMultAt(got.state.Office.Level, got.cfg)
	sk := sim.TokenSkillRnDMult(got.state, got.cfg)
	if math.Abs(sk-1.02) > 1e-9 {
		t.Fatalf("TokenSkillRnDMult=%v want 1.02", sk)
	}
	want := raw * got.currentStreakMult() * pe.RnDMult * sk * hq
	if math.Abs(got.lastTokenRnD["claude-code"]-want) > 1e-9 {
		t.Fatalf("lastTokenRnD=%v want %v (with skill 1.02)", got.lastTokenRnD["claude-code"], want)
	}
	withoutSkill := raw * got.currentStreakMult() * pe.RnDMult * hq
	if math.Abs(got.lastTokenRnD["claude-code"]-withoutSkill*1.02) > 1e-9 {
		t.Fatalf("skill should scale 1.02×: got %v base %v", got.lastTokenRnD["claude-code"], withoutSkill)
	}

	// Dashboard inflow uses the same map (day key from the tick wall clock).
	day := got.metricsDay
	if day == "" {
		day = metrics.DayKey(time.Unix(0, 0))
	}
	if math.Abs(got.metricsDoc.Days[day].RnDInflow["claude-code"]-want) > 1e-9 {
		t.Fatalf("metrics inflow=%v want %v (day=%s)",
			got.metricsDoc.Days[day].RnDInflow["claude-code"], want, day)
	}
}

func TestRenderResourceBarShowsStreakBadge(t *testing.T) {
	m := testModel(t)
	m.lastTokenRnD = map[string]float64{"claude-code": 100}
	m.disp.PulseToken = 5
	m.streakDays = 3
	bar := renderResourceBar(m)
	if !strings.Contains(bar, "🔥3天") || !strings.Contains(bar, "×1.18") {
		t.Fatalf("expected streak badge, got:\n%s", bar)
	}
}

func TestRenderResourceBarHidesTokensAfterPulseEnds(t *testing.T) {
	m := testModel(t)
	m.lastTokenRnD = map[string]float64{"claude-code": 100}
	m.disp.PulseToken = 0 // pulse has fully decayed
	bar := renderResourceBar(m)
	if strings.Contains(bar, "Claude Code") {
		t.Fatalf("token segment should be hidden once the pulse ends:\n%s", bar)
	}
}

func TestDisplayApproachesUsersAndUtils(t *testing.T) {
	m := testModel(t)
	m.state.Models = []model.Model{{Online: true, Users: 1000, Price: 12, Segment: 0}}
	m.state.HasTraining = true
	m.state.Compute.InferenceLoad = 50
	m.state.Compute.RentedInference = map[string]int{"N7": 10}
	truth := truthDisplay(m)
	if truth.TotalUsers < 1000 {
		t.Fatalf("truth TotalUsers=%v", truth.TotalUsers)
	}
	var d displayState
	for i := 0; i < 40; i++ {
		d.approach(truth, 0.3)
	}
	if d.TotalUsers < 990 {
		t.Fatalf("TotalUsers approached=%v want ~%v", d.TotalUsers, truth.TotalUsers)
	}
	if d.TrainUtil < 0.99 {
		t.Fatalf("TrainUtil=%v want ~1", d.TrainUtil)
	}
}

func TestAdvanceDisplaySamplesSparks(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	for i := 0; i < 9; i++ { // 9 ticks → 至少 2 個樣本（tick 4 與 8）
		m.advanceDisplay()
	}
	if m.sparkValuation.n < 2 {
		t.Fatalf("valuation spark samples = %d, want >= 2", m.sparkValuation.n)
	}
	if m.sparkUsers.n < 2 || m.sparkRnD.n < 2 {
		t.Fatal("users/rnd sparks not sampled")
	}
}

func TestCompanyCardShowsTrend(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	for i := 0; i < 9; i++ {
		m.advanceDisplay()
	}
	out := companyCard(m, 50)
	if !strings.Contains(out, "趨勢") {
		t.Fatalf("company card missing trend line: %q", out)
	}
}

func TestAdvanceDisplayTracksCashRate(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	m.snapDisplay()
	m.state.Resources.Cash += 1000 // 下一 tick 現金上升
	for i := 0; i < 20; i++ {
		m.advanceDisplay()
	}
	if m.cashRate <= 0 {
		t.Fatalf("cashRate = %f, want > 0 after cash increase", m.cashRate)
	}
}
