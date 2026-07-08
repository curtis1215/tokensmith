// Package daemon harvests real Claude/Codex token usage into the ledger.
package daemon

import (
	"tokensmith/internal/ingest"
	"tokensmith/internal/ledger"
)

// Harvester tails the logs and accumulates token totals into a durable ledger.
type Harvester struct {
	poller     *ingest.Poller
	ledgerPath string
	cur        ledger.Ledger
}

// New loads any existing ledger and resumes from its cursors; on a fresh start
// it primes the poller to EOF so months of old logs are not counted.
func New(claudeDir, codexDir, ledgerPath string) *Harvester {
	p := ingest.NewPoller(claudeDir, codexDir)
	cur, ok, _ := ledger.Load(ledgerPath)
	if ok {
		p.ImportCursors(cur.Cursors)
	} else {
		p.Prime() // skip history on first-ever start
	}
	return &Harvester{poller: p, ledgerPath: ledgerPath, cur: cur}
}

// Step polls for new usage, folds it into the cumulative totals, and persists
// the ledger stamped with now (unix seconds).
func (h *Harvester) Step(now int64) error {
	for _, e := range h.poller.Poll() {
		h.cur.CumIn += e.InputTokens
		h.cur.CumOut += e.OutputTokens
	}
	h.cur.UpdatedAt = now
	h.cur.Cursors = h.poller.ExportCursors()
	return ledger.Save(h.ledgerPath, h.cur)
}

// Ledger returns the current in-memory ledger snapshot.
func (h *Harvester) Ledger() ledger.Ledger { return h.cur }
