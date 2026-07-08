package store

import (
	"path/filepath"
	"testing"
)

func TestMetaRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "meta.json")
	if _, ok, _ := LoadMeta(p); ok {
		t.Fatal("missing meta should be ok=false")
	}
	if err := SaveMeta(p, Meta{ConsumedIn: 10, ConsumedOut: 5, LastRealUnix: 42}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadMeta(p)
	if err != nil || !ok || got.ConsumedIn != 10 || got.ConsumedOut != 5 || got.LastRealUnix != 42 {
		t.Fatalf("meta round-trip: %+v ok=%v err=%v", got, ok, err)
	}
}
