---
created: 2026-11-07
status: completed
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001
system_design:
  - ../../specs/tasks/system-design/attach-workspace-sources.md
  - ../../specs/tasks/system-design/workspace-source-script-turns.md
legacy_specs: []
---

# Implementation plan: Prevent workspace setup script deadlocks

## Overview

Restore repository attachment when a started Worktree task has no active turn and the added repository has a setup script.
Bind script messages to an existing turn or a completed lifecycle turn before message persistence.
Preserve the task mutex that serializes workspace changes and real conversational turns.

[Issue #5151](https://github.com/kdlbs/kandev/issues/5151) is assigned to `carlosflorencio`.
The implementation order was the backend correction, then desktop and phone regression coverage.
Both work orders are complete.

## Confirmed root cause

Investigation used repository HEAD `52309e613fc9a76d38a9f891b74b4d8b32bff265`.
`git ls-remote origin refs/heads/main` returned that same revision on 2026-11-07.
The issue had no attachments or comments at investigation time.

`AttachWorkspaceSources` holds `lockWorkspaceSources(taskID)` while `commitWorkspaceSourceBatch` calls the runtime materializer.
The materializer creates the worktree and synchronously runs its repository setup script.
`DefaultScriptMessageHandler.createScriptMessage` first requests a `script_execution` message without `TurnID`.
`backendapp.taskServiceAdapter.CreateMessage` forwards the empty ID to the ordinary message path.

With no active turn, `getOrStartTurn` calls `StartTurn`.
`createTurn` attempts to acquire the same non-reentrant task mutex. The caller already owns that mutex and cannot return to release it.
The script process never starts. A canceled script context cannot interrupt `sync.Mutex.Lock`.

`AddBranchToTask` holds the same mutex during worktree creation. Its idle path has the same confirmed failure.
With an existing active turn, message creation reuses that turn and avoids this particular deadlock.

## Reproduction and feasibility evidence

A temporary `TestReproIssue5151` used existing backend fixtures with real SQLite and local Git worktrees.
It wired the production worktree script handler, repository provider, and task-service adapter.
The second repository used `echo issue5151-setup-ok`. The task had a ready environment and a waiting session.

From `apps/backend`, these diagnostic commands ran:

```bash
go test -trimpath ./internal/backendapp -run '^TestReproIssue5151$' -count=1 -timeout=60s
go test -trimpath ./internal/backendapp -run '^TestReproIssue5151/add_branch$' -count=1 -timeout=30s
```

Both idle operations remained blocked after a 3-second guard, despite a 100ms script timeout.
Separate stack captures showed each caller beneath `runWorktreeSetupScript`, `CreateMessage`, `createTurn`, and `lockWorkspaceSources`.
These were expected diagnostic failures. No live user instance was modified.

A temporary Go source overlay applied only the proposed adapter policy.
The diagnostic then passed three cases: idle batch attachment, idle legacy add-branch, and legacy add-branch with an active turn.
Each case verified captured script output and `status: exited`.
Idle cases verified no active turn remained and that `StartTurn` accepted the next user turn.
The active case verified that the script message retained the original active turn.
The final overlay run passed in 0.635s.

The overlay, diagnostic source, and temporary logs were removed after evidence capture.
These experiments establish feasibility. They are not permanent regression coverage or implementation completion.

## Requirement conformance

The [active attachment requirement](../../specs/tasks/requirements/attach-workspace-sources.md) already defines the intended result.
No new product requirement or acceptance ID is necessary.

- `AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.2`: successful persistence and materialization.
- `AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.4`: compensation after materialization failure.
- `AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.5`: conversation, session, and workspace continuity.
- `AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.6`: live legacy add-branch compatibility and exact paths.

The task system owns the repair because the failed contract combines task mutation admission and conversation-turn ownership.
The technical design was incomplete at the script-message boundary.
The [script-turn design](../../specs/tasks/system-design/workspace-source-script-turns.md) supplies that boundary without changing established admission rules.

## Scope

### In scope

- Correct trusted message translation in `apps/backend/internal/backendapp/worktree.go`.
- Preserve explicit or observed active turn IDs. Use `CompletedTurn` after a successful lookup finds no active turn.
- Cover setup and cleanup messages, output, failure status, retries, and subsequent task progress.
- Add regressions through real materializers and the existing desktop and phone attachment flows.

### Out of scope

- Replacing the task mutex, shortening its critical section, or adding an asynchronous script worker.
- Changing public APIs, database schemas, script timeouts, setup warning policy, placement, or provider permissions.
- Repairing a backend already deadlocked before this correction loads, or rewriting historical turns.
- Fixing the separate empty `workspace_relative_path` defect from #4797/#4807.
- Redesigning attachment controls, translated copy, or remote executor materialization.

## Technical approach

In `taskServiceAdapter.CreateMessage`, preserve a supplied `TurnID` first.
Otherwise, use `Service.GetActiveTurn`, which performs a read without acquiring the workspace gate.
Pass an observed turn ID explicitly. After confirmed absence, use the existing internal `CompletedTurn` request field.
Propagate lookup errors without treating them as absence.

`Service.CreateMessage` already prioritizes explicit IDs and routes `CompletedTurn` through `createCompletedTurn`.
No task-service production change is expected.
`bootMsgAdapter`, ordinary conversational message creation, and `createTurn` retain their existing behavior.

This approach preserves the mutation gate through materialization and rollback.
Releasing it around setup risks concurrent attachments or user turns observing an unfinished workspace.
Deferring setup until after success weakens the existing script/output sequence.
Reentrant-lock tokens and extra setup turns add machinery that the completed lifecycle path already supplies.

### Compatibility and verification scope

| Caller or state | Transport and identity | Intended behavior | Evidence and fallback |
| --- | --- | --- | --- |
| Idle Worktree attachment | Files REST or batch MCP, existing task/session/environment | Complete materialization and setup without opening a conversational turn | Backend integration and desktop/phone E2E. Existing input and ownership errors remain. |
| Idle legacy add-branch | Legacy service/MCP, selected existing session | Complete setup and return exact worktree/task-root paths | Backend integration. Existing environment failures still compensate. |
| Active legacy add-branch | Legacy service/MCP, existing active turn | Retain that turn and agent CWD | Backend integration and existing live-attachment regressions. |
| Prelaunch attachment | No ready environment | Persist and defer setup until provisioning | Existing prelaunch materializer and service coverage. |
| Worktree cleanup | Trusted adapter, existing or missing session | Bind existing/completed turn or retain best-effort error handling | Adapter and compensation coverage. Missing-session cleanup behavior remains. |
| Remote or container attachment | Existing executor-specific materializer | Retain its current behavior | No new support claim or live-provider verification in this package. |

## Tests

Task 01 owns the following permanent regressions:

| Acceptance criteria | Test file | Planned tests |
| --- | --- | --- |
| 001.2, 001.5 | `apps/backend/internal/backendapp/worktree_script_turns_test.go` | `TestWorktreeScriptMessageAdapter_IdleLifecycle`, `TestWorktreeScriptMessageAdapter_PreservesTurnBinding`, `TestWorktreeScriptMessageAdapter_ActiveTurnLookupFailure` |
| 001.2, 001.5 | `apps/backend/internal/backendapp/workspace_source_script_turns_test.go` | `TestWorkspaceSourceScripts_AttachIdle` (fails before the correction), `TestWorkspaceSourceScripts_ExactRetry`, `TestWorkspaceSourceScripts_SetupFailure` |
| 001.6 | Same integration file | `TestWorkspaceSourceScripts_AddBranch` with idle and active cases |
| 001.4 | Same integration file | `TestWorkspaceSourceScripts_Compensation` with a post-setup failure and configured cleanup |

The full IDs use prefix `AC-TASKS-ATTACH-WORKSPACE-SOURCES-`.
Keep fixture helpers small and register resource cleanup before assertions.
Use channels for ordering and a generous timeout only as a deadlock guard.
The regression must fail at the lock boundary before the production change.
Do not let a deadlocked goroutine silently pass or remain behind a successful test.

Retain `TestAttachWorkspaceSources_HoldsTaskMutationGateThroughMaterialization` and existing cancellation, compensation, prelaunch, and lifecycle-shadow tests.

## E2E tests

Task 02 adds one focused scenario to each new spec:

- `apps/web/e2e/tests/task/workspace-source-setup-script.spec.ts`, project `chromium`.
- `apps/web/e2e/tests/task/mobile-workspace-source-setup-script.spec.ts`, project `mobile-chrome`.

Both cover `AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.2` and `AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.5`.
Each starts a Worktree task, waits for the existing turn to complete, then attaches a saved repository with a setup script.
Submit through Files using the existing direct-child placement, which preserves the agent CWD.
Verify HTTP completion, the script message/output, Files content, reload persistence, and a response to the next user prompt.
Use a marker from the script to prove execution. Command metadata alone is insufficient.

The phone case uses the existing drawer and touch controls. The desktop case uses the existing dialog.
No rendered controls or layouts change, so the package needs no new UI composition or ASCII preview.
Existing desktop/mobile attachment tests and startup setup-script tests remain compatibility checks.

## Work orders

- [x] [Task 01: Bind worktree script messages without opening a turn](task-01-bind-worktree-script-turns.md)
- [x] [Task 02: Prove setup attachment and subsequent chat on desktop and phone](task-02-cover-desktop-and-phone.md)

Task 01 preceded Task 02. Both executed sequentially in the primary session.
The completed attachment, live add-branch, and current-workspace packages retain their existing statuses and scopes.
This package adds regression coverage without reopening their completed work orders.

## Verification results

Design investigation: confirmed both idle deadlocks and passed the temporary overlay experiment for all three diagnostic cases.

Implementation verification on 2026-11-08:

- Red deadlock evidence was captured before the correction. The idle attachment test stopped at the task mutex with `AttachWorkspaceSources` -> `runWorktreeSetupScript` -> `taskServiceAdapter.CreateMessage` -> `Service.CreateMessage` -> `createTurn` -> `lockWorkspaceSources`.
- Backend adapter and integration suites passed under the race detector, including the new idle, retry, setup-failure, add-branch, compensation, explicit-turn, observed-turn, observed-turn-completion-with-successor, and lookup-failure cases.
- Task-service mutation-gate, add-branch, completed-turn, and lifecycle-shadow regressions passed.
- Worktree script and setup regressions passed.
- Changed-file Go lint reported no issues and `git diff --check` passed.
- Desktop browser coverage passed: 11 tests in 1.8 minutes, including the new scripted attachment scenario and the existing attachment and setup-script regressions.
- Phone browser coverage passed: 2 tests in 27.5 seconds, including the new mobile scenario and the existing mobile attachment regression.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.test.py`, and `python3 scripts/lint-spec-files.py --all` passed.
- The delivery preflight through `.github/scripts/pr-docs.cjs` returned `status: covered` with no errors.

Exact commands and per-work-order evidence are recorded in the two work orders.

Design gates passed on 2026-11-07:

- `python3 scripts/list-docs.py validate`: 437 decisions and 1938 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `.github/scripts/pr-docs.cjs` exported `validateCoverage`: both pending work orders and both system-design references accepted.
  This preflight included the planned runtime path to exercise coverage before implementation. It did not claim a production change.
- Local Markdown link and whitespace checks passed for all four new documents.
- `git diff --check -- docs/specs docs/plans/workspace-source-script-deadlock`: passed.
- `git status --short`: only the existing design link and four new package documents remained. All files were unstaged.

## Risks

- A completed lifecycle turn must not replace an existing conversation turn or hide pending clarification state.
- A failed active-turn lookup must not trigger ordinary turn creation or be treated as an idle result.
- The real script fixture must use the production adapter. A stub that returns a message cannot reproduce this deadlock.
- Initial setup, cleanup, nonzero exits, and exact retries share the adapter and need compatibility coverage.
- A loaded correction cannot release a mutex already held by an old blocked request. Existing affected backends still require restart.
- The issue's original Claude Code runtime was not reproduced live. Backend evidence isolates the failure before agent-provider execution.
