package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"tokensmith/internal/ledger"
)

func writeLine(t *testing.T, f, id string) {
	line := `{"type":"assistant","timestamp":"2026-07-07T10:59:19Z","message":{"id":"` + id + `","usage":{"input_tokens":100,"output_tokens":50}}}` + "\n"
	af, err := os.OpenFile(f, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	af.WriteString(line)
	af.Close()
}

func TestHarvesterAccumulatesAndResumes(t *testing.T) {
	claude, codex := t.TempDir(), t.TempDir()
	lp := filepath.Join(t.TempDir(), "ledger.json")
	f := filepath.Join(claude, "s.jsonl")

	h := New(claude, codex, lp)
	writeLine(t, f, "A")
	if err := h.Step(1000); err != nil {
		t.Fatal(err)
	}
	if l := h.Ledger(); l.CumIn != 100 || l.CumOut != 50 {
		t.Fatalf("after 1 line: %+v, want 100/50", l)
	}
	// second Step, no new data → unchanged
	h.Step(1001)
	if l := h.Ledger(); l.CumIn != 100 {
		t.Fatalf("no-new-data Step double counted: %+v", l)
	}
	// restart from persisted ledger → must not re-read the existing line
	h2 := New(claude, codex, lp)
	writeLine(t, f, "B")
	h2.Step(2000)
	if l := h2.Ledger(); l.CumIn != 200 || l.CumOut != 100 {
		t.Fatalf("after restart+1 line: %+v, want 200/100 (resumed, not re-read)", l)
	}

	got, ok, _ := ledger.Load(lp)
	if !ok || got.UpdatedAt != 2000 {
		t.Fatalf("ledger not persisted with UpdatedAt: %+v ok=%v", got, ok)
	}
}

func TestHarvesterPrimesHistoryOnFirstStart(t *testing.T) {
	claude, codex := t.TempDir(), t.TempDir()
	lp := filepath.Join(t.TempDir(), "ledger.json")
	f := filepath.Join(claude, "s.jsonl")
	writeLine(t, f, "OLD") // history present before daemon ever ran

	h := New(claude, codex, lp) // fresh start → prime to EOF
	h.Step(1000)
	if l := h.Ledger(); l.CumIn != 0 {
		t.Fatalf("first start should skip pre-existing history, got %+v", l)
	}
	writeLine(t, f, "NEW")
	h.Step(1001)
	if l := h.Ledger(); l.CumIn != 100 {
		t.Fatalf("post-prime line should count, got %+v", l)
	}
}
