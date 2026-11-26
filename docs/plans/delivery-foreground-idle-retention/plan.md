---
created: 2026-11-24
status: implemented
requirements:
  - REQ-PLATFORM-DELIVERY-RETENTION-001
  - REQ-PLATFORM-DELIVERY-RETENTION-002
  - REQ-PLATFORM-DELIVERY-RETENTION-005
  - REQ-PLATFORM-DELIVERY-RETENTION-006
system_design:
  - ../../specs/platform/system-design/backend-agent-delivery-retention.md
  - ../../specs/platform/system-design/delivery-stream-storage.md
legacy_specs: []
---

# Fix plan for foreground idle receipt retirement

## Overview

Allow expired delivery receipts to retire through a projected
`foreground_idle` when each row in the prefix passes the existing proofs.
Normal retention batches then reclaim eligible historical receipts.
[GitHub issue #5477](https://github.com/kdlbs/kandev/issues/5477).

The SQL candidate filter and Go payload validator both omit
`foreground_idle`. The prefix loop stops at that row even when the later
output already has a compact receipt. This violates
`AC-PLATFORM-DELIVERY-RETENTION-005.7`.
[Retention requirements](../../specs/platform/requirements/backend-agent-delivery-retention.md),
[candidate filter](../../../apps/backend/internal/task/repository/sqlite/agent_delivery_retention_candidates.go),
[prefix loop](../../../apps/backend/internal/task/repository/sqlite/agent_delivery_retention_events.go).

## Evidence

The temporary test `TestIssue5477ControlPrefixRepro` used the real SQLite
repository on main at `4b7c8fb6c50`. It projected three events with completed
generic effects. The middle event was the control under test. The last event
committed a canonical message. At two hours the retention batch pruned the
last event's payload.

- With `turn_started` in the middle, preview counted three eligible receipts
  at 26 hours. The batch advanced the retired cursor to 3 and removed all
  three inbox rows and generic effects.
- With `foreground_idle` in the middle, preview counted one eligible
  receipt. The batch advanced the retired cursor to 1 of 3 and left two
  inbox rows and two generic effects.
- With `response_attempt_reset` in the middle, preview and retention stopped
  at the same boundary. The stream storage design explicitly protects that event.
  [Projected control policy](../../specs/platform/system-design/delivery-stream-storage.md#projected-control-notifications).

The fixture used an empty submission ID to isolate the type check. The
temporary repro used a Go overlay.

## Technical approach

Add `foreground_idle` to the SQL control allowlist. Add
`streams.EventTypeForegroundIdle` to the Go control case that validates
decoded payloads. Both entries place the event under the existing rules for
nonterminal controls.
[Storage design](../../specs/platform/system-design/delivery-stream-storage.md#foreground-idle-notifications).

Handoff recovery uses the original submission and its actual terminal event.
The idle notification can release the live waiter. The completed generic
receipt already suppresses its callback on replay.
[Handoff recovery design](../../specs/platform/system-design/session-recovery-convergence.md#foreground-handoff-and-unproven-sql-completion).

## Scope boundaries

- `response_attempt_reset` is a protected event. Its generic receipt records
  projection. Retirement requires durable proof of downstream transcript
  retractions. [Response attempt recovery](../../specs/platform/system-design/provider-response-attempt-recovery.md#durable-and-live-cleanup).
- Normal cleanup frees database pages for reuse. Physical reclamation follows
  the existing maintenance contract. Writer scheduling has its own fix scope.
  [Writer scheduling issue #5400](https://github.com/kdlbs/kandev/issues/5400).

## Tests

Put `foreground_idle` before the final output event in the existing
`seedRetentionControls` fixture. `TestDeliveryRetentionControlPrefix` checks
preview, retirement and canonical history for `AC-PLATFORM-DELIVERY-RETENTION-005.7`.
The same fixture feeds the existing replay, protection and restart tests.

## Work orders

- [x] [Task 01. Retire projected foreground idle receipts](task-01-retire-foreground-idle.md)

## Verification results

Investigation reproduced the type barrier on SQLite. The permanent fixture
regression failed before the allowlist edits: preview counted 9 of 11 receipts.
After the SQL and Go allowlist updates, the retention control suite passed with
`-race` on SQLite and PostgreSQL using an isolated PostgreSQL test schema.

`go run ./cmd/sqlguard ./internal`, the persistence store conformance race
suite on both engines, changed-file Go lint, both specification validators,
`make build`, and `git diff --check` passed. The build reported that
`codesign`/`rcodesign` are unavailable, so the macOS helper binaries remain
unsigned.

## Risks

An unresolved earlier row still stops the prefix. The backlog can drain only
where each row passes the retention proofs and the worker has enough capacity.
Logical row counts measure cleanup progress. Physical file size and writer
latency require independent measurements.
