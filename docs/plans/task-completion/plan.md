---
created: 2026-09-09
status: complete
requirements:
  - REQ-TASKS-COMPLETION-001
  - REQ-TASKS-COMPLETION-002
system_design:
  - ../../specs/tasks/system-design/task-completion.md
legacy_specs: []
---

# Implementation Plan: Task completion and conversation follow-ups

## Overview

Make workflow completion explicit and restore follow-ups in completed chats.
Deliver persistence, portable contracts, runtime completion, guarded resume,
workflow editing, then chat UI. Execute sequentially with `/tdd`; `/mobile-parity`
and focused `/e2e` apply to both UI work orders. No delegation is authorized.

The task system owns this package because it owns durable workflow and session
lifecycle contracts. The user supplied the intended behavior; it supersedes the
old name-based completion and permanent completed-chat lock.

UX refinement: show the completion checkbox only on the final step and put its
description behind an info icon. Completion requires final position plus the
enabled setting. Retain per-step values across reorder, with non-final values
inactive. Cover hover/focus on desktop and tap on mobile.

## Confirmed cause and reproduction

The reported Contributor PR Review child reached its final workflow step.
The remote task URL could not be opened in this environment. Local evidence:

- `workflow/models/terminal.go` requires no next step and a trimmed,
  case-insensitive Done/Complete/Completed/Approved name.
- `event_handlers_children_completed.go` writes task COMPLETED and uses
  inferred step membership for parent rollups.
- `setSessionWaitingForInputIfRequestedWithHook` in
  `event_handlers_streaming.go` collapses a terminal child without active
  clarification to COMPLETED. Root sessions take the waiting branch.
- `ResumeTaskSessionWithOptions` rejects COMPLETED. Its missing-executor guard
  also excludes completed sessions after resource reclamation.
- `SessionStoppedBanner` completed mode renders only New Agent. MCP target
  resolution excludes completed sessions from live targets; turn-start rejects
  them through the orchestrator's terminal guard.

Smallest existing reproduction: seed a completed child task with a session and
executor, emit `agent.completed`, then inspect the session. The paired root
test settles to WAITING_FOR_INPUT. Existing tests pass because they pin the old
behavior; permanent regression tests for the new contract begin in implementation.

## Scope

### In scope

- `complete_task_on_enter` across persistence, migration, APIs, portable formats,
  sync, built-ins, bootstrap, duplication, state delivery, and the editor.
- Root/child completion consistency and parent/dependency notification accuracy.
- Explicit same-session Resume, preserved history/ownership, ordered follow-ups,
  cleanup races, and desktop/mobile recovery UI.
- Owning specifications, compatibility amendments, ADR, and public docs at ship.

### Out of scope

- Changing FAILED/CANCELLED ordinary-message admission or their existing
  dedicated manual recovery behavior.
- Office scheduler ownership, archive/delete policy, provider conversation
  protocols, new queue types, or new workflow triggers.
- Mutating the remote reproduction task or deploying this change.

## Technical approach

Follow [the system design](../../specs/tasks/system-design/task-completion.md).
Add a non-null boolean column and a transactional, one-time legacy backfill.
Version-2 portable definitions carry explicit booleans; version-1 compatibility
conversion runs at import/sync only. Both schema constructors and every writer
must agree before runtime predicates change.

Use persisted task state for downstream completion. Retain ordinary completed
task conversations in a promptable state and keep fail-closed idle reclamation.
Permit old completed chats through explicit recovery with expected-state and
execution ownership checks. Keep automatic terminal guards. Retired conversations
retain a durable follow-up marker so resuming cannot claim active workflow work.

## Tests

The work orders name full paths and exact commands. New regression names below
are proposed tests, not tests already run.

| Acceptance criteria | Regression evidence |
| --- | --- |
| `001.5`, `001.6`, `001.7` | `completion_policy_test.go`: `TestCompletionPolicyMigration`, `TestCompletionPolicyReplayPreservesDisabled`, `TestCompletionPolicyMigrationRollback`; required-store fresh/replay/upgrade on both dialects |
| `001.4`, `001.7`, `001.8` | workflow `completion_portable_test.go`: `TestCompletionPortableVersions`, `TestCompletionSyncPreservesFalse`; controller, MCP, DTO and step-event round trips |
| `001.2`, `001.3`, `001.9`–`001.12` | orchestrator `workflow_completion_policy_test.go`: `TestCompletionPolicyEntryPaths`, `TestCompletionPolicyNotifications`; task-service move and dependency tests |
| `002.5`, `002.10`, `002.13` | updated `event_handlers_subtask_waiting_test.go` and `event_handlers_reclaim_wiring_test.go`, including active clarification and reclaim barriers |
| `002.2`–`002.4`, `002.6`–`002.13` | `completed_session_resume_test.go`: `TestCompletedSessionResumePreservesConversation`, `TestCompletedSessionResumeRaces`, `TestCompletedSessionFollowUpOwnership`; MCP dispatch and executor tests |
| `001.1`, `001.4`, `001.7`, `001.13` | workflow draft/create/duplicate/API/WS unit tests and workflow E2E |
| `002.1`–`002.4`, `002.7`, `002.9`, `002.11`, `002.12` | recovery action/input mode/resumption unit tests and completed-chat E2E |

