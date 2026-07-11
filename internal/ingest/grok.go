package ingest

import (
	"encoding/json"
	"fmt"
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
	seen  bool
}

func NewGrokSnapshotSource(grokHome string) *GrokSnapshotSource {
	return &GrokSnapshotSource{
		root:  filepath.Join(grokHome, "sessions"),
		cache: make(map[string]grokSignalCache),
	}
}

func (*GrokSnapshotSource) Source() string { return "grok" }

func (s *GrokSnapshotSource) Totals() (model.SourceTotals, error) {
	if _, err := os.Stat(s.root); err != nil {
		if os.IsNotExist(err) && !s.seen {
			return model.SourceTotals{}, nil
		}
		return model.SourceTotals{}, fmt.Errorf("Grok sessions unavailable: %w", err)
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
			return nil
		}
		var signal struct {
			BeforeCompaction int `json:"totalTokensBeforeCompaction"`
			ContextUsed      int `json:"contextTokensUsed"`
		}
		if err := json.Unmarshal(data, &signal); err != nil {
			return nil
		}
		tokens := max(0, signal.BeforeCompaction) + max(0, signal.ContextUsed)
		cached := grokSignalCache{modTime: info.ModTime().UnixNano(), size: info.Size(), tokens: tokens}
		next[path] = cached
		total += tokens
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return model.SourceTotals{}, err
	}
	s.cache = next
	s.seen = true
	return model.SourceTotals{In: total}, nil
}
