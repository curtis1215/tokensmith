// Package store persists the game state to a JSON file.
package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

// CurrentSchemaVersion is written on every Save. Load accepts this envelope
// or a legacy bare GameState (no schemaVersion), migrating v0 → current.
// v2: individual employees + office + talent market (replaces aggregate staff/stars).
const CurrentSchemaVersion = 2

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

// Load reads the state from path using balance.Default().
// Returns ok=false if the file does not exist.
func Load(path string) (model.GameState, bool, error) {
	return LoadWithConfig(path, balance.Default())
}

// LoadWithConfig is the explicit-config load path (tests / custom balance).
// Legacy bare GameState (schema 0) is backed up once, migrated, validated, and
// rewritten as a versioned envelope. Schema 1 → 2 rewrites after employee-office
// migration (cash compensation + market seed). On any failure the original
// bytes remain.
func LoadWithConfig(path string, b balance.Config) (model.GameState, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return model.GameState{}, false, nil
	}
	if err != nil {
		return model.GameState{}, false, err
	}
	legacyBare := !hasSchemaVersion(data)
	schemaVer, s, err := decodeSaveBytes(data)
	if err != nil {
		return model.GameState{}, false, err
	}
	leg := probeLegacyStaff(data)

	// Soft-repair zero-value progression on older fixtures / mid-feature states.
	if s.Progression.MaxUnlockedGen < 1 {
		s.Progression.MaxUnlockedGen = 1
	}

	if legacyBare {
		// Schema 0 → progression migrate + employee office, then rewrite envelope.
		if err := backupV0(path, data); err != nil {
			return model.GameState{}, false, err
		}
		migrated, err := migrateV0(s, b)
		if err != nil {
			return model.GameState{}, false, err
		}
		migrated = migrateToEmployeeOffice(migrated, b, leg)
		migrated = sim.ClampIndustryToPlayerCap(migrated, b)
		if err := validateState(&migrated, b); err != nil {
			return model.GameState{}, false, err
		}
		if err := Save(path, migrated); err != nil {
			return model.GameState{}, false, err
		}
		return migrated, true, nil
	}

	if schemaVer < CurrentSchemaVersion {
		// Schema 1 → 2: employee office system (probe compensation, rewrite).
		migrated := migrateToEmployeeOffice(s, b, leg)
		migrated = sim.ClampIndustryToPlayerCap(migrated, b)
		if err := validateState(&migrated, b); err != nil {
			return model.GameState{}, false, err
		}
		if err := Save(path, migrated); err != nil {
			return model.GameState{}, false, err
		}
		return migrated, true, nil
	}

	// Current schema: soft-repair office/market defaults + industry player-cap.
	s = ensureEmployeeOfficeDefaults(s, b)
	s = sim.ClampIndustryToPlayerCap(s, b)
	if err := validateState(&s, b); err != nil {
		return model.GameState{}, false, err
	}
	return s, true, nil
}

func hasSchemaVersion(data []byte) bool {
	var probe struct {
		SchemaVersion *int `json:"schemaVersion"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return false
	}
	return probe.SchemaVersion != nil
}

// decodeSaveBytes probes raw JSON for a top-level schemaVersion key.
// Future schema versions (schemaVersion > CurrentSchemaVersion) are rejected so
// older builds never silently drop unknown fields and autosave a downgraded file.
// Bare GameState documents report schemaVersion 0.
func decodeSaveBytes(data []byte) (schemaVersion int, s model.GameState, err error) {
	// Reject empty / non-object payloads early so corrupt files stay put.
	trim := bytes.TrimSpace(data)
	if len(trim) == 0 || trim[0] != '{' {
		return 0, model.GameState{}, errors.New("store: invalid save JSON")
	}

	// Probe without requiring a full envelope decode first.
	var probe struct {
		SchemaVersion *int `json:"schemaVersion"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return 0, model.GameState{}, err
	}
	if probe.SchemaVersion != nil {
		if *probe.SchemaVersion > CurrentSchemaVersion {
			return 0, model.GameState{}, errors.New("store: unsupported schema version")
		}
		var env SaveFile
		if err := json.Unmarshal(data, &env); err != nil {
			return 0, model.GameState{}, err
		}
		return *probe.SchemaVersion, env.State, nil
	}
	// Legacy: bare GameState document.
	if err := json.Unmarshal(data, &s); err != nil {
		return 0, model.GameState{}, err
	}
	return 0, s, nil
}
