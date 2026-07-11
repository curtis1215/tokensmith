// Package daemon harvests real Claude/Codex token usage into the ledger.
package daemon

import (
	"errors"
	"fmt"
	"os"

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
}

// New loads any existing ledger and resumes from its persisted cursors, then
// primes every other tracked file to EOF — so months of old logs (and files
// pruned from a previous ledger) are not re-read.
func New(claudeDir, codexDir, ledgerPath string) *Harvester {
	return NewWithSources([]string{claudeDir}, []string{codexDir}, nil, ledgerPath)
}

// NewWithSources builds a harvester over multiple append-only roots plus
// cumulative snapshot sources such as Grok and OpenCode.
func NewWithSources(
	claudeDirs, codexDirs []string,
	snapshots []ingest.SnapshotSource,
	ledgerPath string,
) *Harvester {
	p := ingest.NewPollerWithRoots(claudeDirs, codexDirs)
	cur, ok, _ := ledger.Load(ledgerPath)
	if ok {
		p.ImportCursors(cur.Cursors)
	}
	p.PrimeUnknown()
	return &Harvester{poller: p, snapshots: snapshots, ledgerPath: ledgerPath, cur: cur}
}

// Step polls for new usage, folds it into the cumulative per-source totals,
// and persists the ledger stamped with now (unix seconds), keeping only
// recently-active cursors so the file stays small.
func (h *Harvester) Step(now int64) error {
	for _, e := range h.poller.Poll() {
		h.add(e.Source, model.SourceTotals{In: e.InputTokens, Out: e.OutputTokens})
	}
	var snapshotErrors []error
	for _, source := range h.snapshots {
		current, err := source.Totals()
		if err != nil {
			snapshotErrors = append(snapshotErrors, fmt.Errorf("%s snapshot: %w", source.Source(), err))
			continue
		}
		h.applySnapshot(source.Source(), current)
	}
	h.cur.UpdatedAt = now
	h.cur.Cursors = pruneCursors(h.poller.ExportCursors(), now)
	snapshotErrors = append(snapshotErrors, ledger.Save(h.ledgerPath, h.cur))
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

func (h *Harvester) applySnapshot(source string, current model.SourceTotals) {
	if source == "" {
		return
	}
	if h.cur.Snapshots == nil {
		h.cur.Snapshots = map[string]model.SourceTotals{}
	}
	previous, known := h.cur.Snapshots[source]
	h.cur.Snapshots[source] = current
	if !known || current.In < previous.In || current.Out < previous.Out {
		return
	}
	h.add(source, model.SourceTotals{In: current.In - previous.In, Out: current.Out - previous.Out})
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
