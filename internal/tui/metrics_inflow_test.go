package tui

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"tokensmith/internal/ledger"
	"tokensmith/internal/metrics"
	"tokensmith/internal/model"
	"tokensmith/internal/sim"
	"tokensmith/internal/store"
)

func TestTokenInflowMatchesLastTokenRnD(t *testing.T) {
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

	now := time.Now()
	nm, _ := m.Update(tickMsg(now))
	got := nm.(Model)

	if len(got.lastTokenRnD) == 0 {
		t.Fatal("expected lastTokenRnD from ledger delta")
	}
	day := metrics.DayKey(now)
	p := got.metricsDoc.Days[day]
	for src, amt := range got.lastTokenRnD {
		if math.Abs(p.RnDInflow[src]-amt) > 1e-9 {
			t.Fatalf("inflow[%s]=%v want lastTokenRnD=%v; day=%+v",
				src, p.RnDInflow[src], amt, p)
		}
	}
}

func TestStaffInflowPositiveWhenResearchers(t *testing.T) {
	m := testModel(t)
	m.state.Employees = []model.Employee{{
		PrimaryRole: model.RoleResearcher,
		Stats:       [model.NumRoles]int{50, 0, 0, 0},
	}}
	m.state.Research.EfficiencyMult = 1

	rate := sim.RnDRatePerSec(m.state, m.cfg)
	if rate <= 0 {
		t.Fatal("setup: expected positive staff R&D rate")
	}

	now := time.Now()
	nm, _ := m.Update(tickMsg(now))
	got := nm.(Model)

	day := metrics.DayKey(now)
	staff := 0.0
	if p, ok := got.metricsDoc.Days[day]; ok && p.RnDInflow != nil {
		staff = p.RnDInflow[metrics.SourceStaff]
	}
	want := rate * tickDT
	if staff <= 0 {
		t.Fatalf("staff inflow=%v want > 0", staff)
	}
	if math.Abs(staff-want) > 1e-6 {
		t.Fatalf("staff inflow=%v want %v (RnDRatePerSec*tickDT)", staff, want)
	}
}
