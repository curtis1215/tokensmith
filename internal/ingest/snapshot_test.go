package ingest

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"tokensmith/internal/model"
)

func TestGrokSnapshotSourceSumsSignalsAndSkipsMalformedFiles(t *testing.T) {
	home := t.TempDir()
	writeSignal := func(rel, body string) {
		path := filepath.Join(home, "sessions", rel, "signals.json")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeSignal("project-a/session-1", `{"totalTokensBeforeCompaction":100,"contextTokensUsed":20}`)
	writeSignal("project-b/session-2", `{"totalTokensBeforeCompaction":50,"contextTokensUsed":5}`)
	writeSignal("project-c/broken", `{not-json`)

	source := NewGrokSnapshotSource(home)
	if source.Source() != "grok" {
		t.Fatalf("Source = %q, want grok", source.Source())
	}
	got, present, err := source.Totals()
	if err != nil {
		t.Fatal(err)
	}
	if !present {
		t.Fatal("existing Grok sessions reported absent")
	}
	if got != (model.SourceTotals{In: 175}) {
		t.Fatalf("Grok totals = %+v, want In=175", got)
	}

	// Re-reading an unchanged tree must remain a cumulative snapshot, not add
	// another copy of the same sessions.
	got, present, err = source.Totals()
	if err != nil || !present || got != (model.SourceTotals{In: 175}) {
		t.Fatalf("unchanged Grok totals = %+v present=%v err=%v, want In=175", got, present, err)
	}
}

func TestGrokSnapshotSourceMissingHomeIsAbsent(t *testing.T) {
	source := NewGrokSnapshotSource(filepath.Join(t.TempDir(), "missing"))
	got, present, err := source.Totals()
	if err != nil || present || got != (model.SourceTotals{}) {
		t.Fatalf("missing Grok home = %+v present=%v err=%v, want absent", got, present, err)
	}
}

func TestGrokSnapshotSourceDoesNotTreatDisappearanceAsZero(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "sessions", "project", "session", "signals.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"contextTokensUsed":42}`), 0o644); err != nil {
		t.Fatal(err)
	}
	source := NewGrokSnapshotSource(home)
	if _, present, err := source.Totals(); err != nil || !present {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(home, "sessions"), filepath.Join(home, "sessions-away")); err != nil {
		t.Fatal(err)
	}
	got, present, err := source.Totals()
	if err != nil || present || got != (model.SourceTotals{}) {
		t.Fatalf("disappeared Grok source = %+v present=%v err=%v, want absent", got, present, err)
	}
}

func TestGrokSnapshotSourceKeepsCachedTotalDuringMalformedRewrite(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "sessions", "project", "session", "signals.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"contextTokensUsed":42}`), 0o644); err != nil {
		t.Fatal(err)
	}
	source := NewGrokSnapshotSource(home)
	if got, present, err := source.Totals(); err != nil || !present || got.In != 42 {
		t.Fatalf("initial total = %+v present=%v err=%v", got, present, err)
	}
	if err := os.WriteFile(path, []byte(`{temporarily-incomplete`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, present, err := source.Totals(); err != nil || !present || got.In != 42 {
		t.Fatalf("malformed rewrite dropped cached total: %+v present=%v err=%v", got, present, err)
	}
}

func TestOpenCodeSnapshotSourceReadsCompletedAssistantTokens(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "opencode.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE message (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		time_created INTEGER NOT NULL,
		time_updated INTEGER NOT NULL,
		data TEXT NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	rows := []struct {
		id   string
		data string
	}{
		{"assistant-xai", `{"role":"assistant","providerID":"xai","time":{"completed":2},"tokens":{"input":10,"output":5,"reasoning":7,"cache":{"read":100,"write":20}}}`},
		{"assistant-anthropic", `{"role":"assistant","providerID":"anthropic","time":{"completed":3},"tokens":{"input":30,"output":9}}`},
		{"assistant-incomplete", `{"role":"assistant","providerID":"openai","time":{"created":4},"tokens":{"input":999,"output":999}}`},
		{"user", `{"role":"user","tokens":{"input":888,"output":888}}`},
	}
	for i, row := range rows {
		if _, err := db.Exec(
			`INSERT INTO message(id, session_id, time_created, time_updated, data) VALUES(?, 's', ?, ?, ?)`,
			row.id, i+1, i+1, row.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	source := NewOpenCodeSnapshotSource(dbPath)
	if source.Source() != "opencode" {
		t.Fatalf("Source = %q, want opencode", source.Source())
	}
	got, present, err := source.Totals()
	if err != nil {
		t.Fatal(err)
	}
	if !present {
		t.Fatal("existing OpenCode database reported absent")
	}
	if got != (model.SourceTotals{In: 40, Out: 14}) {
		t.Fatalf("OpenCode totals = %+v, want 40/14", got)
	}
}

func TestOpenCodeSnapshotSourceReadsActiveWALReadOnly(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "opencode.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE message (data TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO message(data) VALUES
		('{"role":"assistant","time":{"completed":1},"tokens":{"input":12,"output":7}}')`); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dbPath + "-wal"); err != nil {
		t.Fatalf("test fixture has no active WAL: %v", err)
	}

	source := NewOpenCodeSnapshotSource(dbPath)
	got, present, err := source.Totals()
	if err != nil || !present || got != (model.SourceTotals{In: 12, Out: 7}) {
		t.Fatalf("active WAL totals = %+v present=%v err=%v, want 12/7", got, present, err)
	}
}

func TestOpenCodeSnapshotSourceMissingDatabaseIsAbsent(t *testing.T) {
	source := NewOpenCodeSnapshotSource(filepath.Join(t.TempDir(), "missing.db"))
	got, present, err := source.Totals()
	if err != nil || present || got != (model.SourceTotals{}) {
		t.Fatalf("missing OpenCode DB = %+v present=%v err=%v, want absent", got, present, err)
	}
}

func TestOpenCodeSnapshotSourceDoesNotTreatDisappearanceAsZero(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "opencode.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE message (data TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	source := NewOpenCodeSnapshotSource(dbPath)
	if _, present, err := source.Totals(); err != nil || !present {
		t.Fatal(err)
	}
	if err := os.Remove(dbPath); err != nil {
		t.Fatal(err)
	}
	got, present, err := source.Totals()
	if err != nil || present || got != (model.SourceTotals{}) {
		t.Fatalf("disappeared OpenCode DB = %+v present=%v err=%v, want absent", got, present, err)
	}
}
