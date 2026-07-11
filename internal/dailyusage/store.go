package dailyusage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Sink is the write side of daily usage persistence.
type Sink interface {
	Add(Batch) error
}

// Reader is the read side of daily usage persistence.
type Reader interface {
	Load() (Document, bool, error)
}

// Store is a locked, atomic JSON store for daily usage documents.
type Store struct {
	path string
}

// New returns a Store writing to path. Parent directories are created on first Add.
func New(path string) *Store {
	return &Store{path: path}
}

// Load reads the document without mutating a corrupt file.
// ok is false when the file does not exist.
func (s *Store) Load() (Document, bool, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return Document{}, false, nil
	}
	if err != nil {
		return Document{}, false, err
	}
	doc, err := decodeDocument(data)
	if err != nil {
		return Document{}, false, err
	}
	return doc, true, nil
}

// Add applies batch under an advisory lock with atomic rewrite.
// Corrupt or unsupported files are renamed to daily-usage.json.corrupt-<unix>
// and replaced with a fresh v1 document before applying the batch.
func (s *Store) Add(batch Batch) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("dailyusage: mkdir: %w", err)
	}
	release, err := acquireFileLock(s.path+".lock", 250*time.Millisecond)
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	doc, err := s.loadOrRecoverLocked()
	if err != nil {
		return err
	}
	Apply(&doc, batch)
	return s.writeAtomicLocked(doc)
}

// loadOrRecoverLocked must be called while the lock is held.
// On corrupt/unsupported data it renames the file aside and returns a fresh doc.
func (s *Store) loadOrRecoverLocked() (Document, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return Document{SchemaVersion: SchemaVersion, Days: map[string]map[string]SourceUsage{}}, nil
	}
	if err != nil {
		return Document{}, fmt.Errorf("dailyusage: read: %w", err)
	}
	doc, err := decodeDocument(data)
	if err != nil {
		backup := fmt.Sprintf("%s.corrupt-%d", s.path, time.Now().Unix())
		if rerr := os.Rename(s.path, backup); rerr != nil {
			return Document{}, fmt.Errorf("dailyusage: backup corrupt: %w", rerr)
		}
		return Document{SchemaVersion: SchemaVersion, Days: map[string]map[string]SourceUsage{}}, nil
	}
	return doc, nil
}

func (s *Store) writeAtomicLocked(doc Document) error {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	// Same-directory temp for atomic rename.
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".daily-usage-*.tmp")
	if err != nil {
		return fmt.Errorf("dailyusage: temp: %w", err)
	}
	tmpName := tmp.Name()
	// Ensure cleanup on failure paths.
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("dailyusage: chmod temp: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("dailyusage: write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("dailyusage: sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("dailyusage: close temp: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("dailyusage: rename: %w", err)
	}
	success = true
	// Best-effort enforce data-file mode after rename.
	_ = os.Chmod(s.path, 0o600)
	return nil
}

func decodeDocument(data []byte) (Document, error) {
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return Document{}, err
	}
	if doc.SchemaVersion != SchemaVersion {
		// Treat 0 (missing) as corrupt/unsupported unless the rest looks empty-new;
		// plan: unsupported schema is backed up. schemaVersion must be 1.
		if doc.SchemaVersion != 0 {
			return Document{}, fmt.Errorf("dailyusage: unsupported schemaVersion %d", doc.SchemaVersion)
		}
		// Missing schemaVersion with days data: still unsupported for safety.
		if len(doc.Days) > 0 || doc.UpdatedAt != 0 {
			return Document{}, fmt.Errorf("dailyusage: missing schemaVersion")
		}
		// Completely empty object: treat as fresh.
		doc.SchemaVersion = SchemaVersion
	}
	if doc.Days == nil {
		doc.Days = map[string]map[string]SourceUsage{}
	}
	return doc, nil
}
