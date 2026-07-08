package ingest

import "testing"

func TestParseCodexLine(t *testing.T) {
	line := []byte(`{"timestamp":"2026-06-17T17:13:28.019Z","type":"response_item","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":22521,"output_tokens":632,"total_tokens":23153}}}}`)
	ev, ok := ParseCodexLine(line)
	if !ok {
		t.Fatalf("expected token event")
	}
	if ev.Source != "codex" || ev.InputTokens != 22521 || ev.OutputTokens != 632 {
		t.Fatalf("event wrong: %+v", ev)
	}
}

func TestParseCodexLineNonToken(t *testing.T) {
	for _, line := range [][]byte{
		[]byte(`{"timestamp":"2026-06-17T17:13:28Z","type":"response_item","payload":{"type":"message"}}`),
		[]byte(`{"payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":0,"output_tokens":0}}}}`),
		[]byte(`broken`),
	} {
		if _, ok := ParseCodexLine(line); ok {
			t.Errorf("should not parse: %s", line)
		}
	}
}
