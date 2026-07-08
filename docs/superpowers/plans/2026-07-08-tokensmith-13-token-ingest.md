# Tokensmith 13 — 真實 token 採集（招牌機制） Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 採集真實 Claude Code / Codex 的 token 消耗，餵進遊戲的 R&D 燃料——你 coding 時遊戲的 R&D 真的爆發。

**Architecture:** 新增 `internal/ingest` 套件（有檔案 I/O，非純；`internal/sim` 仍純）：純 parser `ParseClaudeCodeLine` / `ParseCodexLine`（JSONL 行 → `model.TokenEvent`），與 `Poller`（掃 log 目錄、依 byte offset cursor 只讀新增、冪等）。原型 TUI 每 tick 呼叫 `poller.Poll()`，把新事件傳給 `sim.Tick(state, dt, events, cfg)`，並在畫面顯示 token 灌注。相依 Plan 01–12（及原型 tui/game）。

**Tech Stack:** Go 1.22+、標準 `testing` + `encoding/json` + `os`/`bufio`/`io`/`bytes`/`path/filepath`/`io/fs`/`strings`/`time`。無外部依賴。

## Global Constraints

- 延續 Plan 01–12。`internal/ingest`、`internal/tui`、`internal/game` **可用檔案 I/O 與 time**；`internal/sim` 仍純。
- 真實 schema（已確認）：
  - Claude Code（`~/.claude/projects/**/*.jsonl`）：`type=="assistant"` 行 → `message.usage.{input_tokens,output_tokens}`；頂層 `timestamp`（RFC3339）。
  - Codex（`~/.codex/sessions/**/*.jsonl`）：`payload.type=="token_count"` 行 → `payload.info.last_token_usage.{input_tokens,output_tokens}`；頂層 `timestamp`。
- cursor 冪等：每檔記 byte offset，只讀 offset 之後、且只推進到「最後一個換行」為止（避免半行）；檔案縮小視為輪替，offset 歸零。
- token → R&D 轉換沿用 sim（`(input + 2×output)/10`）；ingest 只產出 `TokenEvent{Source, Timestamp, InputTokens, OutputTokens}`。

---

### Task 1: `ingest.ParseClaudeCodeLine`

**Files:**
- Create: `internal/ingest/claude.go`
- Test: `internal/ingest/claude_test.go`

**Interfaces:**
- Produces: `ingest.ParseClaudeCodeLine(line []byte) (model.TokenEvent, bool)`（`type=="assistant"` 且有 `message.usage` → 事件+true；否則 false）。

- [ ] **Step 1: 寫失敗測試**

Create `internal/ingest/claude_test.go`:
```go
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
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/ingest/`
Expected: FAIL（`undefined: ParseClaudeCodeLine`）。

- [ ] **Step 3: 寫最小實作**

Create `internal/ingest/claude.go`:
```go
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
	}, true
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/ingest/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/ingest/claude.go internal/ingest/claude_test.go
git commit -m "feat(ingest): parse Claude Code token usage lines"
```

---

### Task 2: `ingest.ParseCodexLine`

**Files:**
- Create: `internal/ingest/codex.go`
- Test: `internal/ingest/codex_test.go`

**Interfaces:**
- Produces: `ingest.ParseCodexLine(line []byte) (model.TokenEvent, bool)`（`payload.type=="token_count"` → `last_token_usage` 事件）。

- [ ] **Step 1: 寫失敗測試**

Create `internal/ingest/codex_test.go`:
```go
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
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/ingest/ -run TestParseCodex`
Expected: FAIL（`undefined: ParseCodexLine`）。

- [ ] **Step 3: 寫最小實作**

Create `internal/ingest/codex.go`:
```go
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
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/ingest/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/ingest/codex.go internal/ingest/codex_test.go
git commit -m "feat(ingest): parse Codex token_count lines"
```

---

### Task 3: `ingest.Poller`（cursor tailing）

**Files:**
- Create: `internal/ingest/poller.go`
- Test: `internal/ingest/poller_test.go`

**Interfaces:**
- Produces:
  - `ingest.NewPoller(claudeDir, codexDir string) *Poller`（可注入目錄，供測試）
  - `ingest.NewDefaultPoller() *Poller`（用 `~/.claude/projects` 與 `~/.codex/sessions`）
  - `(*Poller).Poll() []model.TokenEvent`（掃兩目錄下 `*.jsonl`，依 offset 只讀新增、只到最後換行，解析、更新 offset）

- [ ] **Step 1: 寫失敗測試**

