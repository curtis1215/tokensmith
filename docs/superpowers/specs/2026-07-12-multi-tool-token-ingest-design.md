# Multi-tool Token Ingest — Design

**Date:** 2026-07-12
**Status:** Implemented locally; pending review/merge
**Parent spec:** `2026-07-07-tokensmith-design.md` §4（token 採集）與 `2026-07-10-token-rnd-rebalance-design.md` §3–4

## 1. Goal

Tokensmith must harvest activity from four coding tools without asking the user or an agent to register a path:

- Claude Code (`claude-code`)
- Codex, including alternate Codex homes such as Orca (`codex`)
- standalone Grok CLI (`grok`)
- OpenCode (`opencode`)

Tool identity, not model provider identity, is the accounting boundary. An xAI/Grok model invoked inside OpenCode is credited once to `opencode`; only standalone Grok CLI sessions are credited to `grok`.

## 2. Non-goals

- Do not import browser cookies or OAuth refresh tokens.
- Do not convert quota percentages, billing cents, or credits into R&D.
- Do not require CodexBar at runtime.
- Do not retroactively award historical usage when a source is first discovered.
- Do not change the R&D balance formula or `sim.Tick` signature.

## 3. Architecture

The daemon uses two collector families:

1. **Append-only log collectors** for Claude Code and Codex. They retain the existing inode + byte-offset cursor behavior.
2. **Snapshot collectors** for mutable/cumulative sources. Grok rewrites `signals.json`; OpenCode persists completed assistant messages in SQLite. Their current cumulative totals are compared with a persisted per-source watermark.

```text
Claude JSONL ─┐
Codex JSONL ──┼─ append poller ─┐
              │                 ├─ TokenEvent ─ ledger.Sources ─ TUI
Grok signals ─┼─ snapshot poller┤
OpenCode DB ──┘                 ┘
```

`ledger.Ledger` gains:

```go
Snapshots map[string]model.SourceTotals `json:"snapshots,omitempty"`
```

On the first observation of a snapshot source, the daemon records the current total without awarding it. Later positive deltas are credited. A decreasing total (session cleanup, DB rebuild, or source reset) re-baselines without producing a negative event or waiting for the old total to be exceeded. Snapshot collectors distinguish an absent store from a present store whose total is genuinely zero; absence leaves the persisted watermark untouched.

## 4. Source details

### Claude Code

Keep the existing `~/.claude/projects/**/*.jsonl` parser and message-ID deduplication unchanged.

### Codex

The Codex source locator returns a deduplicated set of session roots:

1. `$CODEX_HOME/sessions`, when set for the daemon.
2. `~/.codex/sessions`.
3. Orca's `~/Library/Application Support/orca/codex-runtime-home/home/sessions` on macOS.

Each root uses the same `token_count` parser. A rollout path is only visited once even if roots resolve to the same directory. New roots are primed to EOF, so discovery never replays history. Durable cursors include device/inode identity, and a later path or hard link to an already-tailed file inherits that physical file's cursor across polls.

### Grok CLI

Use `$GROK_HOME` or `~/.grok`, then recursively inspect `sessions/**/signals.json`. For every session:

```text
estimated tokens = totalTokensBeforeCompaction + contextTokensUsed
```

The aggregate is stored as `SourceTotals.In`; `Out` remains zero because the file does not expose a reliable input/output split. The TUI labels Grok as estimated. File mtime/size caching avoids re-reading unchanged snapshots, and a transient malformed rewrite retains the last valid cached value until the file becomes readable again.

Grok billing RPC values are monetary cents, not token counts, and are not used for R&D.

### OpenCode

Use `$XDG_DATA_HOME/opencode/opencode.db`, otherwise `~/.local/share/opencode/opencode.db`. Open the database read-only and sum completed assistant-message `data.tokens.input` and `data.tokens.output` values. The collector includes all model providers because the accounting source is OpenCode itself.

Cache-read, cache-write, and reasoning subtotals are deliberately excluded in this change to preserve the existing Tokensmith input/output balance contract. DB and WAL file signatures avoid rerunning the aggregate query when nothing changed.

## 5. Failure and privacy behavior

- A missing optional source is normal and silent.
- A temporarily locked OpenCode DB leaves the previous snapshot intact and retries later.
- A malformed Grok file is skipped without blocking other sources.
- Collector errors are logged by the daemon but do not prevent ledger persistence for healthy sources.
- Standalone TUI mode polls mutable snapshots every five seconds instead of on the 250 ms render loop.
- All collection remains local and read-only; no browser, Keychain, OAuth, or provider API access is introduced.

## 6. Display

Known source labels are:

- `Claude Code`
- `Codex`
- `Grok（估算）`
- `OpenCode`

Unknown source keys continue to fall back to their raw string.

## 7. Testing

- Codex locator: default, environment, Orca, and deduplication.
- Multi-root append poller: events from two Codex homes are both harvested once.
- Grok snapshot: multi-file sum, malformed file skip, unchanged-cache behavior.
- OpenCode snapshot: assistant-only filtering, input/output extraction, absent/locked database behavior.
- Daemon watermark: first observation primes, growth credits only the delta, decrease re-baselines, and source absence preserves the watermark across restart.
- TUI source labels include Grok's estimated marker.
- Full validation: `gofmt`, `go test ./...`, `go vet ./...`, and `go build ./...`.
