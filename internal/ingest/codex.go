package ingest

import (
	"encoding/json"
	"time"

	"tokensmith/internal/model"
)

// ParseCodexLine parses one Codex rollout JSONL line into a TokenEvent.
// ok is false unless payload.type == "token_count" with nonzero usage.
func ParseCodexLine(line []byte) (model.TokenEvent, bool) {
	var rec struct {
		Timestamp string `json:"timestamp"`
		Payload   struct {
			Type string `json:"type"`
			Info struct {
				Last struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
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
	if u.InputTokens == 0 && u.OutputTokens == 0 {
		return model.TokenEvent{}, false
	}
	ts, _ := time.Parse(time.RFC3339, rec.Timestamp)
	return model.TokenEvent{
		Source:       "codex",
		Timestamp:    ts,
		InputTokens:  u.InputTokens,
		OutputTokens: u.OutputTokens,
	}, true
}
