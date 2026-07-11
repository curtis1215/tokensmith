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

func TestPrimeUnknownSkipsHistoryForUncursoredFiles(t *testing.T) {
	claude := t.TempDir()
	f := filepath.Join(claude, "s.jsonl")
	line := `{"type":"assistant","timestamp":"2026-07-07T10:59:19Z","message":{"id":"A","usage":{"input_tokens":100,"output_tokens":50}}}` + "\n"
	os.WriteFile(f, []byte(line), 0o644)
	p := NewPoller(claude, t.TempDir())
	p.PrimeUnknown() // no cursor yet → prime to EOF, skip history
	if got := p.Poll(); len(got) != 0 {
		t.Fatalf("PrimeUnknown should skip existing history, got %d", len(got))
	}
}

func TestPrimeUnknownLeavesExistingCursors(t *testing.T) {
	claude := t.TempDir()
	f := filepath.Join(claude, "s.jsonl")
	line := `{"type":"assistant","timestamp":"2026-07-07T10:59:19Z","message":{"id":"A","usage":{"input_tokens":100,"output_tokens":50}}}` + "\n"
	os.WriteFile(f, []byte(line), 0o644)
	p := NewPoller(claude, t.TempDir())
	p.Poll() // establishes a cursor at EOF
	line2 := `{"type":"assistant","timestamp":"2026-07-07T10:59:20Z","message":{"id":"B","usage":{"input_tokens":10,"output_tokens":5}}}` + "\n"
	af, _ := os.OpenFile(f, os.O_APPEND|os.O_WRONLY, 0o644)
	af.WriteString(line2)
	af.Close()
	p.PrimeUnknown() // f is already cursored → must NOT skip the appended line
	if got := p.Poll(); len(got) != 1 {
		t.Fatalf("PrimeUnknown wrongly re-primed a cursored file, got %d", len(got))
	}
}

func TestPollerReadsRotatedFileAfterRegrow(t *testing.T) {
	claude := t.TempDir()
	f := filepath.Join(claude, "session.jsonl")
	mk := func(id string) string {
		return `{"type":"assistant","timestamp":"2026-07-07T10:59:19Z","message":{"id":"` + id + `","usage":{"input_tokens":100,"output_tokens":50}}}` + "\n"
	}
	// Initial file: 2 lines; poll consumes them, cursor sits at their end.
	os.WriteFile(f, []byte(mk("A")+mk("B")), 0o644)
	p := NewPoller(claude, t.TempDir())
	if got := p.Poll(); len(got) != 2 {
		t.Fatalf("initial poll = %d, want 2", len(got))
	}
	// Rotate: replace with a NEW file (new inode) whose 3 lines already exceed
	// the old byte offset. A pure byte-offset cursor would skip the early lines.
	os.Remove(f)
	os.WriteFile(f, []byte(mk("C")+mk("D")+mk("E")), 0o644)
	if got := p.Poll(); len(got) != 3 {
		t.Fatalf("post-rotation poll = %d, want 3 (all new lines read)", len(got))
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

func TestCodexSessionRootsIncludeDefaultEnvAndOrcaWithoutDuplicates(t *testing.T) {
	home := t.TempDir()
	custom := filepath.Join(home, "custom-codex")
	roots := CodexSessionRoots(home, map[string]string{"CODEX_HOME": custom})

	want := []string{
		filepath.Join(custom, "sessions"),
		filepath.Join(home, ".codex", "sessions"),
		filepath.Join(home, "Library", "Application Support", "orca", "codex-runtime-home", "home", "sessions"),
	}
	if len(roots) != len(want) {
		t.Fatalf("CodexSessionRoots = %v, want %v", roots, want)
	}
	for i := range want {
		if roots[i] != want[i] {
			t.Fatalf("root[%d] = %q, want %q", i, roots[i], want[i])
		}
	}

	dup := CodexSessionRoots(home, map[string]string{"CODEX_HOME": filepath.Join(home, ".codex")})
	if len(dup) != 2 {
		t.Fatalf("duplicate CODEX_HOME should collapse, got %v", dup)
	}
}

func TestPollerReadsMultipleCodexRoots(t *testing.T) {
	claude := t.TempDir()
	codexA, codexB := t.TempDir(), t.TempDir()
	line := `{"timestamp":"2026-07-07T10:59:19Z","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":30,"output_tokens":15}}}}` + "\n"
	if err := os.WriteFile(filepath.Join(codexA, "a.jsonl"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexB, "b.jsonl"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}

	p := NewPollerWithRoots([]string{claude}, []string{codexA, codexB, codexA})
	if got := p.Poll(); len(got) != 2 {
		t.Fatalf("multi-root poll = %d events, want 2", len(got))
	}
}

func TestPollerDoesNotDoubleReadHardLinkedSessionAcrossRoots(t *testing.T) {
	codexA, codexB := t.TempDir(), t.TempDir()
	original := filepath.Join(codexA, "shared.jsonl")
	linked := filepath.Join(codexB, "shared.jsonl")
	line := `{"timestamp":"2026-07-07T10:59:19Z","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":30,"output_tokens":15}}}}` + "\n"
	if err := os.WriteFile(original, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(original, linked); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}

	p := NewPollerWithRoots(nil, []string{codexA, codexB})
	if got := p.Poll(); len(got) != 1 {
		t.Fatalf("hard-linked rollout emitted %d events, want 1", len(got))
	}
}

func TestSnapshotSourcePathsRespectEnvironment(t *testing.T) {
	home := t.TempDir()
	env := map[string]string{
		"GROK_HOME":     filepath.Join(home, "grok-custom"),
		"XDG_DATA_HOME": filepath.Join(home, "xdg-data"),
	}
	if got := GrokHome(home, env); got != env["GROK_HOME"] {
		t.Fatalf("GrokHome = %q, want %q", got, env["GROK_HOME"])
	}
	if got := OpenCodeDatabasePath(home, env); got != filepath.Join(env["XDG_DATA_HOME"], "opencode", "opencode.db") {
		t.Fatalf("OpenCodeDatabasePath = %q", got)
	}

	if got := GrokHome(home, nil); got != filepath.Join(home, ".grok") {
		t.Fatalf("default GrokHome = %q", got)
	}
	if got := OpenCodeDatabasePath(home, nil); got != filepath.Join(home, ".local", "share", "opencode", "opencode.db") {
		t.Fatalf("default OpenCodeDatabasePath = %q", got)
	}
}
