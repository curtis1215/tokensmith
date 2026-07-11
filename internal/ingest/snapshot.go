package ingest

import "tokensmith/internal/model"

// SnapshotSource reports a cumulative per-tool token snapshot. Unlike the
// append-only Poller, snapshot sources may rewrite their storage in place, so
// the daemon persists and subtracts a high-water mark before crediting R&D.
type SnapshotSource interface {
	Source() string
	Totals() (model.SourceTotals, error)
}
