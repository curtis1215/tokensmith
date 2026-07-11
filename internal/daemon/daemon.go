// Package daemon harvests real Claude/Codex token usage into the ledger.
package daemon

import (
	"errors"
	"fmt"
	"os"
	"time"

	"tokensmith/internal/dailyusage"
	"tokensmith/internal/ingest"
	"tokensmith/internal/ledger"
	"tokensmith/internal/model"
)

// cursorMaxAgeSec bounds which files' cursors are persisted: a log untouched
// for this long will not receive new tokens, so its cursor is dropped to keep
// the ledger small. On restart PrimeUnknown re-primes such files to EOF.
const cursorMaxAgeSec = 7 * 86400

// Harvester tails the logs and accumulates token totals into a durable ledger.
type Harvester struct {
	poller     *ingest.Poller
	snapshots  []ingest.SnapshotSource
	ledgerPath string
	cur        ledger.Ledger
	daily      *dailyusage.Buffer
}

// New loads any existing ledger and resumes from its persisted cursors, then
// primes every other tracked file to EOF — so months of old logs (and files
// pruned from a previous ledger) are not re-read.
func New(claudeDir, codexDir, ledgerPath string) *Harvester {
	return NewWithSources([]string{claudeDir}, []string{codexDir}, nil, ledgerPath)
}

// NewWithSources builds a harvester over multiple append-only roots plus
// cumulative snapshot sources such as Grok and OpenCode. A disabled daily
// buffer is installed for compatibility.
func NewWithSources(
	claudeDirs, codexDirs []string,
	snapshots []ingest.SnapshotSource,
	ledgerPath string,
) *Harvester {
	return NewWithSourcesAndDaily(claudeDirs, codexDirs, snapshots, dailyusage.NewBuffer(nil), ledgerPath)
}

// NewWithSourcesAndDaily is NewWithSources with an explicit daily-usage buffer.
// Pass dailyusage.NewBuffer(nil) to disable daily recording.
func NewWithSourcesAndDaily(
	claudeDirs, codexDirs []string,
	snapshots []ingest.SnapshotSource,
	daily *dailyusage.Buffer,
	ledgerPath string,
) *Harvester {
	p := ingest.NewPollerWithRoots(claudeDirs, codexDirs)
	cur, ok, _ := ledger.Load(ledgerPath)
	if ok {
		p.ImportCursors(cur.Cursors)
	}
	p.PrimeUnknown()
	if daily == nil {
		daily = dailyusage.NewBuffer(nil)
	}
	return &Harvester{poller: p, snapshots: snapshots, ledgerPath: ledgerPath, cur: cur, daily: daily}
}

// Step polls for new usage, folds it into the cumulative per-source totals,
// records exact positive deltas into the daily-usage store, and persists the
// ledger stamped with now (unix seconds), keeping only recently-active cursors
// so the file stays small.
//
// Daily-stat failure never blocks ledger persistence: both errors are joined.
func (h *Harvester) Step(now int64) error {
	var harvested []model.TokenEvent
	for _, e := range h.poller.Poll() {
		h.add(e.Source, model.SourceTotals{In: e.InputTokens, Out: e.OutputTokens})
		harvested = append(harvested, e)
	}
	var snapshotErrors []error
	for _, source := range h.snapshots {
		current, present, err := source.Totals()
		if err != nil {
			snapshotErrors = append(snapshotErrors, fmt.Errorf("%s snapshot: %w", source.Source(), err))
			continue
		}
		if !present {
			continue
		}
		if delta, ok := h.applySnapshot(source.Source(), current); ok {
			harvested = append(harvested, model.TokenEvent{
				Source:       source.Source(),
				InputTokens:  delta.In,
				OutputTokens: delta.Out,
			})
		}
	}
	h.cur.UpdatedAt = now
	h.cur.Cursors = pruneCursors(h.poller.ExportCursors(), now)

	batch := dailyusage.BatchFromEvents(time.Unix(now, 0), harvested)
	dailyErr := h.daily.Record(batch) // empty batch also flushes pending work

	ledgerErr := ledger.Save(h.ledgerPath, h.cur)
	snapshotErrors = append(snapshotErrors, dailyErr, ledgerErr)
	return errors.Join(snapshotErrors...)
}

func (h *Harvester) add(source string, delta model.SourceTotals) {
	if source == "" || delta.In == 0 && delta.Out == 0 {
		return
	}
	if h.cur.Sources == nil {
		h.cur.Sources = map[string]model.SourceTotals{}
	}
	total := h.cur.Sources[source]
	total.In += delta.In
	total.Out += delta.Out
	h.cur.Sources[source] = total
}

// applySnapshot updates the snapshot watermark and returns a positive delta
// only for known, non-decreasing sources. First observation primes without
// awarding; decreases rebaseline without awarding.
func (h *Harvester) applySnapshot(source string, current model.SourceTotals) (model.SourceTotals, bool) {
	if source == "" {
		return model.SourceTotals{}, false
	}
	if h.cur.Snapshots == nil {
		h.cur.Snapshots = map[string]model.SourceTotals{}
	}
	previous, known := h.cur.Snapshots[source]
	h.cur.Snapshots[source] = current
	if !known || current.In < previous.In || current.Out < previous.Out {
		return model.SourceTotals{}, false
	}
	delta := model.SourceTotals{In: current.In - previous.In, Out: current.Out - previous.Out}
	if delta.In == 0 && delta.Out == 0 {
		return model.SourceTotals{}, false
	}
	h.add(source, delta)
	return delta, true
}

// pruneCursors keeps only cursors for files modified within cursorMaxAgeSec.
func pruneCursors(cs []ingest.CursorState, now int64) []ingest.CursorState {
	out := make([]ingest.CursorState, 0, len(cs))
	for _, c := range cs {
		if fi, err := os.Stat(c.Path); err == nil && now-fi.ModTime().Unix() <= cursorMaxAgeSec {
			out = append(out, c)
		}
	}
	return out
}

// Ledger returns the current in-memory ledger snapshot.
func (h *Harvester) Ledger() ledger.Ledger { return h.cur }
