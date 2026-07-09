package store

import (
	"path/filepath"
	"testing"

	"tokensmith/internal/model"
)

func TestMetaRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "meta.json")
	if _, ok, _ := LoadMeta(p); ok {
		t.Fatal("missing meta should be ok=false")
	}
	in := Meta{
		ConsumedSources: map[string]model.SourceTotals{
			"claude-code": {In: 10, Out: 5},
		},
		LastRealUnix:   42,
		LastActiveDate: "2026-07-10",
		StreakDays:     3,
	}
	if err := SaveMeta(p, in); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadMeta(p)
	if err != nil || !ok {
		t.Fatalf("meta round-trip: ok=%v err=%v", ok, err)
	}
	if got.ConsumedSources["claude-code"] != (model.SourceTotals{In: 10, Out: 5}) {
		t.Fatalf("ConsumedSources not round-tripped: %+v", got.ConsumedSources)
	}
	if got.LastRealUnix != 42 || got.LastActiveDate != "2026-07-10" || got.StreakDays != 3 {
		t.Fatalf("meta round-trip mismatch: %+v", got)
	}
}
