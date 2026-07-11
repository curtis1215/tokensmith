# Daily Agent Token Statistics Design

## 1. Goal

Give players a direct view of how many raw tokens each supported coding tool has contributed today, while retaining enough recent information to diagnose collector or accounting problems.

The displayed sources are the existing accounting identities:

- `claude-code`
- `codex`
- `grok`
- `opencode`

Provider, model, session, and nested agent identities are deliberately out of scope. “Today” means the host machine's local calendar date. Statistics start when this feature is installed; existing history is not backfilled.

## 2. User experience

The overview page gains a dedicated card after the primary headquarters/resources area:

```text
┌ 今日 Token 收成 · 07/12 ──────────────────┐
│ Claude Code   138K  （In 120K / Out 18K） │
│ Codex          97K  （In 85K / Out 12K）  │
│ Grok（估算）    30K  （In 30K）            │
│ OpenCode       51K  （In 42K / Out 9K）   │
│ 合計          316K                         │
└───────────────────────────────────────────┘
```

All four sources are always shown. A source without activity displays zero. Grok retains its estimated marker because its collector does not expose a reliable input/output split.

The wide layout shows total, input, and output tokens. Narrow layouts use a compact representation such as `Claude 138K · Codex 97K` and may wrap across lines, but must keep every source visible. At local midnight the model selects the new date immediately, so the card becomes zero even before another token event or file refresh.

## 3. Single source of truth

Add a focused `internal/dailyusage` package. Its default file is:

```text
<os.UserConfigDir()>/tokensmith/daily-usage.json
```

On macOS this normally resolves to `~/Library/Application Support/tokensmith/daily-usage.json`.

The file is independent from `ledger.json`, `meta.json`, and the game save. It contains raw token accounting only and does not participate in R&D settlement.

Schema version 1:

```json
{
  "schemaVersion": 1,
  "updatedAt": 1783814400,
  "days": {
    "2026-07-12": {
      "claude-code": {
        "in": 120000,
        "out": 18000,
        "lastUpdatedAt": 1783814300
      },
      "codex": {
        "in": 85000,
        "out": 12000,
        "lastUpdatedAt": 1783814200
      }
    }
  }
}
```

Date keys use `YYYY-MM-DD` in `time.Local`. `updatedAt` and `lastUpdatedAt` are Unix seconds. Missing source keys mean zero. Readers must tolerate missing optional fields so schema v1 can be extended compatibly.

The store retains at most the seven most recent valid date keys. Pruning occurs during a successful update. A timezone change may create two different local-date buckets; they remain ordinary date keys and are pruned by sorted date order.

## 4. Write path and concurrency

Both harvesting modes call the same store API with positive raw-token deltas:

- `tokensmithd` records deltas produced by its append poller and snapshot watermark calculation.
- standalone TUI records deltas produced by its built-in pollers.
- daemon-mode TUI is read-only because the daemon already records those deltas.

A batch carries its observed local date and timestamp as well as per-source input/output totals. Pending batches preserve that original date, so a retry after midnight cannot move yesterday's tokens into today.

`Store.Add(batch)` performs a locked read-modify-write:

1. Acquire a separate `daily-usage.json.lock` advisory lock with a 250 ms timeout.
2. Load and validate the current document.
3. Add only positive input/output deltas to the requested date/source buckets.
4. Update per-source and document timestamps.
5. Sort and prune to seven date keys.
6. Write a same-directory temporary file and atomically rename it over the destination.
7. Release the lock.

The data and lock files use owner-only permissions. The separate lock file remains stable while the data file is replaced atomically.

Writers keep failed batches in memory and retry them on later polling cycles. A retry must submit each pending batch exactly once after success. Daily-stat failure never blocks cumulative ledger persistence, token-to-R&D conversion, or gameplay.

## 5. Read path

The TUI loads recent statistics on startup and refreshes the file on the existing five-second mutable-source cadence rather than on every 250 ms render tick.

In standalone mode, a newly observed batch also updates the model's in-memory daily view immediately, independent of whether persistence must retry. In daemon mode, the view is refreshed from the file written by `tokensmithd`.

Rendering always selects the current `time.Local` date from the cached document. A date transition therefore shows a zero-value day immediately without requiring disk I/O at the exact midnight boundary.

## 6. Failure behavior

- Missing file: normal first run; return an empty document.
- Lock contention, permission, or I/O error: return an error, keep the batch pending, and retry later.
- Daemon write failure: log a concise error with the affected date and source totals.
- Standalone write failure: keep gameplay running and show a transient daily-stat warning.
- Corrupt or unsupported JSON: preserve the original bytes by renaming the file to `daily-usage.json.corrupt-<unix>`, then create a fresh version-1 document and apply the current batch.
- Invalid or negative deltas: ignore them; daily totals never decrease.
- Reader failure: retain the last valid in-memory view and surface diagnostics without replacing it with zero.

Corruption recovery applies only to the independent statistics file. It must never rename, reset, or rewrite the ledger, meta, or game save.

## 7. Debugging contract

`daily-usage.json` is intentionally human-readable. For each recent day it exposes:

- raw input and output tokens per tool source;
- exact total as `in + out`;
- the last time each source contributed;
- the last successful document update.

These values can be compared directly with `ledger.json.sources` and collector logs. Game multipliers are excluded, so streak, prestige, balance, and R&D changes cannot obscure the raw ingest signal.

No new CLI command, provider API request, session-level detail, or indefinite audit log is included in this change.

## 8. Test plan

### Store

- accumulate multiple batches for the same source and day;
- keep input/output and different sources independent;
- create a new bucket across a local-midnight boundary;
- preserve a batch's original day across a delayed retry;
- retain only seven recent valid dates;
- round-trip timestamps and tolerate absent optional fields;
- preserve and rebuild a corrupt file;
- prove concurrent locked writers do not lose updates;
- prove failed writes do not acknowledge pending data;
- reject negative deltas without decreasing totals.

### Integration

- daemon append and snapshot deltas are recorded exactly once;
- standalone TUI records direct events exactly once;
- daemon-mode TUI never writes a second copy;
- TUI retains the last valid view on a read error;
- local date changes select a zero-value new day immediately;
- pending failures retry once and clear only after success.

### Presentation

- wide overview card shows four sources, input/output, and total;
- zero-activity sources remain visible;
- Grok is marked estimated;
- narrow rendering includes every source without exceeding layout width.

### Completion gate

- format all changed Go files;
- run focused store, daemon, and TUI tests;
- run `go test ./...`;
- run race tests for the daily store and integrations;
- run `go vet ./...` and `go build ./...`;
- verify a read-only local fixture and midnight-date transition without awarding historical usage.

## 9. Non-goals

- Backfilling usage from before the feature is installed.
- Breaking down usage by model, provider, session, or nested agent.
- Using daily totals as the source for R&D settlement.
- Replacing cumulative ledger watermarks or collector cursors.
- Retaining more than seven daily buckets or building a long-term analytics dashboard.
