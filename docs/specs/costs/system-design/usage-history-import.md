---
status: draft
system: costs
requirements:
  - REQ-COSTS-USAGE-IMPORT-001
  - REQ-COSTS-USAGE-IMPORT-002
  - REQ-COSTS-USAGE-IMPORT-003
  - REQ-COSTS-USAGE-IMPORT-004
  - REQ-COSTS-USAGE-IMPORT-005
---

# Usage History Import System Design

## Purpose and boundaries

This design extends [Retained Usage History](usage-history.md).
Core owns which existing sessions become eligible. Session Cost owns tokscale reads, the import worker, and the settings card.
No SDK addition is required: the plugin uses the existing `ListTargets`, `CurrentTarget`, `Report`, and plugin state APIs.
The [source adoption decision](../../../decisions/2026-11-01-usage-source-adoption.md) records why adoption exists and what it may trust.

| Requirement | Section |
| --- | --- |
| REQ-COSTS-USAGE-IMPORT-001 | Core source adoption |
| REQ-COSTS-USAGE-IMPORT-002 | Import worker |
| REQ-COSTS-USAGE-IMPORT-003 | Status contract, Settings card |
| REQ-COSTS-USAGE-IMPORT-004 | Worker lifecycle |
| REQ-COSTS-USAGE-IMPORT-005 | Per-session isolation, Plugin messaging when retained usage is unavailable |

## Core source adoption

`refreshUsageSourceContextTx` marks `usage_collection_contexts.trusted` only when `CaptureUsageCollectionSessionTx` runs at session creation.
Existing sessions therefore stay `unknown`.

Add `BackfillUsageSourceAdoption(ctx, limit)` to `analytics/repository/sqlite` as a new final step of `BackfillUsageHistory`, so `UsageHistoryWorker` runs it without a new goroutine.
Its cursor is a new `usage_history_migration_state` row with `source_key='source-adoption'`. It pages `task_sessions` by `id` and commits each page with its cursor. Once `complete=1`, it does no further work.

For each session without a trusted context, in one transaction:

1. Stop for this session if `ensureUsageScopeWritableTx` reports an erasing or erased fence.
2. Call `bindExternalUsageScopeTx` to create the scope, subject, and binding.
3. Skip if `ExternalSessionID` is empty, `Forked` is set, `usageSourceScopeUnsafeTx` is true, or `task_transfer_audit` has a row for the task.
4. Set `trusted=1` and `evidence='migration'` on the context, then call `captureCollectionTargetTx` to publish the eligible binding.

Add `usage_collection_contexts.evidence` with values `creation` and `migration`. Normal creation capture writes `creation`.
Discovery and `currentUsageTargetTx` never set `trusted`. Actor fields continue to come only from `captureTargetActorTx`, which reads execution bindings.

`prepareUsageCollectionTargetsTx` returns not ready until the `source-adoption` cursor is complete. Snapshot discovery therefore reports `Preparing=true` during adoption, and no import reads a half-adopted workspace.
Current-target lookups do not wait. During the short adoption window an unadopted session is reported as `unknown`, and the plugin treats it as skipped.

## Import worker

Replace the page-by-page loop in `server/import.go`. A run has three phases, all persisted in the checkpoint.

**`scanning` and all-time step.** Walk every snapshot page through `readUsageTargetPage`, then build an in-memory index of eligible targets keyed by `SubjectBinding`. Source rows are matched to each target with `sessionMatches` on the target's `ExternalSessionID`, so two eligible sessions that share one transcript inside a scope keep separate ranges and writes.
Run one `runReport`. For each target, select rows with the existing `sessionMatches` rule and call `persistTargetUsage` with empty `usageCoverage`.
Count found, imported, missing, and skipped by `SourceScopeStatus`. Merge each eligible target's import date range into a sorted, non-overlapping interval list.
The import range is the trusted range when both bounds are known. When a start is known and the session has no end, the import range runs from that start to today, so an open eligible session still receives daily history. A target with no known start contributes only lifetime usage. The extension exists only in the import index; `trustedTargetDateRange` keeps its two-bounds rule for current-target callers.

