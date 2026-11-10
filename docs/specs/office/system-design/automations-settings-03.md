---
status: current
system: office
requirements:
  - REQ-OFFICE-AUTOMATIONS-SETTINGS-001
created: 2026-11-10
owners:
  - kandev
---

# Automation schedule replacement

## Purpose and boundaries

Office owns saved automation triggers and their firing semantics. This design
extends the [automation settings contract](../requirements/automations-settings.md)
with a schedule replacement boundary. The [existing calendar design](automations-settings-01.md#what)
continues to own parsing, timezone resolution, DST behavior, and validation.

The boundary separates a replacement definition from the previous definition's
evaluation history. Run admission also rejects scheduler observations that an
edit makes obsolete. No API field, table column, migration, or runtime flag is
required.

## Requirement mapping

| Requirement | Acceptance criteria | Design sections |
| --- | --- | --- |
| `REQ-OFFICE-AUTOMATIONS-SETTINGS-001` | .20, .21, .22, .23, .24 | [Definition identity](#definition-identity), [Persistent boundary](#persistent-boundary) |
| `REQ-OFFICE-AUTOMATIONS-SETTINGS-001` | .25, .27 | [Admission ordering](#admission-ordering) |
| `REQ-OFFICE-AUTOMATIONS-SETTINGS-001` | .18, .26 | [Calendar and recovery](#calendar-and-recovery) |

## Definition identity

`ScheduledTriggerConfig` in `apps/backend/internal/automation/models.go` supplies
the public expression and timezone fields. Decode the old and supplied configs
before comparison. JSON member order, formatting, and unknown members do not
define a schedule.

Use this bounded comparison:

- Trim surrounding whitespace from the expression.
- Treat an empty timezone as `UTC`.
- Ignore the separate timezone field for `@every` expressions and expressions
  with a `TZ=` or `CRON_TZ=` prefix. The existing parser owns that precedence.
- Treat empty expressions as unscheduled, regardless of the timezone field.
- Otherwise, compare the normalized expression and effective timezone.

This comparison does not prove arbitrary cron equivalence. Changing `@daily`
to `0 0 * * *` establishes a replacement boundary. Timezone aliases are not
canonicalized. The comparison introduces no second cron parser.

An omitted update config cannot replace the schedule. A supplied config with
the same identity cannot replace it either. Enabled-state-only updates retain
the current boundary. Non-scheduled triggers retain their existing cursor
behavior. Repairing an unreadable old config with a valid supplied schedule
establishes a replacement boundary.

## Persistent boundary

`Store.UpdateTrigger` in `apps/backend/internal/automation/store.go` owns the
config write. When a scheduled definition changes, use one UTC edit timestamp
for `updated_at` and `last_evaluated_at`. Write both timestamps with config,
enabled state, and the existing revision increment in one SQL update.

The update must not temporarily expose the new definition with the old cursor.
On an unchanged definition, omit `last_evaluated_at` from the update. Do not
write back a cursor copied from the earlier read because an evaluation can
advance it between those operations.

`CronScheduler.shouldFire` keeps its existing anchor selection:
`LastEvaluatedAt`, or `CreatedAt` when the cursor is nil. Replacement saves set
the cursor even when its previous value was nil. Clearing a schedule records
the boundary but leaves it unscheduled. A later non-empty replacement records
its own boundary.

Do not anchor every schedule to `UpdatedAt`. That timestamp also moves for
no-op saves and enabled-state changes. Do not clear the cursor to nil because
that falls back to the trigger's older creation time.

`Service.UpdateTrigger` retains authorization and supplied-config validation
before persistence. Its existing `automationRunLock` serializes the write with
automatic run admission. Failed validation or a failed SQL statement leaves
the stored row intact. Keep SQL portable through the existing `Rebind` path.

## Admission ordering

The scheduler inventory is a snapshot. `CronScheduler.fire` currently passes
its trigger ID to `Service.FireTrigger` without a revision check. An obsolete
snapshot can therefore admit a run after a successful edit.

Introduce an internal automatic-schedule admission path carrying the observed
trigger ID, automation ID, `TriggerRevision`, and evaluation instant. The
public manual firing methods retain their current contract. Do not infer
automatic authority from the public `trigger_type` value alone.

Inside the existing per-automation admission mutex:

1. Reload the automation and trigger from storage.
2. Require the same trigger identity, scheduled type, and observed revision.
3. Require an enabled automation and trigger, with no `plugin_event` condition.
4. Recompute due status from the fresh config and cursor at the check instant.
5. Only then perform existing dedup, capacity checks, and run admission.

Use the existing calendar evaluator for the fresh due check. Extract a small
shared helper when necessary. A missing, changed, disabled, unscheduled, or
not-due trigger returns without history, dedup, event, or cursor writes.
Read errors abort the attempt and retain the existing error reporting.

Move the successful evaluation-cursor write inside the same admission mutex,
before releasing it. An admitted firing or a capacity skip advances the cursor.
A dedup result or admission error does not.
Preserve the current warning policy for a failed cursor write. A cursor write
from an older admission cannot occur after a replacement save.

Reuse `admitTriggerLocked` and its existing admission logic. Avoid nested
acquisition of `automationRunLock`. Release the mutex before event publication,
provider work, or any subscriber callback.

The lock defines ordering. When the edit wins, discard the old observation and
let the next tick load the current definition. When admission wins, its run
remains valid and the later save establishes the replacement boundary.

## Calendar and recovery

`nextCronFire` remains the only calendar engine. Five-field expressions,
descriptors, intervals, inline timezone precedence, UTC defaults, and DST rules
retain their current meaning. A replacement occurrence is strictly after the
edit timestamp, including edits exactly on a scheduled instant.

An unchanged schedule retains its pending occurrence. An occurrence missed
after a replacement boundary retains the current single catch-up behavior.
This design suppresses history before an edit and does not add a catch-up
control or change enabled-only recovery policy.

The stored boundary survives restart. No startup scan resets old rows. An edit
made before this fix has no distinct persisted replacement boundary, so the
backend cannot safely reconstruct its intent from `updated_at`.

`Store.UpdateManagedSchedule` already replaces managed trigger rows
transactionally with fresh IDs and creation timestamps. Preserve that separate
replacement lifecycle. Its deleted IDs must also fail the fresh admission
check. The plugin webhook gate and distinct Office routine scheduler retain
their existing contracts.

## Verification and documentation

Use the real service, SQLite store, cron evaluator, and event bus. A fixed Go
`testing/synctest` clock reproduces the reported Saturday-to-Monday edit without
real waiting. Cover both nil and historical cursors, daily/hourly schedules,
timezones and DST, no-op saves, disabled and empty schedules, and rejected edits.

Use deterministic barriers for edit/admission ordering. Assert run rows,
skipped rows, events, and the stored cursor. Pair stale-snapshot rejection with
a positive first-occurrence test and an unchanged-schedule catch-up test.
Use the repository's isolated PostgreSQL fixture for the timestamp update.

An in-process WebSocket-handler-to-scheduler integration supplies end-to-end
evidence for the public update action. No rendered UI changes, browser clock
hooks, or viewport-dependent behavior are required.

When implementation ships, update the Schedule and troubleshooting sections
of `docs/public/automation-and-mcp.md`. Explain the edit boundary beside the
existing creation and downtime behavior. No new public page is required.

Exact tests, files, and commands belong to the
[implementation package](../../../plans/automation-schedule-replacement/plan.md).
The requirement, design, and regression evidence preserve this local correction's
rationale. No separate architecture decision is required.
