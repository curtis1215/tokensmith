// Package ledger persists the daemon's cumulative token harvest so the TUI can
// consume it (online and offline) without re-reading raw logs.
package ledger

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"tokensmith/internal/ingest"
	"tokensmith/internal/model"
)

// Ledger is the monotonically-growing per-source harvest totals plus durable
// cursors. Sources is keyed by TokenEvent.Source ("claude-code" / "codex").
// Older ledger.json files used flat cumIn/cumOut ints instead of Sources —
// those legacy fields are simply absent from a freshly-loaded Ledger (zero
// value), which the daemon treats as "start counting from here."
type Ledger struct {
	Sources   map[string]model.SourceTotals `json:"sources"`
	UpdatedAt int64                         `json:"updatedAt"`
	Cursors   []ingest.CursorState          `json:"cursors,omitempty"`
}

// TotalIn sums InputTokens across every source.
func (l Ledger) TotalIn() int {
	var n int
	for _, s := range l.Sources {
		n += s.In
	}
	return n
}

// TotalOut sums OutputTokens across every source.
func (l Ledger) TotalOut() int {
	var n int
	for _, s := range l.Sources {
		n += s.Out
	}
	return n
}

// DefaultPath is the standard ledger location.
func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "tokensmith", "ledger.json")
}

// Save writes the ledger atomically (temp file + rename).
func Save(path string, l Ledger) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(l)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Load reads the ledger. ok is false when the file does not exist yet.
func Load(path string) (Ledger, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Ledger{}, false, nil
	}
	if err != nil {
		return Ledger{}, false, err
	}
	var l Ledger
	if err := json.Unmarshal(data, &l); err != nil {
		return Ledger{}, false, err
	}
	return l, true, nil
}
