---
created: 2026-10-11
updated: 2026-10-11
status: draft
requirements:
  - REQ-SYSTEM-PAGE-DB-HISTORY-RETENTION-001
  - REQ-SYSTEM-PAGE-DB-HISTORY-RETENTION-002
  - REQ-SYSTEM-PAGE-DB-HISTORY-RETENTION-003
  - REQ-SYSTEM-PAGE-DB-HISTORY-RETENTION-004
  - REQ-SYSTEM-PAGE-DB-HISTORY-RETENTION-005
  - REQ-SYSTEM-PAGE-DB-HISTORY-RETENTION-006
system_design:
  - ../../specs/system-page/system-design/db-history-retention.md
---

# Implementation Plan: DB History Retention

## Overview

Make the operator's approved manual cleanup periodic: reduce message metadata for
messages whose task is finished or archived at a 14-day window, and reduce
completed cleanup-job snapshots on the same pass, so the growth that motivated
this work does not recur.

The change **extends the existing periodic retention policy in place** rather
than adding a sibling record and card. The frame in
`internal/system/dataretention` is hardwired to one policy (settings key
`archived_data_retention` at `store.go:12`, route base at `handler.go:13`, the
two-age record at `policy.go:38`, and archived-only selection at `scan.go:139`),
so a second, differently shaped policy would mean re-designing or copying about
ten reviewed files plus a completed settings card. Instead this plan broadens the
existing message target:

- The message target's eligibility moves from archived-only to **finished or
  archived**, and its initial window moves from 30 to **14 days**.
- The completed cleanup-snapshot target is **reused unchanged** on the same
  periodic pass.
- The `archived_data_retention` settings record, its revision and approval gate,
  the preparation and daily schedule, and the `ArchivedDataRetentionCard` are
  reused; no second record, revision, or card is added.

The existing tool payload policy stays the inactivity-based policy over
active-task history and keeps its own record, window, and approved state. Because
reduction is idempotent by removal marker, this capability and the tool payload
policy remain no-ops on rows the other already reduced.

## Non-goals

- No new settings record, revision stream, or settings card.
- No change to the `tool_payload_retention` record, window, or approved state.
- No message deletion, message content change, database compaction, or file-size
  reduction. No automatic `VACUUM` competing with writes.
- No automatic session-metadata reduction, no catalog-revision pruning, and no
  plan-revision removal. REQ-003 and REQ-004 are design criteria only here.
- No new permanent gate, fingerprint, hash, signature, audit, monitor, or ledger.
- No PostgreSQL support and no change to Office run-history, filesystem, or
  delivery-inbox retention.

## Backend

### Broaden the message target (work order 01)

In `internal/task/repository/sqlite/archived_data_retention_scan.go`, generalize
the candidate walk and its upper cursor from archived tasks to the union of
archived (`archived_at IS NOT NULL`) and finished (task state `COMPLETED`,
`FAILED`, `CANCELLED`) tasks, and add a `HistoryTaskEligible` that is
`ArchivedTaskEligible` **or** (`terminal task state` **and**
`PayloadTaskEligible`) at the same cutoff. In
`internal/system/dataretention/scan.go`, use the generalized selection and
eligibility. In `internal/system/dataretention/policy.go`, move the message
window's initial value from 30 to 14 days. The reducer, marker, eligibility
contracts, keyset paging, and compare-and-swap writes are reused unchanged; the
record shape and settings key are unchanged so a saved record keeps its saved
value.

### Reuse the cleanup-snapshot target (work order 02)

The completed cleanup-job snapshot target already exists in the frame
(`scanJobBatch`, `reduceCleanupJob` in `internal/system/dataretention/scan.go`).
This work order keeps it pinned on the same bounded pass as the broadened message
target, confirming the shared batch budget, evidence-preservation contract, and
malformed-snapshot handling with tests. No new reducer, table, or migration.

### Settings surface (work order 03)

Broaden the existing `ArchivedDataRetentionCard` copy so the message window reads
as covering finished or archived tasks, and update its localized strings in every
catalog. No new card, route, or navigation.

## Frontend

The only frontend work is the broadened card copy and its localized strings
(work order 03). No new route, navigation, or browser workflow is introduced.

## Deferred design (not implemented in this package)

REQ-003 and REQ-004 fix safe criteria for the largest remaining categories; they
are documented in the system design and are not implementation work orders:

