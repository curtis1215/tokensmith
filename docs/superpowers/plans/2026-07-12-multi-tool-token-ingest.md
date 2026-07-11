# Multi-tool Token Ingest Implementation Plan

> **For implementers:** use test-driven development for every behavior change and verification-before-completion before claiming success.

**Goal:** Automatically harvest Claude Code, Codex (including Orca homes), standalone Grok CLI, and OpenCode usage without user registration or runtime dependence on CodexBar.

**Architecture:** Preserve the existing append-only JSONL poller for Claude/Codex, generalize it to multiple roots, and add cumulative snapshot collectors for Grok and OpenCode. Persist snapshot watermarks beside existing cursors so upgrades prime current history and only future deltas become R&D.

**Tech stack:** Go 1.25, stdlib JSON/filesystem packages, `database/sql` with a pure-Go SQLite driver, existing ledger/TUI packages.

## Task 1: Codex multi-home discovery

- [x] Add failing tests for default, `CODEX_HOME`, Orca, root deduplication, and two-root polling.
- [x] Generalize `ingest.Poller` to accept multiple `(root, parser)` sources while keeping `NewPoller(claude, codex)` compatibility.
- [x] Add a deterministic Codex session-root locator and wire it into `tokensmithd`.

## Task 2: Snapshot watermark contract

- [x] Add ledger round-trip tests for `Snapshots`.
- [x] Add daemon tests proving first-observation priming, positive delta credit, reset/rebaseline, and independent sources.
- [x] Add a snapshot-source interface and persist watermarks without changing `ledger.Sources` or TUI consumption.

## Task 3: Grok collector

- [x] Add failing fixture tests for multiple `signals.json` files and malformed input.
- [x] Implement `$GROK_HOME`/default discovery, cached recursive scan, and estimated cumulative totals.
- [x] Wire the collector as source `grok`.

## Task 4: OpenCode collector

- [x] Add a pure-Go SQLite dependency.
- [x] Create failing tests with a temporary OpenCode schema and assistant/user rows.
- [x] Implement `$XDG_DATA_HOME`/default DB discovery and read-only cumulative token queries.
- [x] Wire the collector as source `opencode`.

## Task 5: Presentation and documentation

- [x] Add TUI label tests and labels for Grok/OpenCode.
- [x] Update deployment/data-source documentation.
- [x] Verify migration behavior against existing ledger files.

## Task 6: Verification

- [x] Run `gofmt -w` on changed Go files.
- [x] Run focused ingest/daemon/TUI tests.
- [x] Run `go test ./...`.
- [x] Run `go vet ./...`.
- [x] Run `go build ./...`.
- [x] Run local read-only fixture probes against the installed Grok/OpenCode data and confirm no historical tokens are credited on first observation.
