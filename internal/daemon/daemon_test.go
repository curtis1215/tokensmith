package daemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"tokensmith/internal/ingest"
	"tokensmith/internal/ledger"
	"tokensmith/internal/model"
)

type fakeSnapshotSource struct {
	source string
	totals model.SourceTotals
	err    error
}

func (s *fakeSnapshotSource) Source() string { return s.source }
func (s *fakeSnapshotSource) Totals() (model.SourceTotals, error) {
	return s.totals, s.err
}

func TestHarvesterSnapshotErrorStillPersistsHealthySources(t *testing.T) {
	lp := filepath.Join(t.TempDir(), "ledger.json")
	good := &fakeSnapshotSource{source: "opencode", totals: model.SourceTotals{In: 10}}
	bad := &fakeSnapshotSource{source: "grok", err: errors.New("temporarily unavailable")}
	h := NewWithSources(nil, nil, []ingest.SnapshotSource{good, bad}, lp)
	if err := h.Step(1000); err == nil {
		t.Fatal("snapshot failure should be reported")
	}
	good.totals = model.SourceTotals{In: 15}
	if err := h.Step(1001); err == nil {
		t.Fatal("snapshot failure should remain visible")
	}
	got, ok, err := ledger.Load(lp)
	if err != nil || !ok {
		t.Fatalf("healthy ledger update was not persisted: ok=%v err=%v", ok, err)
	}
	if got.Sources["opencode"] != (model.SourceTotals{In: 5}) {
		t.Fatalf("healthy snapshot delta missing: %+v", got.Sources)
	}
}

