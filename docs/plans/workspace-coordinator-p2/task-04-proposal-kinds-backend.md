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

- `kinds.go`: `KindExecutor` (six methods per the design: `Kind`, `Action`, `ValidatePropose`, `ValidateEdits`, `Execute`, `ReRunsOnStaleClaim`) with
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
  `ToolNames(policy, true)` for every policy, and the PERMISSIONS-001.4 walk of the
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

Interleaving table (written before code; each row is a test in
`interleavings_kinds_test.go`). Expected result for every row: `Execute` runs
at most once and the row settles exactly once.

| # | Order of operations | Expected result |
|---|---|---|
| 1 | Approve A and approve B of one pending non-create proposal run concurrently | One claim wins and runs `Execute` once; the loser gets 409 (row `approving`) or 200 with the settled row; executor stub records one call |
| 2 | Approve claims, `Execute` runs past the stale window, the sweep runs, then `Execute` returns | Sweep settles `failed` `outcome_unknown` without calling `Execute`; the late fenced completion matches zero rows, writes nothing, logs warn `execute_settle_fenced`; caller returns 200 with the failed row |
| 3 | Approve claims and `Execute` runs; the process stops before the settle; startup pass runs | Startup pass settles `failed` `outcome_unknown` without calling `Execute`; the executor stub records no second call |
| 4 | Approve claims; reject arrives while `approving` | Reject returns 409 (reject takes only pending or failed); the approve settles normally |
| 5 | Reject settles a `failed` row; a stale approve read of the same row claims afterwards | The claim is conditional on the status, so it matches zero rows and `Execute` does not run |
| 6 | `Execute` reaches its 60 s deadline | Settle runs on a detached context: `approved` if `Execute` returned nil, `failed` `outcome_unknown` if it returned a deadline error |
| 7 | Deadline fires, the row settles `outcome_unknown`, then the resume launch finishes | The launch goroutine owned by the resume attempt registry writes nothing and logs warn `execute_settle_fenced` |
| 8 | Move: the sweep settles the claim between the claim and the fenced `from_step_id` write | The fenced write matches zero rows, no move call is made, warn `execute_settle_fenced` |
| 9 | Approve of a stale non-create claim | Returns 200 with the `failed` `outcome_unknown` row; no `Execute` call; works with `features.coordinatorPhase2` off |

Conductor rulings that override the design prose:

- R4-1: `Execute` returns at the 60 s deadline. The resume launch runs in a
  goroutine owned by the resume attempt registry (tracked, never unowned), so
  approve never blocks for a cold launch. At the deadline the row settles
  `outcome_unknown`; the late launch result writes nothing (the fenced
  completion matches zero rows and logs warn `execute_settle_fenced`). The
  stale-claim sweep stays the backstop; there is no second sweep.
- R4-2: move check 6 reads every step of the target workflow through the seam
  (a list-steps read) and calls `StartsAgentOnEnter(steps, stepID)`.
- R4-3: `outcome_unknown` only means the side effect may have happened. A
  `from_step_id` pre-write error, before any move call, settles `failed` even
  when the deadline has fired.
- R4-4: a deleted target task (`ErrTaskNotFound`) maps to `task_archived`, the
  code undo uses.

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
move call. A task moved by hand onto a destination that
has since become a Done step settles `approved` with `noop: true`, not
`step_is_done`. A second identical propose after the target was archived
returns the open proposal. A retire committed before a citing propose makes
the propose refuse naming `standing_order_ids`. A move proposal stored with
`starts_agent` true, whose task was then moved by hand onto the destination,
is refused 409 `policy_denied` after `start_agent` flips to `denied`, with
no claim and no `MoveTask` call. A `starts_agent` true move proposal
approved while both `move` and `start_agent` are `denied` returns 409 with
`action` `move`; with only `start_agent` `denied` it returns `action`
`start_agent`. A resume propose refuses CREATED, COMPLETED and live-execution sessions
and an IDLE session with no executor record, and accepts a FAILED one
without a record. A move into a step that only disallows manual moves stores
`starts_agent` false. A second identical propose returns the open proposal
with `deduplicated: true`, and a `failed` proposal still blocks it. Approve
of an archived resume or move target settles `failed` (not 400); a `resume`
or `move` body carrying `text` or `title` is 400 `not_editable` after the
status decision; `text: null` is 400 naming `text`. An `Execute` that hits its
60-second deadline settles `outcome_unknown`; a late completion after a sweep
matches zero rows and writes nothing. Two identical `propose_task_kandev` calls
create two proposals, and a create call citing six ids, or one id twice, is
refused naming `standing_order_ids` before any transaction. The E2E spec drives the mock agent to propose a move and
asserts the task's step after approval through the task API.
Execute classification tests: a move whose fenced `from_step_id` write matches
zero rows makes no move call and returns the current row; a move-seam
`ErrWIPLimitExceeded` fails `step_full`, `ErrMoveConflict` fails `moved`, a
`admitted` false result settles `approved` with `queued: true`, a task moved to another workflow after check 2 fails `moved` (the call carries `ExpectedWorkflowID`), and a task
with a RUNNING session fails `agent_running`; a resume that returns `(nil,
nil)` settles `approved` with `deferred: true`, one that joins a concurrent
attempt settles `approved`, and `ErrResumeAttemptCancelled` fails; a message
fails `task_archived`, `not_accepting` or `queue_full` per cause; an Execute
that returns nil after its deadline fired settles `approved`, one that returns
an unwrapped error with the deadline fired settles `outcome_unknown`, and the
settle succeeds after the Execute context expired; approve of a stale
non-create claim returns 200 with the `failed` row; the sweep and startup pass
leave an unknown-kind `approving` row untouched and settle a non-create
`approving` row `outcome_unknown` with `features.coordinatorPhase2` off.

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
