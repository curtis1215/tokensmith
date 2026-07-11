package dailyusage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tokensmith/internal/model"
)

func TestDayKeyUsesLocalCalendarDate(t *testing.T) {
	old := time.Local
	time.Local = time.FixedZone("UTC+8", 8*3600)
	t.Cleanup(func() { time.Local = old })

	// 2026-07-12 23:59 local → same day; UTC would already be next day.
	at := time.Date(2026, 7, 12, 23, 59, 0, 0, time.Local)
	if got := DayKey(at); got != "2026-07-12" {
		t.Fatalf("DayKey=%q, want 2026-07-12", got)
	}
	// Instant that is next day in UTC+8 but still 2026-07-12 in UTC.
	utc := time.Date(2026, 7, 12, 20, 0, 0, 0, time.UTC)
	if got := DayKey(utc); got != "2026-07-13" {
		t.Fatalf("DayKey(utc instant)=%q, want 2026-07-13", got)
	}
}

func TestBatchFromEventsAggregatesLocalDay(t *testing.T) {
	old := time.Local
	time.Local = time.FixedZone("UTC+8", 8*3600)
	t.Cleanup(func() { time.Local = old })
	at := time.Date(2026, 7, 12, 23, 59, 0, 0, time.Local)
	b := BatchFromEvents(at, []model.TokenEvent{
		{Source: "claude-code", InputTokens: 10, OutputTokens: 3},
		{Source: "claude-code", InputTokens: 7, OutputTokens: 2},
		{Source: "codex", InputTokens: 5, OutputTokens: 1},
	})
	if b.Day != "2026-07-12" {
		t.Fatalf("day=%q", b.Day)
	}
	if b.ObservedAt != at.Unix() {
		t.Fatalf("ObservedAt=%d, want %d", b.ObservedAt, at.Unix())
	}
	if b.Sources["claude-code"] != (model.SourceTotals{In: 17, Out: 5}) {
		t.Fatalf("claude-code=%+v", b.Sources["claude-code"])
	}
	if b.Sources["codex"] != (model.SourceTotals{In: 5, Out: 1}) {
		t.Fatalf("codex=%+v", b.Sources["codex"])
	}
}

func TestBatchFromEventsIgnoresEmptyAndNegativeComponents(t *testing.T) {
	at := time.Date(2026, 7, 12, 12, 0, 0, 0, time.Local)
	b := BatchFromEvents(at, []model.TokenEvent{
		{Source: "grok", InputTokens: -5, OutputTokens: 10}, // negative in ignored → only out if positive
		{Source: "grok", InputTokens: 3, OutputTokens: -2},
		{Source: "", InputTokens: 100, OutputTokens: 50},
		{Source: "codex", InputTokens: 0, OutputTokens: 0},
	})
	// negative components clamped to 0 when summing; zero-total sources omitted
	if got := b.Sources["grok"]; got != (model.SourceTotals{In: 3, Out: 10}) {
		t.Fatalf("grok=%+v, want In:3 Out:10", got)
	}
	if _, ok := b.Sources[""]; ok {
		t.Fatal("empty source should be omitted")
	}
	if _, ok := b.Sources["codex"]; ok {
		t.Fatal("zero-total source should be omitted")
	}
}

func TestApplyKeepsSevenNewestDays(t *testing.T) {
	var d Document
	for day := 1; day <= 8; day++ {
		Apply(&d, Batch{Day: fmt.Sprintf("2026-07-%02d", day), ObservedAt: int64(day),
			Sources: map[string]model.SourceTotals{"codex": {In: day}}})
	}
	if len(d.Days) != 7 {
		t.Fatalf("len(days)=%d, want 7; days=%+v", len(d.Days), d.Days)
	}
	if d.Days["2026-07-01"] != nil {
		t.Fatalf("oldest day should be pruned: days=%+v", d.Days)
	}
	if d.Days["2026-07-08"]["codex"].In != 8 {
		t.Fatalf("newest day missing: %+v", d.Days["2026-07-08"])
	}
}

