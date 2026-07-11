package ingest

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"tokensmith/internal/model"
)

type fileFingerprint struct {
	exists  bool
	size    int64
	modTime int64
}

// OpenCodeSnapshotSource reads completed assistant token usage from OpenCode's
// local SQLite database. Provider/model identity inside OpenCode is ignored for
// accounting: the producing tool is always "opencode".
type OpenCodeSnapshotSource struct {
	path       string
	lastDB     fileFingerprint
	lastWAL    fileFingerprint
	lastTotals model.SourceTotals
	loaded     bool
}

func NewOpenCodeSnapshotSource(path string) *OpenCodeSnapshotSource {
	return &OpenCodeSnapshotSource{path: path}
}

func (*OpenCodeSnapshotSource) Source() string { return "opencode" }

func (s *OpenCodeSnapshotSource) Totals() (model.SourceTotals, error) {
	dbFingerprint, err := fingerprint(s.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			if s.loaded && s.lastDB.exists {
				return model.SourceTotals{}, fmt.Errorf("OpenCode database disappeared: %w", err)
			}
			s.loaded = true
			s.lastDB = fileFingerprint{}
			s.lastWAL = fileFingerprint{}
			s.lastTotals = model.SourceTotals{}
			return model.SourceTotals{}, nil
		}
		return model.SourceTotals{}, err
	}
	walFingerprint, err := fingerprint(s.path + "-wal")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return model.SourceTotals{}, err
	}
	if !dbFingerprint.exists {
		return model.SourceTotals{}, nil
	}
	if s.loaded && dbFingerprint == s.lastDB && walFingerprint == s.lastWAL {
		return s.lastTotals, nil
	}

	dsn := (&url.URL{Scheme: "file", Path: filepath.Clean(s.path), RawQuery: "mode=ro"}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return model.SourceTotals{}, err
	}
	defer db.Close()
	if _, err := db.Exec(`PRAGMA busy_timeout = 250`); err != nil {
		return model.SourceTotals{}, err
	}
	const query = `
		SELECT
			COALESCE(SUM(CAST(json_extract(data, '$.tokens.input') AS INTEGER)), 0),
			COALESCE(SUM(CAST(json_extract(data, '$.tokens.output') AS INTEGER)), 0)
		FROM message
		WHERE json_valid(data)
		  AND json_extract(data, '$.role') = 'assistant'
		  AND json_type(data, '$.time.completed') IN ('integer', 'real')
		  AND json_type(data, '$.tokens') = 'object'
	`
	var in, out int64
	if err := db.QueryRow(query).Scan(&in, &out); err != nil {
		return model.SourceTotals{}, err
	}
	totals := model.SourceTotals{In: int(max(int64(0), in)), Out: int(max(int64(0), out))}
	s.lastDB = dbFingerprint
	s.lastWAL = walFingerprint
	s.lastTotals = totals
	s.loaded = true
	return totals, nil
}

func fingerprint(path string) (fileFingerprint, error) {
	info, err := os.Stat(path)
	if err != nil {
		return fileFingerprint{}, err
	}
	return fileFingerprint{exists: true, size: info.Size(), modTime: info.ModTime().UnixNano()}, nil
}
