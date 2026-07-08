package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Meta is TUI-side runtime persistence kept beside the save: how much of the
// harvest ledger has been consumed, and the wall-clock time of the last play
// (used to settle offline progress). The sim itself never sees wall-clock.
type Meta struct {
	ConsumedIn   int   `json:"consumedIn"`
	ConsumedOut  int   `json:"consumedOut"`
	LastRealUnix int64 `json:"lastRealUnix"`
}

// DefaultMetaPath is the standard meta-file location.
func DefaultMetaPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "tokensmith", "meta.json")
}

// SaveMeta writes the meta atomically (temp file + rename).
func SaveMeta(path string, m Meta) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadMeta reads the meta. ok is false when the file does not exist yet.
func LoadMeta(path string) (Meta, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Meta{}, false, nil
	}
	if err != nil {
		return Meta{}, false, err
	}
	var m Meta
	if err := json.Unmarshal(data, &m); err != nil {
		return Meta{}, false, err
	}
	return m, true, nil
}
