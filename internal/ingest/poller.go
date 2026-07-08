package ingest

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"tokensmith/internal/model"
)

type parser func([]byte) (model.TokenEvent, bool)

type dirSource struct {
	root  string
	parse parser
}

// Poller tails Claude Code and Codex JSONL logs, tracking a per-file byte
// cursor so each token event is emitted exactly once.
type Poller struct {
	sources []dirSource
	offsets map[string]int64
	seen    map[string]bool // dedup keys already emitted (e.g. Claude message ids)
}

// NewPoller builds a poller over explicit directories (injectable for tests).
func NewPoller(claudeDir, codexDir string) *Poller {
	return &Poller{
		sources: []dirSource{
			{claudeDir, ParseClaudeCodeLine},
			{codexDir, ParseCodexLine},
		},
		offsets: map[string]int64{},
		seen:    map[string]bool{},
	}
}

// NewDefaultPoller uses the standard log locations under the home directory.
func NewDefaultPoller() *Poller {
	home, _ := os.UserHomeDir()
	return NewPoller(
		filepath.Join(home, ".claude", "projects"),
		filepath.Join(home, ".codex", "sessions"),
	)
}

// Poll returns token events appended to any tracked log since the last call.
func (p *Poller) Poll() []model.TokenEvent {
	var events []model.TokenEvent
	for _, src := range p.sources {
		_ = filepath.WalkDir(src.root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
				return nil
			}
			events = append(events, p.tailFile(path, src.parse)...)
			return nil
		})
	}
	return events
}

// Prime sets each tracked log's cursor to its current end (via Stat, no content
// read), so a subsequent Poll only reports usage appended after priming — the
// game harvests new coding activity, not the entire history.
func (p *Poller) Prime() {
	for _, src := range p.sources {
		_ = filepath.WalkDir(src.root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
				return nil
			}
			if fi, statErr := os.Stat(path); statErr == nil {
				p.offsets[path] = fi.Size()
			}
			return nil
		})
	}
}

func (p *Poller) tailFile(path string, parse parser) []model.TokenEvent {
	fi, err := os.Stat(path)
	if err != nil {
		return nil
	}
	off := p.offsets[path]
	if fi.Size() < off { // rotated / truncated
		off = 0
	}
	if fi.Size() <= off {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return nil
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil
	}
	lastNL := bytes.LastIndexByte(data, '\n')
	if lastNL < 0 {
		return nil // only a partial line so far
	}
	var events []model.TokenEvent
	for _, line := range bytes.Split(data[:lastNL+1], []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		if ev, ok := parse(line); ok {
			// Claude writes one API response across several rows with identical
			// usage; count each response once by its dedup key.
			if ev.ID != "" {
				if p.seen[ev.ID] {
					continue
				}
				p.seen[ev.ID] = true
			}
			events = append(events, ev)
		}
	}
	p.offsets[path] = off + int64(lastNL) + 1
	return events
}
