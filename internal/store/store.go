// Package store persists the game state to a JSON file.
package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"tokensmith/internal/model"
)

// DefaultPath is the standard save-file location.
func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "tokensmith", "save.json")
}

// Save writes the state to path atomically (temp file + rename).
func Save(path string, s model.GameState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Load reads the state from path. Returns ok=false if the file does not exist.
func Load(path string) (model.GameState, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return model.GameState{}, false, nil
	}
	if err != nil {
		return model.GameState{}, false, err
	}
	var s model.GameState
	if err := json.Unmarshal(data, &s); err != nil {
		return model.GameState{}, false, err
	}
	return s, true, nil
}
