# Tokensmith — Install & Deployment

Reference for humans and agents: how to install, run, and release the game.

## What it is

- Terminal (TUI) management/idle game about running an AI company, written in Go (module `tokensmith`, Go 1.22+; `go.mod` pins 1.25).
- **Signature mechanic:** it harvests your *real* Claude Code / Codex token usage (from `~/.claude/projects` and `~/.codex/sessions` JSONL logs) and converts it into in-game R&D.
- **Two binaries:**
  - `tokensmith` — the game (Bubble Tea TUI). Entry: `main.go`.
  - `tokensmithd` — background daemon that continuously harvests token logs into a ledger the game consumes. Entry: `cmd/tokensmithd/main.go`.

## Install (end users)

### Homebrew (recommended)

```sh
brew install curtis1215/tap/tokensmith   # installs both binaries
brew services start tokensmith           # run the harvest daemon in the background (optional)
tokensmith                               # play
```

Upgrade to the latest release: `brew update && brew upgrade tokensmith`.
Tap repo: <https://github.com/curtis1215/homebrew-tap>

> Note: `brew upgrade` replaces the on-disk binary but does NOT restart an already-running `tokensmith` process. Quit the game (`q`) and relaunch to run the new version.

### From source

```sh
git clone https://github.com/curtis1215/tokensmith
cd tokensmith
go build -o tokensmith .                    # the game
go build -o tokensmithd ./cmd/tokensmithd   # the daemon
./tokensmith
```

## Run / usage

- `tokensmith` — launch the TUI game. Keys are shown per page (Tab switches pages; `q` quits and saves).
- `tokensmithd` — background daemon: every ~5s it tails `~/.claude` + `~/.codex` and accumulates token usage into `ledger.json` (with durable cursors). Start via `brew services start tokensmith`, or run directly: `./tokensmithd &`.
- Without a running daemon the game falls back to its built-in poller (standalone mode); the daemon adds continuous + offline capture.
- `tokensmith --version` / `tokensmithd --version` print the build version.

## Data locations (macOS: `~/Library/Application Support/tokensmith/`)

`os.UserConfigDir()/tokensmith/`:

| File | Owner | Purpose |
|---|---|---|
| `save.json` | game | game save (autosaved ~every 40 ticks + on quit) |
| `ledger.json` | daemon | cumulative token harvest + per-file cursors |
| `meta.json` | game | consumed-token watermark + last-play wall time (for offline settlement) |
| `ledger.json.lock` | daemon | single-instance PID lock |

**Reset a run** (needed after balance changes): quit the game, `rm ~/Library/Application\ Support/tokensmith/save.json`, relaunch.

## Deploy / release (maintainers & agents)

### Repos

- Source: <https://github.com/curtis1215/tokensmith> — public, default branch `main`.
- Homebrew tap: <https://github.com/curtis1215/homebrew-tap> — holds `Formula/tokensmith.rb`.

### Release flow (automated)

Tag a semver on `main` and push — CI does the rest:

```sh
git tag vX.Y.Z && git push origin vX.Y.Z
```

`.github/workflows/release.yml` triggers on `v*` tags and runs **GoReleaser** (`.goreleaser.yaml`):

1. Cross-compiles both binaries for darwin + linux × amd64 + arm64. Version is injected via `-ldflags "-X main.version={{.Version}}"`.
2. Creates the GitHub Release with tarballs + `checksums.txt`, using the workflow's default `GITHUB_TOKEN` (same-repo, no PAT needed).
3. Regenerates `Formula/tokensmith.rb` and pushes it to the tap over **git + SSH** using a deploy key scoped to `homebrew-tap` only. The formula bundles both binaries and a `service` block so `brew services start tokensmith` runs the daemon.

End users then receive it via `brew upgrade tokensmith`.

### Secrets

- **`HOMEBREW_TAP_DEPLOY_KEY`** — GitHub Actions secret on the `tokensmith` repo. An SSH *private* key whose public half is a write deploy key on `homebrew-tap` (scope: that one repo only). The workflow writes it to `~/.ssh/tap_deploy_key` and points GoReleaser at it via `TAP_DEPLOY_KEY_FILE`. Backed up in 1Password → Private vault → item **"Tokensmith Homebrew Tap Deploy Key"**.
  - Rotate: delete the deploy key on `homebrew-tap` → `ssh-keygen -t ed25519` → add the new public key as a write deploy key → update the GitHub secret + the 1Password item.
- No cross-repo PAT is used; the GitHub Release itself only needs the default `GITHUB_TOKEN`.

### Local release (fallback, without CI)

```sh
GITHUB_TOKEN=$(gh auth token) \
TAP_DEPLOY_KEY_FILE=/absolute/path/to/tap_deploy_key \
goreleaser release --clean
```

Requires `goreleaser` (`brew install goreleaser`) and a clean tree on the tag. GoReleaser 2.x still supports the (deprecated) `brews` block used here.

### Versioning

- `var version = "dev"` lives in both `main.go` and `cmd/tokensmithd/main.go`; GoReleaser overrides it per tag.
- Bump minor for features, patch for fixes. Current line: `v0.x`.

## Repo layout (navigation)

| Path | Responsibility |
|---|---|
| `main.go` | game entry (Bubble Tea program) |
| `cmd/tokensmithd/` | daemon entry (harvest loop + lock) |
| `internal/model` | shared value types (pure, no deps) |
| `internal/balance` | all tunable numbers (`Config`, `Default()`) |
| `internal/sim` | pure deterministic simulation core — **no wall-clock/rand/IO; time advances only via `dt`** |
| `internal/ingest` | reads Claude/Codex JSONL token logs (poller + cursors) |
| `internal/ledger` / `internal/store` | persistence (ledger; save + meta) |
| `internal/daemon` | harvest loop + single-instance lock |
| `internal/game` | new-run seeding |
| `internal/tui` | Bubble Tea UI (six pages: 總覽/模型/市場/算力/團隊/科技) |
| `docs/superpowers/specs` · `plans` | design specs + implementation plans |

## Verify / CI gate

```sh
go test ./... && go vet ./... && go build ./...
```

Everything must be green before tagging a release. `internal/sim` purity is load-bearing: keep it free of wall-clock, randomness, and I/O.
