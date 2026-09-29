---
id: "04-proposal-kinds-backend"
title: "Resume, message and move proposals backend"
status: pending
wave: 3
depends_on:
  - "02-policy-enforcement"
  - "03-activity-log-backend"
  - "05-standing-orders-backend"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-PROPOSAL-KINDS-001
  - REQ-COORDINATOR-PROPOSAL-KINDS-002
  - REQ-COORDINATOR-PROPOSAL-KINDS-003
  - REQ-COORDINATOR-STANDING-ORDERS-003
acceptance_criteria:
  - AC-COORDINATOR-PROPOSAL-KINDS-001.1
  - AC-COORDINATOR-PROPOSAL-KINDS-001.2
  - AC-COORDINATOR-PROPOSAL-KINDS-001.3
  - AC-COORDINATOR-PROPOSAL-KINDS-001.4
  - AC-COORDINATOR-PROPOSAL-KINDS-001.5
  - AC-COORDINATOR-PROPOSAL-KINDS-002.1
  - AC-COORDINATOR-PROPOSAL-KINDS-002.2
  - AC-COORDINATOR-PROPOSAL-KINDS-002.3
  - AC-COORDINATOR-PROPOSAL-KINDS-003.1
  - AC-COORDINATOR-PROPOSAL-KINDS-003.2
  - AC-COORDINATOR-PROPOSAL-KINDS-003.3
  - AC-COORDINATOR-PROPOSAL-KINDS-003.4
  - AC-COORDINATOR-PROPOSAL-KINDS-003.5
  - AC-COORDINATOR-STANDING-ORDERS-003.1
system_design:
  - ../../specs/coordinator/system-design/proposal-kinds.md
  - ../../specs/coordinator/system-design/standing-orders.md
---

# Task 04: Resume, Message and Move Proposals Backend (WP-8)

## Summary

Add the three new proposal kinds end to end on the server: propose tools
with their validation and dedupe, a `KindExecutor` registry that the phase-1
approve route dispatches through, message delivery that shares one path with
`message_task_kandev`, the at-most-once stale-claim branch, agent-starting
creates, and `standing_order_ids` on every propose tool.

## In scope

