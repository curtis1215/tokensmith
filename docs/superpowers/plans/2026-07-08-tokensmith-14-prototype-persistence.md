# Tokensmith 14 — 原型存檔 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 讓原型的遊戲狀態存檔——關掉再開會接續上次進度（而非重來）。用 JSON 檔存 `GameState`（正式 daemon 才上 SQLite）。

**Architecture:** 新增 `internal/store` 套件：`Save(path, state)` / `Load(path)` 把 `model.GameState` 以 JSON 落地 / 讀回。原型 `New()` 若有存檔就載入（否則 `game.NewGame()`）；`Update` 每隔一段 tick 存一次、按 `q` 離開前存一次。相依 Plan 01–13（及原型 tui/game）。

**Tech Stack:** Go 1.22+、標準 `testing` + `encoding/json` + `os`/`path/filepath`。無外部依賴。

## Global Constraints

- 延續 Plan 01–13。`internal/store`、`internal/tui` 可用檔案 I/O；`internal/sim` 仍純。
- `GameState` 全欄位皆已匯出，可直接 `encoding/json` 序列化。
- 存檔位置：`~/Library/Application Support/tokensmith/save.json`（跨平台以 `os.UserConfigDir()` 決定）。
- 存檔頻率：每 40 tick（約 10 秒）一次 + 離開前一次；寫入用「先寫暫存檔再 rename」原子替換。

---

### Task 1: `internal/store` — Save / Load

**Files:**
- Create: `internal/store/store.go`
- Test: `internal/store/store_test.go`

**Interfaces:**
- Produces:
  - `store.Save(path string, s model.GameState) error`（原子寫：temp + rename；自動建目錄）
  - `store.Load(path string) (model.GameState, bool, error)`（不存在 → `(zero, false, nil)`）
  - `store.DefaultPath() string`（`<UserConfigDir>/tokensmith/save.json`）

- [ ] **Step 1: 寫失敗測試**

Create `internal/store/store_test.go`:
```go
package store

import (
	"path/filepath"
	"testing"

	"tokensmith/internal/model"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "save.json")
	var s model.GameState
	s.Resources.Cash = 12345
	s.Resources.RnD = 6789
	s.Models = []model.Model{{Gen: 2, Online: true, Users: 1000, Price: 12}}
	s.Prestige.Patents = 3
	s.HiredStars = []string{"aria-chen"}
	if err := Save(path, s); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, ok, err := Load(path)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if got.Resources.Cash != 12345 || got.Resources.RnD != 6789 {
		t.Errorf("resources not restored: %+v", got.Resources)
	}
	if len(got.Models) != 1 || got.Models[0].Users != 1000 {
		t.Errorf("models not restored: %+v", got.Models)
	}
	if got.Prestige.Patents != 3 || len(got.HiredStars) != 1 {
		t.Errorf("prestige/stars not restored: %+v %+v", got.Prestige, got.HiredStars)
	}
}

func TestLoadMissing(t *testing.T) {
	_, ok, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil || ok {
		t.Fatalf("missing file: ok=%v err=%v, want false/nil", ok, err)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/store/`
Expected: FAIL（`undefined: Save`）。

- [ ] **Step 3: 寫最小實作**

Create `internal/store/store.go`:
```go
// Package store persists the game state to a JSON file.
package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"tokensmith/internal/model"
)

// DefaultPath is the standard save-file location.
func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "tokensmith", "save.json")
}

// Save writes the state to path atomically (temp file + rename).
func Save(path string, s model.GameState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Load reads the state from path. Returns ok=false if the file does not exist.
func Load(path string) (model.GameState, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return model.GameState{}, false, nil
	}
	if err != nil {
		return model.GameState{}, false, err
	}
	var s model.GameState
	if err := json.Unmarshal(data, &s); err != nil {
		return model.GameState{}, false, err
	}
	return s, true, nil
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/store/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/store_test.go
git commit -m "feat(store): JSON save/load of game state"
```

---

### Task 2: 原型接上存檔（載入 / 定期存 / 離開存）

**Files:**
- Modify: `internal/tui/tui.go`
- Test: `internal/tui/tui_test.go`