**`daily` step.** Walk the merged intervals one day at a time. Run `runDatedReport(day, day)` once, and match its rows against the index. For each matched target whose range includes the day, call `currentSourceTarget` with the target's binding and then `persistTargetUsage` with that day's coverage. A row with no match in the index is ignored. A matched target with no row for the day is not visited, so a day with no rows writes nothing.

**`completed`.** Save counts and finish the run.

The index is rebuilt after a restart, because the immutable snapshot makes the rebuild deterministic. It holds only target identifiers and date bounds.
`validateUsageSnapshot` runs before each report. `currentSourceTarget` runs before each write. An `errUsageSourceUnavailable` or `errUsageScopeChanged` result reclassifies that target as skipped and removes it from the index. A denied snapshot still discards the checkpoint.

`waitImportBudget` waits the previous report's duration, clamped to 5 to 60 seconds. Reports continue to serialize with composer refreshes and collection in `reportCoordinator`, whose `reportTimeout` remains 120 seconds.

### Checkpoint

Replace `historicalImportState` with schema version 3, stored in the existing `workspace` state scope:

- `RunID`, `Status`, `Phase`, `SnapshotID`, `SnapshotUpperBound`, `ScanCursor`.
- `Intervals`, `NextDay`, `DaysTotal`, `DaysDone`, `DayAttempts`, `DaySecondsTotal`.
- `SessionsFound`, `SessionsImported`, `SessionsMissing`, `SessionsSkipped`.
- `StartedAt`, `FinishedAt`, `LastSuccessfulAt`, `LastError`.

Import was never released, so remove `LegacyDiagnostics`, `PageTargets`, `PageDates`, `PageFound`, and their handling. An unreadable or older checkpoint is treated as not started.
The checkpoint holds no usage amounts and no transcript content. `usage_state_privacy.go` keeps enforcing that boundary.

## Worker lifecycle

One worker per plugin process runs one import at a time, because tokscale reads the same host transcripts whichever workspace asks.
Add a plugin-scope `import-queue` state value containing workspace IDs in start order.

| Trigger | Result |
| --- | --- |
| Start, no active import | Append to queue and start the worker |
| Start, another workspace active | Append to queue with status `queued` |
| Start, same workspace active | Return current status |
| Cancel | Set `cancelled`, remove from queue, and cancel the context only if this workspace is active |
| Plugin start (`SetHost`) | After state cleanup, start the worker if the queue is not empty |
| `Close()` | Set an internal `shuttingDown` flag and cancel the context. Do not write a terminal status |
| Disabled collection | Set `disabled`, keep position, and remove from queue. Starting again resumes |
| Worker finishes a run | Remove from queue and start the next entry |

Replace `importCancel` and `importWorkspace` with one active-run record holding workspace, run ID, and cancel function.
Every exit from the run function passes through one deferred `settle(result)`. It writes `failed` when no terminal status has been written, except during shutdown. This removes the silent returns in the current loop.
A state write failure retries `settle` on a short background context, then logs the error. A running status left by a crashed process is resolved by the restart rule above.
A failed config read is not a disabled collection setting. The worker waits and retries it; only an observed `false` stops the run with `disabled`.
`DayAttempts` resets after a successful day. Three consecutive failures produce `failed`, with position kept. Start on a `failed` or `disabled` checkpoint resumes; Start on `completed` or `cancelled` begins a new run.

### Shared blocking work

The plugin worker is the only caller that owns an import report. Its context is the active-run context. If a composer refresh shares the report in `reportCoordinator`, cancelling the import leaves the report running while another waiter exists, and the composer proceeds. If the import is the last waiter, the existing `wait` logic cancels the process.
Verify this boundary with a test that cancels the import while a composer waiter is blocked on the same scope, and asserts the composer still receives the result.

## Status contract

`historicalImportView` carries only what the card renders:

`workspace_id`, `status`, `phase`, `sessions_found`, `sessions_imported`, `sessions_missing`, `sessions_skipped`, `days_total`, `days_done`, `current_day`, `eta_seconds`, `last_successful_at`, `undated`, `last_error`.