func TestApplyAccumulatesAndClampsNegative(t *testing.T) {
	var d Document
	Apply(&d, Batch{
		Day: "2026-07-12", ObservedAt: 100,
		Sources: map[string]model.SourceTotals{
			"claude-code": {In: 10, Out: 5},
			"codex":       {In: 3, Out: 1},
		},
	})
	Apply(&d, Batch{
		Day: "2026-07-12", ObservedAt: 200,
		Sources: map[string]model.SourceTotals{
			"claude-code": {In: 7, Out: 2},
			"codex":       {In: -99, Out: 4}, // negative in ignored
		},
	})
	got := d.Days["2026-07-12"]["claude-code"]
	if got.In != 17 || got.Out != 7 || got.LastUpdatedAt != 200 {
		t.Fatalf("claude-code=%+v", got)
	}
	gotC := d.Days["2026-07-12"]["codex"]
	if gotC.In != 3 || gotC.Out != 5 {
		t.Fatalf("codex=%+v, want In:3 Out:5", gotC)
	}
	if d.UpdatedAt != 200 {
		t.Fatalf("UpdatedAt=%d, want 200", d.UpdatedAt)
	}
	if d.SchemaVersion != 1 {
		t.Fatalf("SchemaVersion=%d, want 1", d.SchemaVersion)
	}
}

func TestApplyRejectsInvalidDayAndEmptyBatch(t *testing.T) {
	var d Document
	if Apply(&d, Batch{Day: "not-a-date", ObservedAt: 1, Sources: map[string]model.SourceTotals{"codex": {In: 1}}}) {
		t.Fatal("invalid day should return false")
	}
	if Apply(&d, Batch{Day: "2026-07-12", ObservedAt: 1, Sources: nil}) {
		t.Fatal("empty sources should return false")
	}
	if Apply(&d, Batch{Day: "2026-07-12", ObservedAt: 1, Sources: map[string]model.SourceTotals{"codex": {In: 0, Out: 0}}}) {
		t.Fatal("zero-only batch should return false")
	}
	if d.Days != nil && len(d.Days) != 0 {
		t.Fatalf("document should stay empty: %+v", d)
	}
}

func TestApplyPreservesBatchOriginalDayAcrossMidnight(t *testing.T) {
	// Batch stamped with yesterday must not land in today's bucket.
	var d Document
	Apply(&d, Batch{
		Day: "2026-07-11", ObservedAt: 999,
		Sources: map[string]model.SourceTotals{"grok": {In: 42}},
	})
	if d.Days["2026-07-11"]["grok"].In != 42 {
		t.Fatalf("expected yesterday bucket: %+v", d.Days)
	}
	if d.Days["2026-07-12"] != nil {
		t.Fatal("must not create today bucket from yesterday batch")
	}
}

func TestApplyPositiveOnlyTimestamps(t *testing.T) {
	var d Document
	// Zero ObservedAt still applies deltas but should not write a zero lastUpdatedAt if we already have one?
	// Spec: update per-source and document timestamps with ObservedAt.
	Apply(&d, Batch{
		Day: "2026-07-12", ObservedAt: 50,
		Sources: map[string]model.SourceTotals{"opencode": {In: 1}},
	})
	if d.Days["2026-07-12"]["opencode"].LastUpdatedAt != 50 {
		t.Fatalf("LastUpdatedAt=%d", d.Days["2026-07-12"]["opencode"].LastUpdatedAt)
	}
	// Later zero ObservedAt: still apply tokens; timestamp update uses max or the provided value.
	// Spec says "Update per-source and document timestamps" with the batch ObservedAt.
	// Only positive ObservedAt should update timestamps (plan: positive-only timestamps).
	Apply(&d, Batch{
		Day: "2026-07-12", ObservedAt: 0,
		Sources: map[string]model.SourceTotals{"opencode": {In: 2}},
	})
	got := d.Days["2026-07-12"]["opencode"]
	if got.In != 3 {
		t.Fatalf("In=%d, want 3", got.In)
	}
	if got.LastUpdatedAt != 50 {
		t.Fatalf("zero ObservedAt must not clobber LastUpdatedAt: %d", got.LastUpdatedAt)
	}
	if d.UpdatedAt != 50 {
		t.Fatalf("UpdatedAt=%d, want 50", d.UpdatedAt)
	}
}

func TestDefaultPath(t *testing.T) {
	p := DefaultPath()
	if !strings.HasSuffix(p, filepath.Join("tokensmith", "daily-usage.json")) {
		t.Fatalf("DefaultPath=%q, want .../tokensmith/daily-usage.json", p)
	}
	dir, err := os.UserConfigDir()
	if err == nil && dir != "" && !strings.HasPrefix(p, dir) {
		t.Fatalf("DefaultPath=%q should be under UserConfigDir %q", p, dir)
	}
}