**Interfaces:**
- Consumes: `store.Load` / `Save` / `DefaultPath`。
- Produces:
  - `Model` 新欄位 `savePath string`、`ticksSinceSave int`
  - `New()`：若 `store.Load(DefaultPath())` 有存檔 → 用它；否則 `game.NewGame()`
  - `Update(tickMsg)`：每 40 tick 存一次
  - `Update(key "q"/"ctrl+c")`：離開前 `store.Save` 再 `tea.Quit`

- [ ] **Step 1: 寫失敗測試**

在 `internal/tui/tui_test.go` 末尾新增（import 需含 `"os"`, `"path/filepath"`, `"tokensmith/internal/store"`, `"tokensmith/internal/model"`）：
```go
func TestNewLoadsSaveIfPresent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "save.json")
	var s model.GameState
	s.Resources.RnD = 999999
	if err := store.Save(path, s); err != nil {
		t.Fatal(err)
	}
	m := newAt(path) // test constructor with explicit save path
	if m.state.Resources.RnD != 999999 {
		t.Fatalf("New did not load save: RnD=%v", m.state.Resources.RnD)
	}
}

func TestQuitSavesState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "save.json")
	m := newAt(path)
	m.poller = ingestEmptyPoller(t) // hermetic
	m.state.Resources.Cash = 42
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("quit should return a command")
	}
	got, ok, _ := store.Load(path)
	if !ok || got.Resources.Cash != 42 {
		t.Fatalf("quit did not save: ok=%v cash=%v", ok, got.Resources.Cash)
	}
	_ = os.Remove(path)
}
```

（若 `ingestEmptyPoller` 尚未存在，於測試檔加入 helper：
```go
func ingestEmptyPoller(t *testing.T) *ingest.Poller {
	return ingest.NewPoller(t.TempDir(), t.TempDir())
}
```）

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/tui/`
Expected: FAIL（`undefined: newAt`）。

- [ ] **Step 3: 寫最小實作**

在 `internal/tui/tui.go`：import 加入 `"tokensmith/internal/store"`。

`Model` 加欄位：
```go
	savePath       string
	ticksSinceSave int
```

把 `New()` 改為委派給 `newAt`，並新增 `newAt`：
```go
func New() Model { return newAt(store.DefaultPath()) }

func newAt(savePath string) Model {
	state, ok, _ := store.Load(savePath)
	if !ok {
		state = game.NewGame()
	}
	return Model{
		state:    state,
		cfg:      balance.Default(),
		poller:   ingest.NewDefaultPoller(),
		savePath: savePath,
	}
}
```

`Update` 的 `tickMsg` 分支末尾（`return m, tick()` 之前）加入定期存檔：
```go
		m.ticksSinceSave++
		if m.ticksSinceSave >= 40 {
			m.ticksSinceSave = 0
			_ = store.Save(m.savePath, m.state)
		}
```

`Update` 的 `q` / `ctrl+c` 分支改為先存再離開：
```go
		case "q", "ctrl+c":
			_ = store.Save(m.savePath, m.state)
			return m, tea.Quit
```

**測試潔淨性（重要）**：既有 tui 測試都用 `New()`，現在會讀 / 寫真實存檔位置（`TestQuitKey` 甚至會寫出真實 `save.json`）。把**所有**既有用 `m := New()` 的 tui 測試改成 `m := newAt(filepath.Join(t.TempDir(), "s.json"))`，確保 hermetic、不碰真實存檔。（`TestNewHasPoller`、`TestUpdateTickAdvancesState`、`TestTrainKeyStartsTraining`、`TestRentKeysAddCapacity`、`TestViewNonEmpty`、`TestQuitKey`、`TestTickPollsTokens` 等。）

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
git commit -m "feat(tui): load/save game state for persistent runs"
```

---

## 完成後狀態

原型可存檔：關掉再開會接續上次進度（現金 / R&D / 模型 / 對手 / 科技 / prestige / 明星全部保留），每約 10 秒自動存、離開前也存。存檔在 `<UserConfigDir>/tokensmith/save.json`，原子寫入。搭配 token 採集，真正達成「陪跑」——背景累積、隨時回來接續。

**後續**：正式 daemon + IPC（常駐背景 + 桌面通知）、SQLite 存檔（取代 JSON、含 ingest cursors / 事件日誌 / prestige 跨局）、完整六頁 TUI。
