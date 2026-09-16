---
created: 2026-09-16
status: done
requirements:
  - REQ-TASKS-PARENT-CHILD-MESSAGE-INTERRUPT-001
  - REQ-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001
system_design:
  - ../../specs/tasks/system-design/parent-child-message-interrupt.md
legacy_specs: []
---

# Implementation Plan: MCP Queued-message Wakeup

## Overview

Close the confirmed peer-message admission/readiness race reported in
[issue 3716](https://github.com/kdlbs/kandev/issues/3716). One sequential work order
wires the existing guarded readiness operation and verifies actual delivery.
The issue is assigned to `carlosflorencio`.

This package covers a confirmed contributor to stranded messages. It does not
claim to explain every symptom in the issue or authorize closing it as fully fixed.

## Evidence and root cause

Source inspected at `4c6e80091d`:

1. `internal/mcp/handlers/handlers.go:dispatchTaskMessage` selects the queue path
   from a previously read `RUNNING` or `STARTING` session snapshot.
2. `queueTaskMessage` persists through `QueueMessageWithMetadataForSession`,
   publishes queue status, and returns. It never requests readiness checking.
3. If the final ready event drains the empty queue before insertion, the now-idle
   target has no subsequent event that must consume this new entry.
4. `internal/orchestrator/service.go:CheckQueueAdmissionReadiness` already covers
   this ordering for browser admission. `QueueHandlers.wsQueueMessage` calls it;
   MCP ordinary admission does not.

A temporary `issue3716_probe_test.go` used the existing handler fixtures. It
captured a busy session, updated the authoritative database row to
`WAITING_FOR_INPUT`, then called `dispatchTaskMessage` with the stale snapshot.
The result was `queued`, one pending entry, no prompt call, and zero calls to a
readiness-capable launcher. The test expected one readiness call and failed there.
The probe was removed after diagnosis; no permanent tests or production code
were changed. This proves the missing wakeup seam, not a complete production
reproduction of the reporter's timeout.

## Scope

### In scope

- Ordinary MCP peer-message admission after a busy snapshot becomes stale.
- The same admission path reached through `dispatchPreparedTaskMessage`.
- Required MCP-to-orchestrator wiring with the captured session incarnation.
- FIFO, sender metadata, Auto-run, and existing lifecycle guards.
- Handler-to-provider acceptance evidence using controlled test collaborators.

### Out of scope and unresolved issue evidence

- The reported 30-second transport timeout. There is no reporter trace or
  reproduction here proving which operation blocked. Status snapshot/publication
  occurs after persistence, but that ordering alone does not prove the timeout.
- Deferred-move delivery, same-step semantics, transition-ledger interpretation,
  zombie turns, and the `workflow_id` schema observation in the comments.
  Current immediate moves use `MoveTaskWithOptions` and entry overlays; they
  must not receive an ordinary-message wakeup before their transition commits.
- New admission receipts, blind retry, startup scans, and historical data repair.
- UI, translations, public parameters, schemas, and new background workers.

## Requirement conformance

The implementation violates existing behavior; no new requirement is needed.
Reuse `AC-TASKS-PARENT-CHILD-MESSAGE-INTERRUPT-001.1` and its queue/turn safety
contract. Preserve `AC-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001.3`, `.4`, and `.6`
for automatic FIFO dispatch and durable paused policy.

The assumption check found no material product choice: unrelated senders may
queue but may not interrupt; Auto-run does not bypass lifecycle barriers.
The new task-system design documents the missing producer integration.

## Technical approach

Add `CheckQueueAdmissionReadiness(context.Context, messagequeue.QueueSessionIdentity)`
to the MCP `SessionLauncher` interface in `handlers.go`. Update its test doubles
and any required production adapter, discovered through compile checks. The
production orchestrator already implements the method.

After successful ordinary queue insertion, invoke the checker outside admission
locks and before best-effort status publication. Pass the existing captured
identity; do not re-resolve the primary session or use the manual drain API.
Keep existing error behavior before admission and the accepted `queued` response
after admission. Keep explicit interrupt on its existing atomic operation.

Reuse `CheckQueueAdmissionReadiness` and its automatic reservation rather than
duplicating eligibility logic. Do not change deferred-move helpers. Do not add
an optional type assertion that silently skips production readiness wiring.

## Tests

Add `internal/mcp/handlers/message_task_readiness_test.go` with these cases:

- `TestMessageTaskReadiness_ReadyBeforeAdmission`: stale busy snapshot, real
  idle row, successful admission and exactly one readiness request for its identity.
- `TestMessageTaskReadiness_PreparedBusySession`: the prepared-session queue
  branch uses the same check.
- `TestMessageTaskReadiness_RejectedAdmission`: capacity or identity rejection
  neither checks readiness nor changes accepted work.
- `TestMessageTaskReadiness_UnrelatedSender`: the full handler accepts queued
  mode and never invokes interrupt; explicit interrupt remains forbidden.
- `TestMessageTaskReadiness_Delivery`: real queue and orchestrator with an
  accepting fake executor; exactly one accepted prompt and preserved attribution.

Extend `internal/orchestrator/resume_prompt_queue_test.go` for
ready-before-insert, insert-before-ready, concurrent ready/admission checks,
Auto-run OFF, a pending clarification, WIP wait, and incarnation replacement.
Assert provider acceptance and pending-entry ownership, not only queue count.
Use channel barriers, not sleeps. Keep older FIFO work ahead of new work.

These cases cover the peer-message criterion and Auto-run `.3`/`.4`/`.6` above.
Existing browser readiness tests are a dependency, not a second implementation.

## End-to-end evidence

The backend handler-to-orchestrator-to-fake-executor delivery case proves the
agent-facing flow. It must enter through `handleMessageTask`, control the stale
read boundary, use authoritative session storage, and observe prompt acceptance.
No rendered UI changes or artificial browser scenario are required.

## Work orders

- [x] [Task 01: Wake eligible MCP queue admissions](task-01-wake-mcp-queue.md) (done)

## Verification results

Diagnosis command, from `apps/backend`:

```bash
go test ./internal/mcp/handlers -run '^TestIssue3716StaleBusySnapshotRequestsReadiness$' -count=1
```

Expected failure reproduced: readiness calls were `0`, expected `1`. The temporary
probe was then removed. The permanent regression test now passes after the
required readiness call was added.

Implementation and verification checks passed:

```text
(cd apps/backend && go test ./internal/mcp/handlers -run 'TestMessageTaskReadiness_' -count=1)
(cd apps/backend && go test -race ./internal/mcp/handlers ./internal/orchestrator ./internal/orchestrator/handlers ./internal/orchestrator/messagequeue -count=1)
(cd apps/backend && go test ./internal/mcp/... -run '^$')
(cd apps/backend && make build)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The race run passed all four packages. The backend build passed; its only
output of note was the existing warning that macOS artifacts were left
unsigned because no codesign tool was installed. Catalog validation covered
281 decisions and 970 specifications. Assignment was verified through
`gh issue view`.

Package checks passed: `python3 scripts/list-docs.py validate` (281 decisions,
970 specifications), `python3 scripts/lint-spec-files.py --all`, and
`git diff --check`. Assignment was verified through `gh issue view`.

## Risks

- Running a manual drain would silently enable paused queues.
- Checking a newly resolved identity could dispatch a replacement session.
- A test that only observes queue removal could mistake failed dispatch for delivery.
- A synchronous checker must not wait for the new turn to complete.
- This narrow repair cannot justify marking the timeout or move reports resolved.

## Documentation impact

Internal design and plan only. Public documentation remains unchanged because
this package restores an existing delivery contract and introduces no public API.