`status` is one of `not_started`, `preparing`, `queued`, `running`, `completed`, `failed`, `cancelled`, `disabled`, `unsupported`.
`unsupported` now means only that the Host lacks retained usage. It is never used for an ineligible session.
`eta_seconds` is omitted until `DaysDone` in this run reaches 3. It is the average of report plus pause time multiplied by the remaining days.
`last_error` stays the existing fixed public message.
A status read never runs target discovery or creates a snapshot. A checkpoint without a snapshot returns the current state; the verified action context already proves workspace access, and only the worker captures membership.

## Settings card

Keep `makeHistoricalImportSettings` and its 2-second poll in `ui/bundle.js`. Poll while the status is `preparing`, `queued`, or `running`.
Use `host.ui.Progress` with `value` and `max` during the daily step, and no value during earlier phases.
The action button is Import, Cancel while active or queued, Resume after `failed` or `disabled`, and Import again after `completed` or `cancelled`.
The layout is one column on every width. Actions keep the existing `min-h-11` touch target, and status text uses `aria-live="polite"`.
New copy goes in `IMPORT_TRANSLATIONS`, which covers the plugin's `en` and `pt-pt` catalogs.

## Per-session isolation

Add a helper that classifies `resolveUsageTarget` errors: `errUsageSourceUnavailable` and `errUsageScopeChanged` are per-session, and everything else aborts the pass.

In `collectOnce`, a per-session result marks the pending entry `Skipped`, keeps it for rechecking while the session is active, and excludes it from `eligible`. A terminal skipped session is removed from `Pending`. If no session is eligible, no report runs.
Remove pending entries that have no `SubjectBinding` and no live session, which currently stay in the checkpoint forever.

In `unavailableSource`, add `ReportError="source_ineligible"` for those two errors. The composer maps it to the new safe-attribution copy.
Live calculation for eligible existing sessions needs no change beyond core adoption.

## Native usage on existing sessions

`BackfillUsageHistory` runs `BackfillUsageSourceAdoption` before any source page. Native backfill needs an eligible binding, so adoption must finish first, or eligible sessions record a permanent gap.
A successful native retention deletes that event's `usage_history_migration_gaps` row under both record-key forms: `usage_event_id` and the ledger row ID.

Deletion of an event that stays ineligible is allowed only when the `source-adoption` cursor is complete and a native gap row exists for the event. `preserveNativeUsageRowTx` and `requireNativeUsagePreservedTx` share that check. Before adoption completes, they keep failing with `unpreservedNativeUsageError`, so deletion stays retryable. See AC-COSTS-USAGE-HISTORY-007.11 and 007.12.

## Plugin messaging when retained usage is unavailable

`usageUpgrade` and `importUnsupported` name both remedies, because a current Host with `features.usageHistory` off returns the same `Unimplemented` result as an older Host. Live cost is not offered in this state, because `authorizeSessionUsageActionScope` needs the retained API. Plugin 0.5.0 must therefore not be published until the toggle is on in `prod`.

## Failure and security

- Import writes use `Report`, so existing scope, ownership, and idempotency checks still apply.
- An ineligible session's `ExternalSessionID` is already withheld by `retainedTarget.dto`, so its transcript cannot be matched.
- Adoption's transfer rule relies on `task_transfer_audit`. A cross-organization workspace move that left no audit row cannot be detected. In that case `usageSourceScopeUnsafeTx` still blocks a transcript shared across scopes, and the adopted session keeps unknown actor attribution.

## Observability

Add these registered counters beside the other usage history metrics: `usage_source_adoption_total`, labelled by the closed set `adopted`, `no_transcript`, `forked`, `unsafe`, `transferred`. No task, session, or workspace identifier is a label.
The plugin logs phase changes and counts, without session or transcript identifiers.

## Related decisions

- [Usage source adoption](../../../decisions/2026-11-01-usage-source-adoption.md)
- [Retained usage history](../../../decisions/2026-10-28-retained-usage-history.md)
