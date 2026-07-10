package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"tokensmith/internal/model"
)

// Meta is TUI-side runtime persistence kept beside the save: how much of the
// harvest ledger has been consumed (per source), the coding-streak counter,
// and the wall-clock time of the last play (used to settle offline
// progress). The sim itself never sees wall-clock.
type Meta struct {
	ConsumedSources map[string]model.SourceTotals `json:"consumedSources"`
	LastRealUnix    int64                         `json:"lastRealUnix"`
	// LastActiveDate is "YYYY-MM-DD" in local time — the last calendar day any
	// tokens were harvested. StreakDays counts consecutive such days; a
	// skipped day resets it to 1 on the next active day.
	LastActiveDate string `json:"lastActiveDate"`
	StreakDays     int    `json:"streakDays"`
	// LastCampaignUnix is the wall-clock time of the last completed board
	// cycle (or the session that armed the clock). Used only by the TUI to
	// compute catch-up cycles; the pure sim never sees this field.
	LastCampaignUnix int64 `json:"lastCampaignUnix"`
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
