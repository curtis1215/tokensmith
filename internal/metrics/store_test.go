package metrics

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "metrics-history.json")
	s := New(p)
	doc := EmptyDocument()
	UpsertSnapshot(&doc, "2026-07-14", 1, 2, 3, 10)
	if err := s.Save(doc); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.Load()
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if got.Days["2026-07-14"].Users != 1 {
		t.Fatalf("users=%v", got.Days["2026-07-14"].Users)
	}
}

func TestStoreMissing(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "nope.json"))
	_, ok, err := s.Load()
	if err != nil || ok {
		t.Fatalf("want missing ok=false err=nil, got ok=%v err=%v", ok, err)
	}
}

func TestStoreCorruptRecovers(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "metrics-history.json")
	if err := os.WriteFile(p, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := New(p)
	doc, ok, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !ok || doc.SchemaVersion != SchemaVersion {
		t.Fatalf("recover: ok=%v schema=%d", ok, doc.SchemaVersion)
	}
	// original renamed to corrupt-*
	matches, _ := filepath.Glob(p + ".corrupt-*")
	if len(matches) != 1 {
		t.Fatalf("expected corrupt backup, got %v", matches)
	}
}
