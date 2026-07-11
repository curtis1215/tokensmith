package ingest

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"

	"tokensmith/internal/model"
)

type grokSignalCache struct {
	modTime int64
	size    int64
	tokens  int
}

// GrokSnapshotSource aggregates standalone Grok CLI session signals. Grok's
// signals expose a cumulative context total but not a reliable input/output
// split, so the estimate is reported entirely as input tokens.
type GrokSnapshotSource struct {
	root  string
	cache map[string]grokSignalCache
}

func NewGrokSnapshotSource(grokHome string) *GrokSnapshotSource {
	return &GrokSnapshotSource{
		root:  filepath.Join(grokHome, "sessions"),
		cache: make(map[string]grokSignalCache),
	}
}

func (*GrokSnapshotSource) Source() string { return "grok" }

func (s *GrokSnapshotSource) Totals() (model.SourceTotals, bool, error) {
	if _, err := os.Stat(s.root); err != nil {
		if os.IsNotExist(err) {
			return model.SourceTotals{}, false, nil
		}
		return model.SourceTotals{}, false, err
	}
	next := make(map[string]grokSignalCache, len(s.cache))
	total := 0
	err := filepath.WalkDir(s.root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if entry.IsDir() || entry.Name() != "signals.json" {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		if cached, ok := s.cache[path]; ok && cached.modTime == info.ModTime().UnixNano() && cached.size == info.Size() {
			next[path] = cached
			total += cached.tokens
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			if cached, ok := s.cache[path]; ok {
				next[path] = cached
				total += cached.tokens
			}
			return nil
		}
		var signal struct {
			BeforeCompaction int `json:"totalTokensBeforeCompaction"`
			ContextUsed      int `json:"contextTokensUsed"`
		}
		if err := json.Unmarshal(data, &signal); err != nil {
			if cached, ok := s.cache[path]; ok {
				next[path] = cached
				total += cached.tokens
			}
			return nil
		}
		tokens := max(0, signal.BeforeCompaction) + max(0, signal.ContextUsed)
		cached := grokSignalCache{modTime: info.ModTime().UnixNano(), size: info.Size(), tokens: tokens}
		next[path] = cached
		total += tokens
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return model.SourceTotals{}, false, err
	}
	s.cache = next
	return model.SourceTotals{In: total}, true, nil
}
