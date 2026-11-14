---
created: 2026-11-14
status: complete
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-002
system_design:
  - ../../specs/platform/system-design/interrupted-prompt-recovery.md
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
legacy_specs: []
---

# Implementation Plan: Resume blocked by an earlier settled incident

## Overview

After [#5312](https://github.com/kdlbs/kandev/pull/5312) shipped, Resume on task
`fc7cfeee` (session `cbf39cbf`, ACP `01a1ccb6`) still failed. Codex reattached to
the same conversation and the interrupted "continue" prompt was retired in the
agentctl journal, but the SQL recovery block stayed open. A follow-up "Continue
from saved history" then failed with "Failed to start agent Codex".

Intended behavior is unchanged and already specified: Resume keeps the native
conversation and clears the block (`AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.12`,
`.24`), and context continuation works (`AC-AGENTS-HARNESS-SESSION-CONTINUITY-002.1`).
This plan fixes implementation defects; no requirement changes.

## Evidence and root causes

| # | Defect | Root cause | Evidence |
| --- | --- | --- | --- |
| A | Resume leaves the block open | Session carries records from an earlier incident (`prompt:internal:33359210`, settled `cancelled` on 2026-11-12 02:38): a `silent_restart_restore` attempt still `pending` and a recovery notice in phase `settled`. `finishPreparedNativeAttemptTx` and `restoreAuthenticatedNativePresentationTx` route through the historical-terminal checks, which require the earlier submission in `proof.Submissions`. The lifecycle proof lists only unresolved SQL rows, so both return false. | Read-only replay of the session's live rows in a temp DB: without the earlier submission `finishPreparedNativeAttemptTx=false`, notice step false, full resolve `changed=false`. With it: both true, `changed=true`. Settlement-time attempt retirement only exists since #5296 (2026-11-13), so this is legacy data. |
| B | "Continue from saved history" fails to start | `replacementGenerationPending` (`lifecycle/durable_silent_restore_generation.go`) waits for a successor SQL generation only when `ReplacesNativeConversation` (fresh start). Context continuation sets `ForceContextContinuation`, so its `g2` events are compared with SQL generation 1 and rejected (`deliveryEventIsStale` → `durable adoption owner identity mismatch`), the stream closes, and `session/new` fails. | Log 10:39:49.587 `durable agent event ownership check for "mcp_attachment": durable adoption owner identity mismatch`, then `session/new failed: ... agent stream not connected`. |
| C | Refusal logged without cause | Resolver returns `false, nil`; `sessionRecoveryBlockResolutionGuidance` returns a plain recovery error, logged as `reason: unclassified`. | Log 10:39:38.879. |
| D | Failed continuation leaves an orphan stream | Journal stream `…:g2` has `high_water=32, acknowledged=0`, no SQL generation row and no SQL cursor, so `HasUnresolvedWork` is true for the session. This may block the next Resume. | Read-only inspection of a copy of `delivery.bbolt`. |

## Scope

### In scope

- Task 01: the historical-terminal checks accept the SQL settlement when the proof
  omits the earlier submission; refusal-step WARN; `sql_resolution_refused` reason.
- Task 02: successor-generation wait covers context continuation.
- Task 03: abandon orphaned successor streams (failed bootstrap and before native
  Resume evaluates unresolved work).

### Out of scope

- Requirement changes, UI changes, or new recovery controls.
- Startup sweeps. A restart still starts no agent; repairs happen inside Resume.
- The SQL `cancelled` vs journal `completed` label of the earlier submission. The
  SQL settlement is authoritative for a fully settled row; Task 01 does not
  compare it with the journal.

## Work orders

| Task | Title | Status | Depends on |
| --- | --- | --- | --- |
| [01](task-01-accept-settled-earlier-incidents.md) | Accept settled earlier incidents during Resume | done | none |
| [02](task-02-continuation-successor-wait.md) | Successor wait covers context continuation | done | none |
| [03](task-03-abandon-orphan-successor-streams.md) | Abandon orphaned successor streams | done | 02 |

Tasks run in order 01, 02, 03 in one PR.

## Verification

```bash
cd apps/backend
go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite/ -run 'NativeResume|HistoricalTerminal|SettledEarlier' -count=1
go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle/ -run 'Successor|Continuation|SilentRestore|OrphanStream|NativeResume' -count=1
go test -trimpath -tags fts5 -race ./internal/orchestrator/ -run 'NativeResume|DeliveryRecovery|RefusalLog' -count=1
go test -trimpath -tags fts5 ./internal/agentctl/journal/ ./internal/agentctl/server/process/ -count=1
cd ../.. && bash scripts/lint-go-changed $(git diff --name-only origin/main -- '*.go')
```

Live check after merge and restart: Resume on task `fc7cfeee` clears the banner
and keeps ACP session `01a1ccb6-86db-7033-abf2-6f232e14c06b`.

## Risks

- Task 01 must not accept an unsettled earlier submission. Keep every condition of
  `loadHistoricalTerminalSettlementTx`; only the proof-row presence becomes optional.
- Task 03 deletes journal data. Abandon only streams with no committed SQL
  generation, no SQL cursor and no live owner; never a committed generation's
  stream.