Criterion suffixes refer to `AC-TASKS-COMPLETION`.

## E2E tests

- `tests/workflow/workflow-task-completion.spec.ts`, project `chromium`:
  toggle/save/reload, checked final step, unchecked final Done, rename/reorder,
  hidden controls on non-final steps, info-icon disclosure,
  and same-conversation follow-up. Covers `001.1`–`001.4`, `001.7`, `001.13`.
- `tests/workflow/mobile-workflow-task-completion.spec.ts`, `mobile-chrome`:
  tap the setting, save/reload, perform the same completed/unchecked flows,
  assert 44 px touch label, tap-to-open help, final-step-only visibility, and
  no horizontal overflow. Same criteria.
- `tests/session/completed-session-resume.spec.ts`, `chromium`:
  root and child histories, passive open/reload, Resume, second response in the
  same session, failed resume feedback, and non-primary historical ownership.
  Covers `002.1`–`002.5`, `002.9`, `002.12`.
- `tests/session/mobile-completed-session-resume.spec.ts`, `mobile-chrome`:
  Resume and send by touch, preserved history and task state, no overflow,
  visible New Agent, and safe-area/touch geometry. Same criteria.
- Run the neighbouring recovery, resume-queue, workflow-agent-switch, and
  child-completion specs named in work orders to protect existing semantics.

Seed with existing API fixtures, assert user outcomes through the UI, and use
causal waits. Rebuild production assets through the managed runner. Keep one
worker per shard, run desktop/mobile separately, and do not increase timeouts.

## Work orders

- [x] [Task 01: Persist completion settings](task-01-persistence.md) (done)
- [x] [Task 02: Carry portable completion settings](task-02-portable-contracts.md) (done)
- [x] [Task 03: Apply configured task completion](task-03-runtime-completion.md) (done)
- [x] [Task 04: Resume completed conversations safely](task-04-session-resume.md) (done)
- [x] [Task 05: Expose workflow completion settings](task-05-workflow-editor.md) (done)
- [x] [Task 06: Restore completed-chat interaction](task-06-chat-recovery.md) (done)

## Verification results

Design investigation on 2026-09-09, from `apps/backend`:

```bash
rtk go test ./internal/workflow/models -run TestIsTerminalStep -count=1
rtk go test ./internal/orchestrator -run 'TestHandleAgentCompleted_(SubtaskWithoutRequestsInputCollapsesToCompleted|NonTerminalSubtaskWithoutRequestsInputWritesWaiting|SubtaskWithRequestsInputStillWritesWaiting|SiblingSessionWithoutRequestsInputStillWritesWaiting)$' -count=1
```

Both passed: 23 model tests/subtests and 4 orchestrator tests reported by RTK.
These confirmed the pre-change behavior. Implementation checks are recorded in
the task work orders.

Design validation passed:

```bash
rtk python3 scripts/lint-spec-files.py --all
rtk python3 scripts/lint-spec-files.test.py
rtk git diff --check
```

All specification files passed; the specification-linter suite passed 30 tests.

Implementation verification passed:

```text
Task 01 persistence policy/migration gate: 8 tests
Task 01 required-store and conformance gate: 305 tests
Task 02 backend portable/API/sync gate: 535 tests
Task 02 MCP workflow/config gate: 172 tests
Task 02 SQLite workflow/bootstrap gate: 65 tests
Task 03 task-service completion gate: 159 tests
Task 03 orchestrator completion gate: 180 tests
Task 04 focused resume gates: 153 + 103 + 103 + 65 tests
Task 05/06 focused frontend gate: 75 tests
workflow/task backend package gate: 1,693 tests
desktop workflow E2E: 12 tests
mobile workflow E2E: 8 tests
desktop recovery-neighbor E2E: 23 tests
mobile recovery-neighbor E2E: 4 tests
desktop completed-chat E2E: 1 test
mobile completed-chat E2E: 1 test
public-doc validator tests: 61 tests
published-doc validation: 46 pages
specification lint and git diff whitespace checks: passed
```

## Risks

- Both repositories initialize `workflow_steps`; constructor order must not
  lose backfill values or reference missing columns.
- An unguarded replay backfill would overwrite saved false values.
- Version-2 exports require upgraded readers. Version-1 inputs remain supported.
- Terminal-state helpers serve different purposes. A global relaxation could
  permit old callbacks, implicit resume, credential access, or sibling takeover.
- Built-in Kanban currently reopens Done on turn-start. Follow-up dispatch must
  suppress this automatic action while preserving explicit task reopening.
- Historical profile sessions, resumed queues, and reclaim callbacks share
  ownership boundaries; race tests are required before UI exposure.
- PostgreSQL coverage requires `KANDEV_TEST_POSTGRES_DSN`; a skipped dialect is
  not passing migration evidence.
- Public docs describe shipped behavior until implementation updates them.
