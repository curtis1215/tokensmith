package ledger

import (
	"os"
	"path/filepath"
	"testing"

	"tokensmith/internal/ingest"
	"tokensmith/internal/model"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "ledger.json")
	if _, ok, _ := Load(p); ok {
		t.Fatal("missing ledger should report ok=false")
	}
	in := Ledger{
		Sources: map[string]model.SourceTotals{
			"claude-code": {In: 100, Out: 50},
			"codex":       {In: 20, Out: 10},
		},
		Snapshots: map[string]model.SourceTotals{
			"grok":     {In: 500},
			"opencode": {In: 80, Out: 40},
		},
		UpdatedAt: 1700000000,
		Cursors:   []ingest.CursorState{{Path: "/x.jsonl", Inode: 7, Offset: 42}},
	}
	if err := Save(p, in); err != nil {
		t.Fatal(err)
	}
	got, ok, err := Load(p)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if got.Sources["claude-code"] != (model.SourceTotals{In: 100, Out: 50}) {
		t.Fatalf("claude-code totals not round-tripped: %+v", got.Sources["claude-code"])
	}
	if got.Sources["codex"] != (model.SourceTotals{In: 20, Out: 10}) {
		t.Fatalf("codex totals not round-tripped: %+v", got.Sources["codex"])
	}
	if got.Snapshots["grok"] != (model.SourceTotals{In: 500}) ||
		got.Snapshots["opencode"] != (model.SourceTotals{In: 80, Out: 40}) {
		t.Fatalf("snapshot watermarks not round-tripped: %+v", got.Snapshots)
	}
	if got.UpdatedAt != 1700000000 {
		t.Fatalf("UpdatedAt not round-tripped: %v", got.UpdatedAt)
	}
	if len(got.Cursors) != 1 || got.Cursors[0].Offset != 42 {
		t.Fatalf("cursors not round-tripped: %+v", got.Cursors)
	}
	if got.TotalIn() != 120 || got.TotalOut() != 60 {
		t.Fatalf("TotalIn/TotalOut = %d/%d, want 120/60", got.TotalIn(), got.TotalOut())
	}
}

func TestLoadOldSchemaIgnoresLegacyFields(t *testing.T) {
	p := filepath.Join(t.TempDir(), "ledger.json")
	// Pre-migration ledger.json shape (flat cumIn/cumOut, no "sources" key).
	legacy := `{"cumIn":100,"cumOut":50,"updatedAt":123}`
	if err := os.WriteFile(p, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok, err := Load(p)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if got.UpdatedAt != 123 {
		t.Fatalf("UpdatedAt should still parse: %v", got.UpdatedAt)
	}
	if got.TotalIn() != 0 || got.TotalOut() != 0 {
		t.Fatalf("legacy cumIn/cumOut must not leak into Sources: %+v", got.Sources)
	}
}