Create `internal/ingest/poller_test.go`:
```go
package ingest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPollerTailsNewLines(t *testing.T) {
	claude := t.TempDir()
	codex := t.TempDir()
	f := filepath.Join(claude, "session.jsonl")
	line := `{"type":"assistant","timestamp":"2026-07-07T10:59:19Z","message":{"usage":{"input_tokens":100,"output_tokens":50}}}` + "\n"
	if err := os.WriteFile(f, []byte(line+line), 0o644); err != nil {
		t.Fatal(err)
	}
	p := NewPoller(claude, codex)
	ev := p.Poll()
	if len(ev) != 2 {
		t.Fatalf("first poll = %d events, want 2", len(ev))
	}
	// second poll with no new data → 0
	if got := p.Poll(); len(got) != 0 {
		t.Fatalf("second poll = %d events, want 0 (cursor)", len(got))
	}
	// append one more line → 1 new event
	af, _ := os.OpenFile(f, os.O_APPEND|os.O_WRONLY, 0o644)
	af.WriteString(line)
	af.Close()
	if got := p.Poll(); len(got) != 1 {
		t.Fatalf("third poll = %d events, want 1", len(got))
	}
}

func TestPollerIgnoresPartialLine(t *testing.T) {
	claude := t.TempDir()
	f := filepath.Join(claude, "s.jsonl")
	full := `{"type":"assistant","timestamp":"2026-07-07T10:59:19Z","message":{"usage":{"input_tokens":100,"output_tokens":50}}}` + "\n"
	partial := `{"type":"assistant"` // no newline yet
	os.WriteFile(f, []byte(full+partial), 0o644)
	p := NewPoller(claude, t.TempDir())
	if got := p.Poll(); len(got) != 1 {
		t.Fatalf("poll = %d, want 1 (partial line held back)", len(got))
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/ingest/ -run TestPoller`
Expected: FAIL（`undefined: NewPoller`）。

- [ ] **Step 3: 寫最小實作**

Create `internal/ingest/poller.go`:
```go
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
}

// NewPoller builds a poller over explicit directories (injectable for tests).
func NewPoller(claudeDir, codexDir string) *Poller {
	return &Poller{
		sources: []dirSource{
			{claudeDir, ParseClaudeCodeLine},
			{codexDir, ParseCodexLine},
		},
		offsets: map[string]int64{},
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
			events = append(events, ev)
		}
	}
	p.offsets[path] = off + int64(lastNL) + 1
	return events
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/ingest/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/ingest/poller.go internal/ingest/poller_test.go
git commit -m "feat(ingest): tail JSONL logs with per-file cursor"
```

---

### Task 4: 原型 TUI 接上真實 token 採集

**Files:**
- Modify: `internal/tui/tui.go`
- Test: `internal/tui/tui_test.go`

**Interfaces:**
- Consumes: `ingest.NewDefaultPoller`, `(*Poller).Poll`。
- Produces:
  - `Model` 新欄位 `poller *ingest.Poller`、`lastTokens int`
  - `New()` 建立 poller
  - `Update(tickMsg)`：`events := m.poller.Poll()` → 傳給 `sim.Tick(m.state, tickDT, events, m.cfg)`；`m.lastTokens` = 本次 events 的 input+output 總和
  - `View()` 顯示 token 灌注指示（`lastTokens > 0` 時顯示 `⚡ token +N`）

- [ ] **Step 1: 寫失敗測試**

在 `internal/tui/tui_test.go` 的 import 加入 `"tokensmith/internal/ingest"`，並在末尾新增：
```go
func TestNewHasPoller(t *testing.T) {
	if New().poller == nil {
		t.Fatalf("New() should create an ingest poller")
	}
}

func TestTickPollsTokens(t *testing.T) {
	m := New()
	m.poller = ingest.NewPoller(t.TempDir(), t.TempDir()) // hermetic: empty log dirs
	before := m.state.GameTime
	nm, _ := m.Update(tickMsg(time.Unix(0, 0)))
	if nm.(Model).state.GameTime <= before {
		t.Fatalf("tick did not advance after polling")
	}
}
```

**同時**把既有會 tick 的測試 `TestUpdateTickAdvancesState` 改為注入空目錄 poller（保持 hermetic、不讀真實 log），在 `m := New()` 之後加一行：
```go
	m.poller = ingest.NewPoller(t.TempDir(), t.TempDir())
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/tui/`
Expected: FAIL（`m.poller` undefined）。

- [ ] **Step 3: 寫最小實作**

在 `internal/tui/tui.go`：import 加入 `"tokensmith/internal/ingest"`。

`Model` 結構加入欄位：
```go
	poller     *ingest.Poller
	lastTokens int
```

`New()` 改為建立 poller：
```go
func New() Model {
	return Model{
		state:  game.NewGame(),
		cfg:    balance.Default(),
		poller: ingest.NewDefaultPoller(),
	}
}
```

`Update` 的 `tickMsg` 分支改為採集真實 token：
```go
	case tickMsg:
		events := m.poller.Poll()
		m.lastTokens = 0
		for _, e := range events {
			m.lastTokens += e.InputTokens + e.OutputTokens
		}
		m.state = sim.Tick(m.state, tickDT, events, m.cfg)
		return m, tick()
```

`View()` 的資源條後面加上 token 指示（例如把 `res` 那行改成含 token 灌注）：
```go
	res := fmt.Sprintf("💰 $%.0f    ⚡ R&D %.0f    🖥 訓練 %.0f · 推理 %.1f/%.0f",
		s.Resources.Cash, s.Resources.RnD,
		s.Compute.TrainingCapacity, s.Compute.InferenceLoad, s.Compute.InferenceCapacity)
	if m.lastTokens > 0 {
		res += fmt.Sprintf("    ⚡token +%d", m.lastTokens)
	}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/tui/`
Expected: PASS。

- [ ] **Step 5: 跑整包 + vet + build + commit**

Run:
```bash
go test ./...
go vet ./...
go build -o tokensmith .
```
Expected: 全部 PASS、vet 無警告、可建置。

```bash
git add internal/tui/tui.go internal/tui/tui_test.go
git commit -m "feat(tui): feed real Claude Code/Codex tokens into the sim"
```

---

## 完成後狀態

招牌機制生效：原型每 tick 掃 `~/.claude/projects` 與 `~/.codex/sessions` 的 JSONL，把新的 token 消耗（Claude Code assistant usage + Codex token_count）轉成 `TokenEvent` 餵進 `sim.Tick` 的燃料迴圈——你用 Claude Code / Codex coding 時，遊戲的 R&D 真的爆發，畫面顯示 `⚡token +N`。cursor 冪等、只讀新增、`internal/sim` 仍純。

**後續**：正式 daemon + IPC（常駐背景採集 + 桌面通知）、SQLite 存檔、完整六頁 TUI。