- Session model metadata and repeated model-list copies: share an inline provider
  catalog once under the session's runtime-identity key, preserve the selection,
  and (offline, backup-gated) prune a shared revision that no session
  `catalog_ref` names.
- Plan revisions: preserve HEAD, `revert_of_revision_id` ancestry, and
  user-authored revisions; exclude active tasks; keep a reversible recency
  window; remove only offline under a verified backup.

A later package with an explicit instruction owns the implementation.

## Tests

- **Message target:** repository tests prove an in-window finished candidate and
  an in-window archived candidate are reduced with the existing marker; a live or
  pending task, a nonterminal task, and an out-of-window task are retained; an
  already-reduced row is a no-op; and a concurrent metadata write wins
  (compare-and-swap). `internal/system/dataretention` tests prove the target
  shares the batch budget, the approved-revision gate, the maintenance admission,
  and restart resumption.
- **Cleanup-snapshot target:** tests prove a succeeded in-window job is reduced
  with the evidence contract preserved, a non-succeeded or out-of-window job is
  retained, and a malformed snapshot is skipped without deletion.
- **Settings:** web tests prove the card withholds deletion until preparation is
  approved and keeps the stored-byte-reduction copy.

Every test is source-adjacent (a backend `*_test.go` next to its source, a
frontend `*.test.ts` next to its module). No permanent integration, E2E, or CI
job is added, and no verification framework is introduced.

## Verification strategy

Run only the changed packages:

```bash
cd apps/backend && go test ./internal/system/dataretention/... ./internal/task/repository/sqlite/... ./internal/task/models/... -count=1
cd apps/backend && go test ./internal/system/dataretention/... ./internal/task/repository/sqlite/... -race -count=1
cd apps/backend && go build ./... && go vet ./...
cd apps/backend && gofmt -l internal/system/dataretention internal/task/repository/sqlite internal/task/models
cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check
```

The end-to-end demonstration for this package is a finite, isolated readback
outside automatic test collection: build a small fixture database with an
in-window finished task, an archived task, a live task, and a completed cleanup
job; run the periodic analysis and a pass; and print before/after counts and
stored-byte estimates per target plus proof that the live task, the selection,
and the archive evidence are preserved. It is a one-off confirmation, not a
permanent job.

## Implementation waves

Wave 1 (backend, shared retention service and SQLite package):

- [x] [task-01-message-metadata-target](task-01-message-metadata-target.md)
- [x] [task-02-cleanup-snapshot-target](task-02-cleanup-snapshot-target.md)

Wave 2 (surface, depends on the backend target):

- [x] [task-03-settings-surface](task-03-settings-surface.md)

Wave 3 (documentation, depends on 01 and 02):

- [x] [task-04-operator-documentation](task-04-operator-documentation.md)

Work orders 01 and 02 touch the same retention service and SQLite repository
package, so they are not parallel-safe from one worktree.

## Risks

- The broadened message target overlaps the tool payload policy on the same
  field. It is safe because reduction is idempotent by marker and the tool
  payload policy keeps its own record and protections; the overlap is deliberate
  and documented.
- The finished-task eligibility is expensive; the target reuses the existing
  statement budget and fails closed rather than guessing.
- A saved `archived_data_retention` record keeps its saved window, so an install
  that already saved 30 days does not silently move to 14; the administrator sets
  the window in the card.
- Plan-revision and catalog removal are destructive; they remain deferred and
  offline-only, not part of this implementation.

## Open choices for the owner (not decided here)

None block the requested outcome. The 14-day window is the owner-provided
contract for this capability (REQ-001, REQ-005) and is the initial value of the
existing record, not a reversible default to re-tune. A separate choice remains
whether the deferred REQ-003 and REQ-004 designs are implemented.

## Out of scope

- Delivery inbox and effect retention (separate task), restart-midturn repair
  (Operations), Office run-history and filesystem-cache retention.
- Message deletion, compaction, index removal, and file-size reduction.
- Automatic session-metadata reduction, catalog-revision pruning, and
  plan-revision removal; worktrees, task environments, attachments, and
  provider-side history.

## Implementation results

Status: implemented on branch `feature/kandev-w-099-kandev-m4a`; PR opened from
the fork.

- `internal/task/repository/sqlite/archived_data_retention_scan.go`: added
  `HistoryTaskEligible` (archived OR finished), `FinishedTaskEligible`,
  `terminalTaskState`, and replaced the archived-only cursor with
  `NextHistoryTaskID`/`HistoryUpperTaskID`.
