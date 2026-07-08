package ledger

import (
	"path/filepath"
	"testing"

	"tokensmith/internal/ingest"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "ledger.json")
	if _, ok, _ := Load(p); ok {
		t.Fatal("missing ledger should report ok=false")
	}
	in := Ledger{
		CumIn: 100, CumOut: 50, UpdatedAt: 1700000000,
		Cursors: []ingest.CursorState{{Path: "/x.jsonl", Inode: 7, Offset: 42}},
	}
	if err := Save(p, in); err != nil {
		t.Fatal(err)
	}
	got, ok, err := Load(p)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if got.CumIn != 100 || got.CumOut != 50 || got.UpdatedAt != 1700000000 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if len(got.Cursors) != 1 || got.Cursors[0].Offset != 42 {
		t.Fatalf("cursors not round-tripped: %+v", got.Cursors)
	}
}
