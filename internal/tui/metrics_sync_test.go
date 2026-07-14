package tui

import (
	"path/filepath"
	"testing"
	"time"

	"tokensmith/internal/metrics"
	"tokensmith/internal/model"
)

func TestMetricsFlushPersistsSnapshot(t *testing.T) {
	dir := t.TempDir()
	save := filepath.Join(dir, "s.json")
	m := newAt(save)
	m.poller = ingestEmptyPoller(t)
	m.state.Models = []model.Model{{Online: true, Users: 42, Price: 12}}
	m.state.Resources.RnD = 7
	m.metricsSnapshotNow(time.Now())
	m.metricsDirty = true
	m.metricsFlush()
	s := metrics.New(filepath.Join(dir, "metrics-history.json"))
	doc, ok, err := s.Load()
	if err != nil || !ok {
		t.Fatalf("load: %v ok=%v", err, ok)
	}
	day := metrics.DayKey(time.Now())
	p := doc.Days[day]
	if p.Users != 42 {
		t.Fatalf("users=%v want 42", p.Users)
	}
	if p.MonthlyRevenue != 42*12 {
		t.Fatalf("revenue=%v want %v", p.MonthlyRevenue, 42*12)
	}
	if p.RnDStock != 7 {
		t.Fatalf("rnd=%v want 7", p.RnDStock)
	}
	if !p.OpenSet || p.OpenUsers != 42 {
		t.Fatalf("open snapshot not frozen: %+v", p)
	}
}

func TestMetricsDayRollResetsInflowOnly(t *testing.T) {
	old := time.Local
	time.Local = time.FixedZone("UTC+8", 8*3600)
	t.Cleanup(func() { time.Local = old })

	dir := t.TempDir()
	save := filepath.Join(dir, "s.json")
	m := newAt(save)
	m.poller = ingestEmptyPoller(t)

	yesterday := time.Date(2026, 7, 13, 23, 0, 0, 0, time.Local)
	today := time.Date(2026, 7, 14, 1, 0, 0, 0, time.Local)
	yKey := metrics.DayKey(yesterday)
	tKey := metrics.DayKey(today)

	m.metricsDay = yKey
	m.state.Models = []model.Model{{Online: true, Users: 10, Price: 5}}
	m.state.Resources.RnD = 3
	m.metricsSnapshotNow(yesterday)
	metrics.AddInflow(&m.metricsDoc, yKey, "claude-code", 1.5, yesterday.Unix())
	m.metricsDirty = true

	m.metricsMaybeRollDay(today)
	if m.metricsDay != tKey {
		t.Fatalf("metricsDay=%q want %q", m.metricsDay, tKey)
	}

	// Rolling should have flushed the dirty prior day to disk.
	s := metrics.New(filepath.Join(dir, "metrics-history.json"))
	doc, ok, err := s.Load()
	if err != nil || !ok {
		t.Fatalf("load after roll: %v ok=%v", err, ok)
	}
	yp := doc.Days[yKey]
	if yp.Users != 10 {
		t.Fatalf("yesterday users=%v want 10", yp.Users)
	}
	if yp.RnDInflow["claude-code"] != 1.5 {
		t.Fatalf("yesterday inflow=%v want 1.5", yp.RnDInflow["claude-code"])
	}

	// New day starts with empty inflow; stock snapshot is independent.
	m.state.Models = []model.Model{{Online: true, Users: 20, Price: 5}}
	m.state.Resources.RnD = 9
	m.metricsSnapshotNow(today)
	tp := m.metricsDoc.Days[tKey]
	if tp.Users != 20 {
		t.Fatalf("today users=%v want 20", tp.Users)
	}
	if tp.RnDInflow != nil && tp.RnDInflow["claude-code"] != 0 {
		t.Fatalf("today inflow should be empty, got %v", tp.RnDInflow)
	}
	// Old day still retained in memory.
	if m.metricsDoc.Days[yKey].RnDInflow["claude-code"] != 1.5 {
		t.Fatalf("old day inflow lost after roll")
	}
}

func TestMetricsNewAtPathsLoadsExistingHistory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metrics-history.json")
	day := metrics.DayKey(time.Now())
	doc := metrics.EmptyDocument()
	metrics.UpsertSnapshot(&doc, day, 99, 11, 5, time.Now().Unix())
	if err := metrics.New(path).Save(doc); err != nil {
		t.Fatal(err)
	}

	m := newAt(filepath.Join(dir, "s.json"))
	if m.metricsPath != path {
		t.Fatalf("metricsPath=%q want %q", m.metricsPath, path)
	}
	if m.metricsStore == nil {
		t.Fatal("metricsStore nil")
	}
	p, ok := m.metricsDoc.Days[day]
	if !ok || p.Users != 99 {
		t.Fatalf("loaded day %+v ok=%v", p, ok)
	}
}
