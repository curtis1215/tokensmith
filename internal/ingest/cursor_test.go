package ingest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCursorExportImportResumes(t *testing.T) {
	claude := t.TempDir()
	f := filepath.Join(claude, "s.jsonl")
	line := `{"type":"assistant","timestamp":"2026-07-07T10:59:19Z","message":{"id":"A","usage":{"input_tokens":100,"output_tokens":50}}}` + "\n"
	os.WriteFile(f, []byte(line), 0o644)

	p1 := NewPoller(claude, t.TempDir())
	if got := p1.Poll(); len(got) != 1 {
		t.Fatalf("p1 first poll = %d, want 1", len(got))
	}
	saved := p1.ExportCursors()

	// A fresh poller restoring the cursors must NOT re-read the existing line.
	p2 := NewPoller(claude, t.TempDir())
	p2.ImportCursors(saved)
	if got := p2.Poll(); len(got) != 0 {
		t.Fatalf("p2 after restore = %d, want 0 (resumed)", len(got))
	}
}
