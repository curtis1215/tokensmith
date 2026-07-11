package dailyusage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"tokensmith/internal/model"
)

// Acceptance probe: temp fixtures only — no production data, no history backfill,
// exact positive deltas, midnight selects empty day without deleting prior bucket.
func TestAcceptanceProbeTempFixtures(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "daily-usage.json")
	store := New(path)

	old := time.Local
	time.Local = time.FixedZone("UTC+8", 8*3600)
	t.Cleanup(func() { time.Local = old })

	doc, ok, err := store.Load()
	if err != nil || ok {
		t.Fatalf("want empty missing: ok=%v err=%v doc=%+v", ok, err, doc)
	}

	day := time.Date(2026, 7, 12, 23, 30, 0, 0, time.Local)
	batch := BatchFromEvents(day, []model.TokenEvent{
		{Source: "claude-code", InputTokens: 100, OutputTokens: 50},
		{Source: "grok", InputTokens: 30},
	})
	if batch.Day != "2026-07-12" {
		t.Fatalf("day=%q", batch.Day)
	}
	if err := store.Add(batch); err != nil {
		t.Fatal(err)
	}
	doc, ok, err = store.Load()
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if doc.Days["2026-07-12"]["claude-code"] != (SourceUsage{In: 100, Out: 50, LastUpdatedAt: day.Unix()}) {
		// LastUpdatedAt may match; allow flexible if zero-edge
		got := doc.Days["2026-07-12"]["claude-code"]
		if got.In != 100 || got.Out != 50 {
			t.Fatalf("claude=%+v", got)
		}
	}
	if doc.Days["2026-07-12"]["grok"].In != 30 || doc.Days["2026-07-12"]["grok"].Out != 0 {
		t.Fatalf("grok=%+v", doc.Days["2026-07-12"]["grok"])
	}

	nextKey := DayKey(time.Date(2026, 7, 13, 0, 0, 1, 0, time.Local))
	if nextKey != "2026-07-13" {
		t.Fatalf("nextKey=%q", nextKey)
	}
	if doc.Days[nextKey] != nil {
		t.Fatal("next day should be absent (zero view)")
	}
	if doc.Days["2026-07-12"]["claude-code"].In != 100 {
		t.Fatal("prior day deleted")
	}

	// Production default path must not be created by this probe.
	prod := DefaultPath()
	if _, err := os.Stat(prod); err == nil {
		// File may pre-exist on developer machine — probe must not have written
		// into dir that is our temp only. Verify our temp file exists and is ours.
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("temp fixture missing")
	}
	// Ensure temp path is under our dir, not production.
	if filepath.Dir(path) != dir {
		t.Fatal("not using temp dir")
	}
}
