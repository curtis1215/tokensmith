// Package dailyusage tracks raw per-source daily token growth for diagnostics.
// It is independent of ledger, meta, and game save; values never affect R&D.
package dailyusage

import (
	"os"
	"path/filepath"
	"sort"
	"time"

	"tokensmith/internal/model"
)

// SchemaVersion is the on-disk document version.
const SchemaVersion = 1

// MaxDays is the number of most-recent valid date keys retained.
const MaxDays = 7

// SourceUsage is one source's raw token totals for a single local day.
type SourceUsage struct {
	In            int   `json:"in"`
	Out           int   `json:"out"`
	LastUpdatedAt int64 `json:"lastUpdatedAt,omitempty"`
}

// Document is the versioned daily-usage file contents.
type Document struct {
	SchemaVersion int                               `json:"schemaVersion"`
	UpdatedAt     int64                             `json:"updatedAt"`
	Days          map[string]map[string]SourceUsage `json:"days"`
}

// Batch is a set of positive raw-token deltas observed at a specific local day.
// Day is preserved across retries so midnight cannot re-bucket the batch.
type Batch struct {
	Day        string
	ObservedAt int64
	Sources    map[string]model.SourceTotals
}

// DayKey returns the YYYY-MM-DD key for at in time.Local.
func DayKey(at time.Time) string {
	return at.In(time.Local).Format("2006-01-02")
}

// BatchFromEvents aggregates events into a Batch for the local day of at.
// Negative input/output components are treated as zero; empty sources and
// zero-total sources are omitted.
func BatchFromEvents(at time.Time, events []model.TokenEvent) Batch {
	b := Batch{
		Day:        DayKey(at),
		ObservedAt: at.Unix(),
		Sources:    map[string]model.SourceTotals{},
	}
	for _, e := range events {
		if e.Source == "" {
			continue
		}
		in, out := e.InputTokens, e.OutputTokens
		if in < 0 {
			in = 0
		}
		if out < 0 {
			out = 0
		}
		if in == 0 && out == 0 {
			continue
		}
		tot := b.Sources[e.Source]
		tot.In += in
		tot.Out += out
		b.Sources[e.Source] = tot
	}
	// Drop sources that ended at zero (all-negative components).
	for src, tot := range b.Sources {
		if tot.In == 0 && tot.Out == 0 {
			delete(b.Sources, src)
		}
	}
	return b
}

// Apply folds batch into doc. Returns false when the batch has no effect
// (invalid day, empty sources, or only non-positive components).
// Valid ISO date keys are retained; at most MaxDays newest keys remain.
func Apply(doc *Document, batch Batch) bool {
	if doc == nil {
		return false
	}
	if !validDayKey(batch.Day) {
		return false
	}
	// Pre-scan for any positive component.
	hasPositive := false
	for src, tot := range batch.Sources {
		if src == "" {
			continue
		}
		in, out := clampNonNeg(tot.In), clampNonNeg(tot.Out)
		if in > 0 || out > 0 {
			hasPositive = true
			break
		}
	}
	if !hasPositive {
		return false
	}

	if doc.Days == nil {
		doc.Days = map[string]map[string]SourceUsage{}
	}
	if doc.SchemaVersion == 0 {
		doc.SchemaVersion = SchemaVersion
	}
	day := doc.Days[batch.Day]
	if day == nil {
		day = map[string]SourceUsage{}
		doc.Days[batch.Day] = day
	}
	for src, tot := range batch.Sources {
		if src == "" {
			continue
		}
		in, out := clampNonNeg(tot.In), clampNonNeg(tot.Out)
		if in == 0 && out == 0 {
			continue
		}
		cur := day[src]
		cur.In += in
		cur.Out += out
		if batch.ObservedAt > 0 {
			cur.LastUpdatedAt = batch.ObservedAt
		}
		day[src] = cur
	}
	if batch.ObservedAt > 0 {
		doc.UpdatedAt = batch.ObservedAt
	}
	doc.SchemaVersion = SchemaVersion
	pruneDays(doc)
	return true
}

// DefaultPath is the standard daily-usage file location.
func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "tokensmith", "daily-usage.json")
}

func clampNonNeg(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

func validDayKey(day string) bool {
	if len(day) != 10 {
		return false
	}
	_, err := time.Parse("2006-01-02", day)
	return err == nil
}

func pruneDays(doc *Document) {
	if len(doc.Days) <= MaxDays {
		return
	}
	keys := make([]string, 0, len(doc.Days))
	for k := range doc.Days {
		keys = append(keys, k)
	}
	sort.Strings(keys) // ISO dates sort chronologically
	drop := len(keys) - MaxDays
	for i := 0; i < drop; i++ {
		delete(doc.Days, keys[i])
	}
}