- `kinds.go`: `KindExecutor` (`Validate`, `Execute`, `Editable`) with
  `create_task` wrapping the phase-1 path, and `resume`, `message`, `move`
  ([design](../../specs/coordinator/system-design/proposal-kinds.md#executors)).
- `propose_resume_kandev`, `propose_message_kandev`, `propose_move_kandev`
  and their MCP actions, registered by task 02's `ToolNames` (`001.1`).
- Target checks: watched, unarchived, same workspace, not a conversation
  task (`001.2`); step checks for move, with `starts_agent` stored from the
  destination's eligibility at propose (`001.3`); session checks
  (`001.4`); dedupe on `coordinator_proposals_open_target` returning the
  open proposal, read first, before target validation, so a repeat call
  returns it even after the target stopped validating, and repeated under
  the lock (`001.5`).
- The end-state catalog test that the registered coordinator tools equal
  `ToolNames(policy, true)` for every policy, and the `001.4` walk of the
  `KindExecutor` registry asserting no kind merges or targets a
  `CompleteTaskOnEnter` step. The three propose tools' `ToolForAction` rows
  and handlers are registered here.
- `start_agent` for creates: `EligibleStep` widened only while
  `requires_approval`, `starts_agent` stored, `auto_start_on_create` marker
  on approval, and the approve-time 409 `policy_denied` (`002.1` to `002.3`).
- Approval: resume through `ResumeTaskSession`, message through the
  extracted `TaskMessenger` (queued delivery, refusing CREATED, FAILED,
  CANCELLED or no session), move with `outcome_json.from_step_id` recorded
  before the move, the Execute checks in the design's stated order,
  `step_starts_agent` when the destination became
  agent-starting after a `starts_agent` false propose, and a `noop: true`
  outcome with no call when the task already sits on the destination
  (`003.1` to `003.3`); `not_editable` for any edit except
  message text (`003.4`).
- Stale claim of a non-create kind settles `failed` with
  `outcome_unknown` and never re-runs; Approve on a failed card creates a new
  claim as a new approval (`003.5`).
- `standing_order_ids` on all four propose tools: at most 5 and unique
  before the transaction; active orders of the caller checked inside the
  locked propose transaction; stored on the proposal; when the list is
  non-empty, task 05's `MarkApplied(tx, orderIDs, createdAt)` called in the
  same transaction right after the insert
  (`STANDING-ORDERS-003.1`, [design](../../specs/coordinator/system-design/standing-orders.md#last-applied)).
- Activity rows for every change of these kinds through task 03's writer.

## Out of scope

- Cards, Edit dialog and copy (task 09).
- Direct stall Resume and Send it back from the web (task 09, which calls
  existing session routes).
- Stop proposals (not in phase 2).

## Acceptance

- Each kind approves once, settles `approved` or `failed` with a reason, and
  leaves one activity row per change.
- `message_task_kandev` and message approval share one delivery function.
- No path merges a pull request or moves a task to a Done step.

## Verification

Write the interleaving table first, before code: two concurrent approves of
one proposal; the stale-claim sweep during an execution; a crash after the
execution and before the settle; a reject racing an approve. Each row names
the order of the operations and the expected result: one execution at most.

```bash
make -C apps/backend test PKG=./internal/coordinator/...
make -C apps/backend test PKG=./internal/mcp/...
make -C apps/backend test PKG=./internal/orchestrator/...
cd apps/web && pnpm e2e:run tests/coordinator/proposal-kinds-backend.spec.ts
```

Tests: a table per tool over each refusal naming its field; 10 concurrent
identical proposes leave one row; approve against an archived target settles
`failed`; a message to a WAITING session prompts with auto-resume and to a
RUNNING one queues, and to CREATED is refused; a claim stale past its window
settles `failed` with `outcome_unknown` and the executor stub records zero
second calls; `starts_agent` create approved after `start_agent` flips to
`denied` is 409; a move whose destination turned agent-starting after
propose settles `failed` with `step_starts_agent` and does not move; a move
onto the task's current step settles `approved` with `noop: true` and no
`MoveTask` call. A task moved by hand onto a destination that
has since become a Done step settles `approved` with `noop: true`, not
`step_is_done`. A second identical propose after the target was archived
returns the open proposal. A retire committed before a citing propose makes
the propose refuse naming `standing_order_ids`. A move proposal stored with
`starts_agent` true, whose task was then moved by hand onto the destination,
is refused 409 `policy_denied` after `start_agent` flips to `denied`, with
no claim and no `MoveTask` call. A `starts_agent` true move proposal
approved while both `move` and `start_agent` are `denied` returns 409 with
`action` `move`; with only `start_agent` `denied` it returns `action`
`start_agent`. Two identical `propose_task_kandev` calls
create two proposals, and a create call citing six ids, or one id twice, is
refused naming `standing_order_ids` before any transaction. The E2E spec drives the mock agent to propose a move and
asserts the task's step after approval through the task API.

## Likely files

- `apps/backend/internal/coordinator/kinds.go`, `kind_resume.go`,
  `kind_message.go`, `kind_move.go`, `proposals.go`, `eligibility.go`,
  `recovery.go`
- `apps/backend/internal/mcp/handlers/handlers.go` (`TaskMessenger`
  extraction from `handleMessageTask`)
- `apps/backend/internal/mcp/server/coordinator_tools.go`
- `apps/backend/internal/orchestrator/task_operations.go` (read only unless
  a seam is needed)

## Dependencies

- Task 02 (guard, bound tool list); task 03 (activity writer, move undo
  reads the recorded `from_step_id`); task 05 (`MarkApplied` helper).

## Risks

- Extracting `TaskMessenger` must keep `message_task_kandev` behaviour
  byte-for-byte; its existing handler tests run unchanged.
- An executor that runs but whose settle fails must not re-run; the claim is
  the at-most-once record.
