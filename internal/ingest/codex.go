package ingest

import (
	"encoding/json"
	"time"

	"tokensmith/internal/model"
)

// ParseCodexLine parses one Codex rollout JSONL line into a TokenEvent.
// ok is false unless payload.type == "token_count" with nonzero usage.
//
// last_token_usage.input_tokens includes cached_input_tokens (prompt-cache
// hits), unlike Claude Code's usage.input_tokens which reports only fresh,
// non-cached tokens. Subtracting the cache hit here keeps the two sources'
// R&D-per-token accounting comparable — see internal/sim.TokenRawRnD.
func ParseCodexLine(line []byte) (model.TokenEvent, bool) {
	var rec struct {
		Timestamp string `json:"timestamp"`
		Payload   struct {
			Type string `json:"type"`
			Info struct {
				Last struct {
					InputTokens       int `json:"input_tokens"`
					CachedInputTokens int `json:"cached_input_tokens"`
					OutputTokens      int `json:"output_tokens"`
				} `json:"last_token_usage"`
			} `json:"info"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(line, &rec); err != nil {
		return model.TokenEvent{}, false
	}
	if rec.Payload.Type != "token_count" {
		return model.TokenEvent{}, false
	}
	u := rec.Payload.Info.Last
	freshInput := u.InputTokens - u.CachedInputTokens
	if freshInput < 0 {
		freshInput = 0
	}
	if freshInput == 0 && u.OutputTokens == 0 {
		return model.TokenEvent{}, false
	}
	ts, _ := time.Parse(time.RFC3339, rec.Timestamp)
	return model.TokenEvent{
		Source:       "codex",
		Timestamp:    ts,
		InputTokens:  freshInput,
		OutputTokens: u.OutputTokens,
	}, true
}
