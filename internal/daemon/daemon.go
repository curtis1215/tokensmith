// Package daemon harvests real Claude/Codex token usage into the ledger.
package daemon

import (
	"os"

	"tokensmith/internal/ingest"
	"tokensmith/internal/ledger"
)

// cursorMaxAgeSec bounds which files' cursors are persisted: a log untouched
// for this long will not receive new tokens, so its cursor is dropped to keep
// the ledger small. On restart PrimeUnknown re-primes such files to EOF.
const cursorMaxAgeSec = 7 * 86400

// Harvester tails the logs and accumulates token totals into a durable ledger.
type Harvester struct {
	poller     *ingest.Poller
	ledgerPath string
	cur        ledger.Ledger
}

// New loads any existing ledger and resumes from its persisted cursors, then
// primes every other tracked file to EOF — so months of old logs (and files
// pruned from a previous ledger) are not re-read.
func New(claudeDir, codexDir, ledgerPath string) *Harvester {
	p := ingest.NewPoller(claudeDir, codexDir)
	cur, ok, _ := ledger.Load(ledgerPath)
	if ok {
		p.ImportCursors(cur.Cursors)
	}
	p.PrimeUnknown()
	return &Harvester{poller: p, ledgerPath: ledgerPath, cur: cur}
}

// Step polls for new usage, folds it into the cumulative totals, and persists
// the ledger stamped with now (unix seconds), keeping only recently-active
// cursors so the file stays small.
func (h *Harvester) Step(now int64) error {
	for _, e := range h.poller.Poll() {
		h.cur.CumIn += e.InputTokens
		h.cur.CumOut += e.OutputTokens
	}
	h.cur.UpdatedAt = now
	h.cur.Cursors = pruneCursors(h.poller.ExportCursors(), now)
	return ledger.Save(h.ledgerPath, h.cur)
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
