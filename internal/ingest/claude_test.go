package ingest

import "testing"

func TestParseClaudeCodeLine(t *testing.T) {
	line := []byte(`{"type":"assistant","timestamp":"2026-07-07T10:59:19.656Z","message":{"usage":{"input_tokens":11381,"output_tokens":154,"cache_read_input_tokens":18556}}}`)
	ev, ok := ParseClaudeCodeLine(line)
	if !ok {
		t.Fatalf("expected usage event")
	}
	if ev.Source != "claude-code" || ev.InputTokens != 11381 || ev.OutputTokens != 154 {
		t.Fatalf("event wrong: %+v", ev)
	}
	if ev.Timestamp.IsZero() {
		t.Errorf("timestamp not parsed")
	}
}

func TestParseClaudeCodeLineNonUsage(t *testing.T) {
	for _, line := range [][]byte{
		[]byte(`{"type":"user","timestamp":"2026-07-07T10:59:19Z","message":{}}`),
		[]byte(`{"type":"assistant","message":{}}`),
		[]byte(`not json`),
	} {
		if _, ok := ParseClaudeCodeLine(line); ok {
			t.Errorf("should not parse: %s", line)
		}
	}
}