- `internal/system/dataretention/scan.go`, `service.go`: the message pass now
  uses the broadened cursor and eligibility; the message target reuses the same
  keyset paging, compare-and-swap write, batch budget, and cleanup-snapshot
  target on one pass.
- `internal/system/dataretention/policy.go`: message window initial value
  30 to 14 days; persisted record shape unchanged.
- `apps/web/src/locales/*/system.json`: the Data & Logs card copy now reads as
  finished-or-archived history retention; `zh-hk`/`zh-tw` regenerated with
  `pnpm run i18n:zh-hant`.
- `docs/public/operations.md`: new "Conversation history retention" section.

Verification run:

- `go test -trimpath ./internal/system/dataretention/... -count=1` -> ok (3 new
  tests: default window 14, broadened finished+archived pass retention of a live
  task, idempotent by marker).
- `go test -trimpath ./internal/task/repository/sqlite/ -run TestHistory -count=1`
  -> ok.
- `go test -trimpath ./internal/system/dataretention/... ./internal/task/repository/sqlite/... -count=1`
  -> dataretention ok; the sqlite package fails only on the pre-existing Windows
  time-precision tests (UpdatedAt comparison and optimistic-lock; unrelated to
  this change and failing on the base).
- `go build ./internal/system/dataretention/... ./internal/task/repository/sqlite/...`
  and `go vet` -> clean; `gofmt -l` -> clean.
- `pnpm run i18n:check`, `pnpm run typecheck`, `pnpm run lint` -> pass.
- `pnpm exec vitest run components/settings/system` -> 34 files, 309 tests pass.

## Isolated actual API readback

Run outside the operational home because `scripts/dev-isolated.ps1:111` treats
`%USERPROFILE%\.kandev` as an operational root and refuses the assigned worktree
(which lives under it). A throwaway worktree at the same commit was created at
`D:\260720_Ai_Workspace_Tools\kandev-wt-w099`, the instance was launched there
with the canonical script, and the worktree was removed afterward.

Commands (from `D:\260720_Ai_Workspace_Tools\kandev-wt-w099`):

```powershell
git worktree add --detach D:\260720_Ai_Workspace_Tools\kandev-wt-w099 feature/kandev-w-099-kandev-m4a
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\dev-isolated.ps1 -Timeout 150
# backend http://127.0.0.1:48429, KANDEV_HOME C:\Users\dream\.kandev-test-48429-c02d0c1e
```

Effective policy read from the real API (`GET /api/v1/system/database/archived-data-retention`)
before any save: `enabled=false, archived_age=14, cleanup_age=7, supported=true`.
This records the actual effective message window as 14 days; the package's
saved-record preservation is an operator-input distinction, not the outcome.

After seeding one finished, one archived, and one live task (a 15-day-inactive
CONVERSATION each with an 8 KB tool payload) via the isolated DB, the run was
driven entirely through the HTTP API:

```powershell
# PUT base {enabled:true, archived_age:{days:14}, cleanup_age:{days:7}, revision:0, backup_choice:"skip"}
# poll GET base until last_run is present
```

Result (`last_run`): `kind=cleanup, state=succeeded, complete=true`,
`messages: rows=2, eligible=2, reduced=2, bytes=15656, total_bytes=16190`,
`archived_cutoff=<now-14d>`, `cleanup_cutoff=<now-7d>`, `next_due_at=<+24h>`.

DB readback after the pass:

| message | before | after | `payload_retention` | payload present |
| --- | --- | --- | --- | --- |
| `m-fin` (finished) | 8095 B | 267 B | yes | no |
| `m-arch` (archived) | 8095 B | 267 B | yes | no |
| `m-live` (active) | 8095 B | 8095 B | no | yes |

The pass reduced the finished and archived conversations through the real API,
reported a stored-byte reduction, and left the active task untouched.

Teardown: `kandev-kill.ps1 -Pidfile <pidfile> -Yes` stopped the backend; the
`kandev-test-48429-...\AppData\Local\hermes\bin` entry the run added was removed
from the user PATH; `git worktree remove` returned the throwaway worktree.

This is the code-level/original outcome. The installed readback (an existing
saved record whose window is set to 14 days, then a pass on real data) is a
separate operator deployment input.
