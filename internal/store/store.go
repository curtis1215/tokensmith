// Package store persists the game state to a JSON file.
package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"tokensmith/internal/model"
)

// CurrentSchemaVersion is written on every Save. Load accepts this envelope
// or a legacy bare GameState (no schemaVersion). Migration of legacy fields
// is Task 13.
const CurrentSchemaVersion = 1

// SaveFile is the versioned on-disk envelope.
type SaveFile struct {
	SchemaVersion int             `json:"schemaVersion"`
	State         model.GameState `json:"state"`
}

// DefaultPath is the standard save-file location.
func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "tokensmith", "save.json")
}

// Save writes the state to path atomically (temp file + rename) as a
// versioned SaveFile envelope.
func Save(path string, s model.GameState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	env := SaveFile{SchemaVersion: CurrentSchemaVersion, State: s}
	data, err := json.Marshal(env)
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
// Versioned envelopes (top-level schemaVersion) unwrap State; absent
// schemaVersion means a legacy bare GameState. Corrupt bytes are never
// rewritten — the original file is left untouched and an error is returned.
func Load(path string) (model.GameState, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return model.GameState{}, false, nil
	}
	if err != nil {
		return model.GameState{}, false, err
	}
	s, err := decodeSaveBytes(data)
	if err != nil {
		return model.GameState{}, false, err
	}
	return s, true, nil
}

// decodeSaveBytes probes raw JSON for a top-level schemaVersion key.
func decodeSaveBytes(data []byte) (model.GameState, error) {
	// Reject empty / non-object payloads early so corrupt files stay put.
	trim := bytes.TrimSpace(data)
	if len(trim) == 0 || trim[0] != '{' {
		return model.GameState{}, errors.New("store: invalid save JSON")
	}

	// Probe without requiring a full envelope decode first.
	var probe struct {
		SchemaVersion *int `json:"schemaVersion"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return model.GameState{}, err
	}
	if probe.SchemaVersion != nil {
		var env SaveFile
		if err := json.Unmarshal(data, &env); err != nil {
			return model.GameState{}, err
		}
		return env.State, nil
	}
	// Legacy: bare GameState document.
	var s model.GameState
	if err := json.Unmarshal(data, &s); err != nil {
		return model.GameState{}, err
	}
	return s, nil
}
