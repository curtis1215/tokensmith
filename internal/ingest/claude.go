// Package ingest reads real AI-coding-tool token usage from local logs.
package ingest

import (
	"encoding/json"
	"time"

	"tokensmith/internal/model"
)

// ParseClaudeCodeLine parses one Claude Code JSONL line into a TokenEvent.
// ok is false for non-assistant lines, lines without usage, or bad JSON.
func ParseClaudeCodeLine(line []byte) (model.TokenEvent, bool) {
	var rec struct {
		Type      string `json:"type"`
		Timestamp string `json:"timestamp"`
		Message   struct {
			ID    string `json:"id"`
			Usage *struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		} `json:"message"`
	}
	if err := json.Unmarshal(line, &rec); err != nil {
		return model.TokenEvent{}, false
	}
	if rec.Type != "assistant" || rec.Message.Usage == nil {
		return model.TokenEvent{}, false
	}
	ts, _ := time.Parse(time.RFC3339, rec.Timestamp)
	return model.TokenEvent{
		Source:       "claude-code",
		Timestamp:    ts,
		InputTokens:  rec.Message.Usage.InputTokens,
		OutputTokens: rec.Message.Usage.OutputTokens,
		ID:           rec.Message.ID, // Claude writes one response across several rows; dedup by id
	}, true
}
