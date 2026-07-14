package metrics

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Store is an atomic JSON store for metrics-history documents.
type Store struct {
	path string
}

// New returns a Store writing to path. Parent directories are created on Save.
func New(path string) *Store {
	return &Store{path: path}
}

// Load reads the document. Missing file → (EmptyDocument(), false, nil).
// Corrupt or unsupported schema → rename to path.corrupt-<unix>,
// return (EmptyDocument(), true, nil) so the TUI never blocks.
func (s *Store) Load() (Document, bool, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return EmptyDocument(), false, nil
	}
	if err != nil {
		return EmptyDocument(), false, err
	}
	doc, err := decodeDocument(data)
	if err != nil {
		backup := fmt.Sprintf("%s.corrupt-%d", s.path, time.Now().Unix())
		if rerr := os.Rename(s.path, backup); rerr != nil {
			return EmptyDocument(), false, fmt.Errorf("metrics: backup corrupt: %w", rerr)
		}
		return EmptyDocument(), true, nil
	}
	return doc, true, nil
}

// Save prunes the document, ensures parent dir, and atomically writes JSON (mode 0o600).
func (s *Store) Save(doc Document) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("metrics: mkdir: %w", err)
	}
	Prune(&doc)
	if doc.SchemaVersion == 0 {
		doc.SchemaVersion = SchemaVersion
	}
	if doc.Days == nil {
		doc.Days = map[string]DayPoint{}
	}
	return s.writeAtomic(doc)
}

func (s *Store) writeAtomic(doc Document) error {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	// Same-directory temp for atomic rename.
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".metrics-history-*.tmp")
	if err != nil {
		return fmt.Errorf("metrics: temp: %w", err)
	}
	tmpName := tmp.Name()
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("metrics: chmod temp: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("metrics: write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("metrics: sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("metrics: close temp: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("metrics: rename: %w", err)
	}
	success = true
	_ = os.Chmod(s.path, 0o600)
	return nil
}

func decodeDocument(data []byte) (Document, error) {
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return Document{}, err
	}
	if doc.SchemaVersion != SchemaVersion {
		// Unsupported non-zero schema is corrupt.
		if doc.SchemaVersion != 0 {
			return Document{}, fmt.Errorf("metrics: unsupported schemaVersion %d", doc.SchemaVersion)
		}
		// Missing schemaVersion with any data: treat as corrupt.
		if len(doc.Days) > 0 || doc.UpdatedAt != 0 {
			return Document{}, fmt.Errorf("metrics: missing schemaVersion")
		}
		// Completely empty object: treat as fresh.
		doc.SchemaVersion = SchemaVersion
	}
	if doc.Days == nil {
		doc.Days = map[string]DayPoint{}
	}
	return doc, nil
}