func writeLine(t *testing.T, f, id string) {
	line := `{"type":"assistant","timestamp":"2026-07-07T10:59:19Z","message":{"id":"` + id + `","usage":{"input_tokens":100,"output_tokens":50}}}` + "\n"
	af, err := os.OpenFile(f, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	af.WriteString(line)
	af.Close()
}

func writeCodexLine(t *testing.T, f string, in, out int) {
	line := fmt.Sprintf(`{"timestamp":"2026-07-07T10:59:19Z","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":%d,"output_tokens":%d}}}}`+"\n", in, out)
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
	if l := h.Ledger(); l.TotalIn() != 100 || l.TotalOut() != 50 {
		t.Fatalf("after 1 line: %+v, want 100/50", l)
	}
	// second Step, no new data → unchanged
	h.Step(1001)
	if l := h.Ledger(); l.TotalIn() != 100 {
		t.Fatalf("no-new-data Step double counted: %+v", l)
	}
	// restart from persisted ledger → must not re-read the existing line
	h2 := New(claude, codex, lp)
	writeLine(t, f, "B")
	h2.Step(2000)
	if l := h2.Ledger(); l.TotalIn() != 200 || l.TotalOut() != 100 {
		t.Fatalf("after restart+1 line: %+v, want 200/100 (resumed, not re-read)", l)
	}

	got, ok, _ := ledger.Load(lp)
	if !ok || got.UpdatedAt != 2000 {
		t.Fatalf("ledger not persisted with UpdatedAt: %+v ok=%v", got, ok)
	}
}

func TestStepSplitsBySource(t *testing.T) {
	claude, codex := t.TempDir(), t.TempDir()
	lp := filepath.Join(t.TempDir(), "ledger.json")
	cf := filepath.Join(claude, "s.jsonl")
	xf := filepath.Join(codex, "s.jsonl")

	h := New(claude, codex, lp)
	writeLine(t, cf, "A")         // claude-code: in=100 out=50
	writeCodexLine(t, xf, 30, 15) // codex: in=30 out=15
	if err := h.Step(1000); err != nil {
		t.Fatal(err)
	}
	l := h.Ledger()
	if l.Sources["claude-code"].In != 100 || l.Sources["claude-code"].Out != 50 {
		t.Fatalf("claude-code totals = %+v, want 100/50", l.Sources["claude-code"])
	}
	if l.Sources["codex"].In != 30 || l.Sources["codex"].Out != 15 {
		t.Fatalf("codex totals = %+v, want 30/15", l.Sources["codex"])
	}
	if l.TotalIn() != 130 || l.TotalOut() != 65 {
		t.Fatalf("combined totals = %d/%d, want 130/65", l.TotalIn(), l.TotalOut())
	}
}

func TestStepPrunesOldFileCursors(t *testing.T) {
	claude, codex := t.TempDir(), t.TempDir()
	lp := filepath.Join(t.TempDir(), "ledger.json")
	f := filepath.Join(claude, "s.jsonl")

	h := New(claude, codex, lp)
	writeLine(t, f, "A")
	now := time.Now().Unix()
	h.Step(now) // harvest A → cursor for f exists
	// age the file well past the retention window
	past := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(f, past, past); err != nil {
		t.Fatal(err)
	}
	h.Step(now)

	got, _, _ := ledger.Load(lp)
	if len(got.Cursors) != 0 {
		t.Fatalf("stale file cursor should be pruned, got %d cursors", len(got.Cursors))
	}
	if got.TotalIn() != 100 {
		t.Fatalf("pruning must not change totals: %+v", got)
	}
}

func TestPrunedFileNotRereadOnRestart(t *testing.T) {
	claude, codex := t.TempDir(), t.TempDir()
	lp := filepath.Join(t.TempDir(), "ledger.json")
	f := filepath.Join(claude, "s.jsonl")

	h := New(claude, codex, lp)
	writeLine(t, f, "A")
	now := time.Now().Unix()
	h.Step(now) // cumIn 100
	past := time.Now().Add(-30 * 24 * time.Hour)
	os.Chtimes(f, past, past)
	h.Step(now) // prune drops f's cursor from the ledger

	// Restart: f has no persisted cursor, but PrimeUnknown must prime it to EOF
	// rather than re-reading "A" (which would inflate the totals).
	h2 := New(claude, codex, lp)
	h2.Step(now)
	if l := h2.Ledger(); l.TotalIn() != 100 {
		t.Fatalf("pruned file re-read on restart (inflation): %+v", l)
	}
}

func TestHarvesterPrimesHistoryOnFirstStart(t *testing.T) {
	claude, codex := t.TempDir(), t.TempDir()
	lp := filepath.Join(t.TempDir(), "ledger.json")
	f := filepath.Join(claude, "s.jsonl")
	writeLine(t, f, "OLD") // history present before daemon ever ran

	h := New(claude, codex, lp) // fresh start → prime to EOF
	h.Step(1000)
	if l := h.Ledger(); l.TotalIn() != 0 {
		t.Fatalf("first start should skip pre-existing history, got %+v", l)
	}
	writeLine(t, f, "NEW")
	h.Step(1001)
	if l := h.Ledger(); l.TotalIn() != 100 {
		t.Fatalf("post-prime line should count, got %+v", l)
	}
}

func TestHarvesterSnapshotSourcesPrimeDeltaAndRebaseline(t *testing.T) {
	lp := filepath.Join(t.TempDir(), "ledger.json")
	grok := &fakeSnapshotSource{source: "grok", totals: model.SourceTotals{In: 1000}}
	opencode := &fakeSnapshotSource{source: "opencode", totals: model.SourceTotals{In: 200, Out: 50}}
	h := NewWithSources(nil, nil, []ingest.SnapshotSource{grok, opencode}, lp)

	// Existing source history is a baseline, not an initial windfall.
	if err := h.Step(1000); err != nil {
		t.Fatal(err)
	}
	if l := h.Ledger(); l.TotalIn() != 0 || l.TotalOut() != 0 {
		t.Fatalf("first snapshot observation must prime only: %+v", l)
	}
	if l := h.Ledger(); l.Snapshots["grok"] != (model.SourceTotals{In: 1000}) ||
		l.Snapshots["opencode"] != (model.SourceTotals{In: 200, Out: 50}) {
		t.Fatalf("snapshot baselines not persisted in memory: %+v", l.Snapshots)
	}

	grok.totals = model.SourceTotals{In: 1120}
	opencode.totals = model.SourceTotals{In: 230, Out: 59}
	if err := h.Step(1001); err != nil {
		t.Fatal(err)
	}
	if got := h.Ledger().Sources["grok"]; got != (model.SourceTotals{In: 120}) {
		t.Fatalf("Grok delta = %+v, want In=120", got)
	}
	if got := h.Ledger().Sources["opencode"]; got != (model.SourceTotals{In: 30, Out: 9}) {
		t.Fatalf("OpenCode delta = %+v, want 30/9", got)
	}

	// Cleanup or DB rebuild may reduce a cumulative snapshot. Rebaseline now,
	// then count growth from the new lower value on the next step.
	grok.totals = model.SourceTotals{In: 20}
	if err := h.Step(1002); err != nil {
		t.Fatal(err)
	}
	if got := h.Ledger().Sources["grok"]; got != (model.SourceTotals{In: 120}) {
		t.Fatalf("decrease must not add or subtract credits: %+v", got)
	}
	grok.totals = model.SourceTotals{In: 27}
	if err := h.Step(1003); err != nil {
		t.Fatal(err)
	}
	if got := h.Ledger().Sources["grok"]; got != (model.SourceTotals{In: 127}) {
		t.Fatalf("post-rebaseline growth = %+v, want In=127", got)
	}

	// A restart resumes the persisted snapshot watermark.
	grok.totals = model.SourceTotals{In: 30}
	h2 := NewWithSources(nil, nil, []ingest.SnapshotSource{grok}, lp)
	if err := h2.Step(1004); err != nil {
		t.Fatal(err)
	}
	if got := h2.Ledger().Sources["grok"]; got != (model.SourceTotals{In: 130}) {
		t.Fatalf("restart snapshot delta = %+v, want In=130", got)
	}
}
