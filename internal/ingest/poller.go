package ingest

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"tokensmith/internal/model"
)

type parser func([]byte) (model.TokenEvent, bool)

type dirSource struct {
	root  string
	parse parser
}

// fileCursor tracks how far a log file has been consumed, keyed to the file's
// identity so a rotated file (new inode at the same path) restarts from 0 even
// if the replacement has already grown past the old byte offset.
type fileCursor struct {
	inode  uint64
	offset int64
}

type fileIdentity struct {
	device uint64
	inode  uint64
}

// inodeOf returns the file's inode, or 0 on platforms without Unix stat.
func inodeOf(fi os.FileInfo) uint64 {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return st.Ino
	}
	return 0
}

func identityOf(fi os.FileInfo) fileIdentity {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return fileIdentity{device: uint64(st.Dev), inode: st.Ino}
	}
	return fileIdentity{}
}

// Poller tails Claude Code and Codex JSONL logs, tracking a per-file cursor
// (inode + byte offset) so each token event is emitted exactly once and file
// rotation is detected.
type Poller struct {
	sources []dirSource
	cursors map[string]fileCursor
	seen    map[string]bool // dedup keys already emitted (e.g. Claude message ids)
}

// NewPoller builds a poller over explicit directories (injectable for tests).
func NewPoller(claudeDir, codexDir string) *Poller {
	return NewPollerWithRoots([]string{claudeDir}, []string{codexDir})
}

// NewPollerWithRoots builds a poller over multiple Claude Code and Codex log
// roots. Duplicate roots are collapsed so a session cannot be harvested twice
// merely because two discovery mechanisms resolved to the same directory.
func NewPollerWithRoots(claudeDirs, codexDirs []string) *Poller {
	sources := make([]dirSource, 0, len(claudeDirs)+len(codexDirs))
	for _, root := range uniqueRoots(claudeDirs) {
		sources = append(sources, dirSource{root: root, parse: ParseClaudeCodeLine})
	}
	for _, root := range uniqueRoots(codexDirs) {
		sources = append(sources, dirSource{root: root, parse: ParseCodexLine})
	}
	return &Poller{
		sources: sources,
		cursors: map[string]fileCursor{},
		seen:    map[string]bool{},
	}
}

func uniqueRoots(roots []string) []string {
	out := make([]string, 0, len(roots))
	seen := make(map[string]bool, len(roots))
	for _, root := range roots {
		if root == "" {
			continue
		}
		root = filepath.Clean(root)
		key := root
		if resolved, err := filepath.EvalSymlinks(root); err == nil {
			key = resolved
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, root)
	}
	return out
}

// NewDefaultPoller uses the standard log locations under the home directory.
func NewDefaultPoller() *Poller {
	home, _ := os.UserHomeDir()
	return NewPollerWithRoots(
		[]string{filepath.Join(home, ".claude", "projects")},
		CodexSessionRoots(home, envMap()),
	)
}

// Poll returns token events appended to any tracked log since the last call.
func (p *Poller) Poll() []model.TokenEvent {
	var events []model.TokenEvent
	visited := make(map[fileIdentity]fileCursor)
	for _, src := range p.sources {
		_ = filepath.WalkDir(src.root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
				return nil
			}
			if info, infoErr := d.Info(); infoErr == nil {
				identity := identityOf(info)
				if identity.inode != 0 {
					if cursor, duplicate := visited[identity]; duplicate {
						p.cursors[path] = cursor
						return nil
					}
					events = append(events, p.tailFile(path, src.parse)...)
					visited[identity] = p.cursors[path]
					return nil
				}
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
func (p *Poller) Prime() { p.prime(false) }

// PrimeUnknown primes only files that do not yet have a cursor. Used on daemon
// restart so files whose cursor was dropped from a pruned persisted set (or
// files first seen after restart) resume at EOF instead of being re-read from
// the start (which would double-count their history).
func (p *Poller) PrimeUnknown() { p.prime(true) }

func (p *Poller) prime(onlyUnknown bool) {
	for _, src := range p.sources {
		_ = filepath.WalkDir(src.root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
				return nil
			}
			if onlyUnknown {
				if _, seen := p.cursors[path]; seen {
					return nil
				}
			}
			if fi, statErr := os.Stat(path); statErr == nil {
				p.cursors[path] = fileCursor{inode: inodeOf(fi), offset: fi.Size()}
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
	ino := inodeOf(fi)
	cur := p.cursors[path]
	off := cur.offset
	// A different inode at this path (rotation) or a file now shorter than our
	// cursor (truncation) means the old file is gone — restart from the start.
	if cur.inode != ino || fi.Size() < off {
		off = 0
	}
	if fi.Size() <= off {
		p.cursors[path] = fileCursor{inode: ino, offset: off}
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
	p.cursors[path] = fileCursor{inode: ino, offset: off + int64(lastNL) + 1}
	return events
}
