package dailyusage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"tokensmith/internal/model"
)

func TestStoreLoadMissingReturnsEmpty(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "daily-usage.json"))
	doc, ok, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("missing file should report ok=false")
	}
	if doc.Days != nil && len(doc.Days) != 0 {
		t.Fatalf("want empty doc, got %+v", doc)
	}
}

func TestStoreRoundTripAndPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "daily-usage.json")
	s := New(path)
	if err := s.Add(Batch{
		Day: "2026-07-12", ObservedAt: 100,
		Sources: map[string]model.SourceTotals{"claude-code": {In: 10, Out: 3}},
	}); err != nil {
		t.Fatal(err)
	}
	doc, ok, err := s.Load()
	if err != nil || !ok {
		t.Fatalf("Load: ok=%v err=%v", ok, err)
	}
	got := doc.Days["2026-07-12"]["claude-code"]
	if got.In != 10 || got.Out != 3 || got.LastUpdatedAt != 100 {
		t.Fatalf("round-trip=%+v", got)
	}
	if doc.UpdatedAt != 100 || doc.SchemaVersion != 1 {
		t.Fatalf("doc meta=%+v", doc)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("data perms=%o, want 0600", fi.Mode().Perm())
	}
	lockFI, err := os.Stat(path + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	if lockFI.Mode().Perm() != 0o600 {
		t.Fatalf("lock perms=%o, want 0600", lockFI.Mode().Perm())
	}
}

func TestStoreAccumulatesAcrossAdds(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "daily-usage.json"))
	_ = s.Add(Batch{Day: "2026-07-12", ObservedAt: 1, Sources: map[string]model.SourceTotals{"codex": {In: 5, Out: 1}}})
	_ = s.Add(Batch{Day: "2026-07-12", ObservedAt: 2, Sources: map[string]model.SourceTotals{"codex": {In: 7, Out: 3}}})
	doc, _, _ := s.Load()
	got := doc.Days["2026-07-12"]["codex"]
	if got.In != 12 || got.Out != 4 {
		t.Fatalf("got=%+v", got)
	}
}

func TestStoreConcurrentAddsLoseNoUpdates(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "daily-usage.json"))
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.Add(Batch{Day: "2026-07-12", ObservedAt: 100,
				Sources: map[string]model.SourceTotals{"codex": {In: 10, Out: 2}}}); err != nil {
				t.Errorf("Add: %v", err)
			}
		}()
	}
	wg.Wait()
	doc, _, _ := s.Load()
	got := doc.Days["2026-07-12"]["codex"]
	if got.In != 200 || got.Out != 40 {
		t.Fatalf("lost update=%+v", got)
	}
}

func TestStoreCorruptFilePreservedAndRebuilt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "daily-usage.json")
	if err := os.WriteFile(path, []byte("not-json{{{"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := New(path)
	if err := s.Add(Batch{
		Day: "2026-07-12", ObservedAt: 50,
		Sources: map[string]model.SourceTotals{"grok": {In: 9}},
	}); err != nil {
		t.Fatal(err)
	}
	// Original bytes preserved under corrupt-<unix>
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var backup string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "daily-usage.json.corrupt-") {
			backup = filepath.Join(dir, e.Name())
			break
		}
	}
	if backup == "" {
		t.Fatalf("expected corrupt backup, dir=%v", entries)
	}
	raw, _ := os.ReadFile(backup)
	if string(raw) != "not-json{{{" {
		t.Fatalf("backup contents=%q", raw)
	}
	doc, ok, err := s.Load()
	if err != nil || !ok {
		t.Fatalf("Load after rebuild: ok=%v err=%v", ok, err)
	}
	if doc.Days["2026-07-12"]["grok"].In != 9 {
		t.Fatalf("rebuilt doc=%+v", doc)
	}
}

func TestStoreUnsupportedSchemaVersionBackedUp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "daily-usage.json")
	raw, _ := json.Marshal(map[string]any{"schemaVersion": 99, "days": map[string]any{}})
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	s := New(path)
	if err := s.Add(Batch{
		Day: "2026-07-12", ObservedAt: 1,
		Sources: map[string]model.SourceTotals{"opencode": {In: 1}},
	}); err != nil {
		t.Fatal(err)
	}
	found := false
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), "daily-usage.json.corrupt-") {
			found = true
		}
	}
	if !found {
		t.Fatal("unsupported schema should be backed up as corrupt")
	}
	doc, _, _ := s.Load()
	if doc.SchemaVersion != 1 || doc.Days["2026-07-12"]["opencode"].In != 1 {
		t.Fatalf("rebuilt=%+v", doc)
	}
}

func TestStoreHeldLockTimeoutWithoutMutation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "daily-usage.json")
	// Seed a valid document first.
	s := New(path)
	if err := s.Add(Batch{
		Day: "2026-07-12", ObservedAt: 10,
		Sources: map[string]model.SourceTotals{"codex": {In: 1}},
	}); err != nil {
		t.Fatal(err)
	}
	// Hold the advisory lock.
	release, err := acquireFileLock(path+".lock", 250*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	err = s.Add(Batch{
		Day: "2026-07-12", ObservedAt: 20,
		Sources: map[string]model.SourceTotals{"codex": {In: 99}},
	})
	if err == nil {
		t.Fatal("want lock timeout error")
	}
	if !errors.Is(err, ErrLockTimeout) && !strings.Contains(err.Error(), "lock") {
		t.Fatalf("unexpected err: %v", err)
	}
	doc, _, _ := s.Load()
	if doc.Days["2026-07-12"]["codex"].In != 1 {
		t.Fatalf("mutation under held lock: %+v", doc.Days)
	}
}

func TestStoreLoadDoesNotMutateCorrupt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "daily-usage.json")
	if err := os.WriteFile(path, []byte("{bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := New(path)
	_, ok, err := s.Load()
	if ok || err == nil {
		t.Fatalf("Load corrupt: ok=%v err=%v", ok, err)
	}
	// File still original; no corrupt- rename from Load alone.
	raw, _ := os.ReadFile(path)
	if string(raw) != "{bad" {
		t.Fatalf("Load mutated file: %q", raw)
	}
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		if strings.Contains(e.Name(), "corrupt") {
			t.Fatalf("Load must not create backup: %s", e.Name())
		}
	}
}

func TestStoreRejectsNegativeWithoutDecreasing(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "daily-usage.json"))
	_ = s.Add(Batch{Day: "2026-07-12", ObservedAt: 1, Sources: map[string]model.SourceTotals{"codex": {In: 10, Out: 5}}})
	_ = s.Add(Batch{Day: "2026-07-12", ObservedAt: 2, Sources: map[string]model.SourceTotals{"codex": {In: -100, Out: -50}}})
	doc, _, _ := s.Load()
	got := doc.Days["2026-07-12"]["codex"]
	if got.In != 10 || got.Out != 5 {
		t.Fatalf("totals decreased: %+v", got)
	}
}
