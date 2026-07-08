package ingest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPollerTailsNewLines(t *testing.T) {
	claude := t.TempDir()
	codex := t.TempDir()
	f := filepath.Join(claude, "session.jsonl")
	line := `{"type":"assistant","timestamp":"2026-07-07T10:59:19Z","message":{"usage":{"input_tokens":100,"output_tokens":50}}}` + "\n"
	if err := os.WriteFile(f, []byte(line+line), 0o644); err != nil {
		t.Fatal(err)
	}
	p := NewPoller(claude, codex)
	ev := p.Poll()
	if len(ev) != 2 {
		t.Fatalf("first poll = %d events, want 2", len(ev))
	}
	// second poll with no new data → 0
	if got := p.Poll(); len(got) != 0 {
		t.Fatalf("second poll = %d events, want 0 (cursor)", len(got))
	}
	// append one more line → 1 new event
	af, _ := os.OpenFile(f, os.O_APPEND|os.O_WRONLY, 0o644)
	af.WriteString(line)
	af.Close()
	if got := p.Poll(); len(got) != 1 {
		t.Fatalf("third poll = %d events, want 1", len(got))
	}
}

func TestPollerIgnoresPartialLine(t *testing.T) {
	claude := t.TempDir()
	f := filepath.Join(claude, "s.jsonl")
	full := `{"type":"assistant","timestamp":"2026-07-07T10:59:19Z","message":{"usage":{"input_tokens":100,"output_tokens":50}}}` + "\n"
	partial := `{"type":"assistant"` // no newline yet
	os.WriteFile(f, []byte(full+partial), 0o644)
	p := NewPoller(claude, t.TempDir())
	if got := p.Poll(); len(got) != 1 {
		t.Fatalf("poll = %d, want 1 (partial line held back)", len(got))
	}
}

func TestPollerPrimeSkipsHistory(t *testing.T) {
	claude := t.TempDir()
	f := filepath.Join(claude, "session.jsonl")
	line := `{"type":"assistant","timestamp":"2026-07-07T10:59:19Z","message":{"usage":{"input_tokens":100,"output_tokens":50}}}` + "\n"
	os.WriteFile(f, []byte(line+line), 0o644)
	p := NewPoller(claude, t.TempDir())
	p.Prime() // move cursors to end; skip existing history
	if got := p.Poll(); len(got) != 0 {
		t.Fatalf("after prime, poll = %d events, want 0 (history skipped)", len(got))
	}
	// usage appended after priming is harvested
	af, _ := os.OpenFile(f, os.O_APPEND|os.O_WRONLY, 0o644)
	af.WriteString(line)
	af.Close()
	if got := p.Poll(); len(got) != 1 {
		t.Fatalf("after append, poll = %d events, want 1", len(got))
	}
}

func TestPollerDedupsByMessageID(t *testing.T) {
	claude := t.TempDir()
	f := filepath.Join(claude, "session.jsonl")
	// Claude writes one response across several rows with the same message id.
	dup := `{"type":"assistant","timestamp":"2026-07-07T10:59:19Z","message":{"id":"msg_A","usage":{"input_tokens":100,"output_tokens":50}}}` + "\n"
	other := `{"type":"assistant","timestamp":"2026-07-07T10:59:20Z","message":{"id":"msg_B","usage":{"input_tokens":10,"output_tokens":5}}}` + "\n"
	os.WriteFile(f, []byte(dup+dup+other), 0o644)
	p := NewPoller(claude, t.TempDir())
	if got := p.Poll(); len(got) != 2 { // msg_A once + msg_B once, not 3
		t.Fatalf("poll = %d events, want 2 (deduped by message id)", len(got))
	}
}
