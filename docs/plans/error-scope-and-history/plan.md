---
created: 2026-09-14
status: complete
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002
system_design:
  - ../../specs/agents/system-design/session-recovery-failures.md
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
legacy_specs: []
---

# Implementation Plan: Error scope and history

## Overview

Retain session errors as chronological messages after recovery. Show shared task errors above all task tabs.
Implement session history first, then the shared task surface, then cross-surface regression coverage and public guidance.
All three sequential work orders are complete. This document records the implementation and verification results for the accepted design package.

## Confirmed intent and ownership

The user chose ordinary session error messages and rejected duplicate banners.
The user requires shared task/workspace failures to remain visible across agent sessions and other tabs.
Tasks own durable failure identity, history, and shared projections. Agents retain recovery eligibility and session presentation.
The proposed shared position is below the task header and above tabs. It covers this task, not every task in the workspace.
No material question blocks this design.

## Evidence and scope

Task ef406dc0-8d8b-41bf-b9f1-39ec4b026ff9 had a saved-session load failure after exactly two minutes on September 14.
The diagnostic bundle showed ACP `session/load` returning `context deadline exceeded`. The underlying provider delay remains unproven.
`task-chat-panel.tsx` passes the recovery card as `prependContent` before the history loader.
`message-list-native-scroll.ts` releases recovery top ownership after user scrolling. Later history restores message position and moves the card away.
`deduplicateRecoveryMessages` removes prior recovery messages after later conversation activity.
`SettledActionMessage` hides its body during STARTING, RUNNING, and COMPLETED states.
The projector retains a separate `taskError`, but the public aggregate can select a newer session error.

In scope: durable session entries, resolved action state, ordinary scrolling, explicit failure scope, shared task projection, all-tab presentation, localization, and regression tests.
Out of scope: provider timeout increases, provider hang diagnosis, automatic conversation replacement, a global alert center, and a multi-incident task queue.
The generic timeout label defect remains a separate diagnostic finding. This package preserves existing sanitized cause semantics.

## Technical approach

Task 01 extends the existing fenced error admission and persisted message path.
Keep one marker per session/stamp, preserve markers after resolution, and render recovery actions only on the current error. Bootstrap failures use the execution-fenced terminal commit and then create the same idempotent session message. If that accepted commit is followed by a transcript write failure, the executor retries the same message identity. Profile-specific pre-agent failures remain session-owned, while shared worktree failures use task scope, and Office sessions use the same history producer.
Remove activity-based error filtering, active-session body hiding, and error-specific scroll placement across task Chat and Quick Chat.

Task 02 adds explicit scope to normalized error metadata and exposes independent `TaskStatusSummary.task_error`.
Project existing task metadata independently of session errors. Preserve aggregate `active_error` compatibility.
Mount one shared task-shell alert with a desktop dialog or phone drawer. Retain existing action authorization and stamp guards. Read the live status summary after hydration, announce each task error stamp once, and remove duplicate mobile top padding while retaining the outer task-shell offset.

Task 03 proves recovery followed by new messages, pagination, reload, tab switching, mixed scopes, and mobile geometry. Its composed panel assertions count persisted recovery rows and provisional notices together, and legacy unstamped rows retain controls only when they match the current session error.
Update public recovery guidance only after the behavior exists.

## ASCII UI preview

UI-01: Desktop task, shared failure plus an independently recovered session error.

```text
+--------------------------------------------------------+
| Task title / workflow                                  | fixed
| ! Workspace preparation failed. [Recovery details]      | shared
+--------------------------------------------------------+
| Session A | Session B | Plan | Pull request | Files      | tabs
+--------------------------------------------------------+
| Earlier conversation                                 ^ |
| ! Session resume failed. 10:18                        | |
|   Loading timed out. [Recovery details]                | | scrolls
|   Recovered.                                          | |
| Agent: I resumed work...                              v |
+--------------------------------------------------------+
| Message input                                          | fixed
+--------------------------------------------------------+
```

UI-02: Phone task Chat, same two independent failures.

```text
+--------------------------------+
| < Task title       Session A v | fixed task chrome
| ! Workspace preparation failed |
| [Recovery details]             | shared on every view
+--------------------------------+
| Earlier messages             ^ |
| ! Session resume failed      | |
| Loading timed out.           | | transcript scrolls
| Recovered. [Details]         | |
| Agent: I resumed work...     v |
+--------------------------------+
| Message input                  | safe-area clearance
| Chat | Plan | Files | More     | existing navigation
+--------------------------------+
```

UI-03: Session entry states, inside either transcript.

```text
Unresolved: ! Resume failed. [Resume] [Recovery details]
Pending:    ! Resume failed. Resuming... [Details]
Recovered:  ! Resume failed. Recovered. [Details]
Older:      ! Resume failed. [Details]   (no stale actions)
```

Expanded session details wrap inline. Phone actions stack with 44-pixel targets.
A failed new attempt appends a separate error entry. Same-stamp updates retain position.

UI-04: Shared details, desktop dialog / phone inset bottom drawer.

```text
+--------------------------------+
| Workspace preparation       X  |
| Affected repository: owner/repo|
| Safe cause and bounded details |
| [Valid recovery action]        |
+--------------------------------+
```

The phone drawer has safe-area clearance, an internally scrolling body, and 44-pixel controls.
The desktop dialog uses compact controls. Closing details does not clear the shared alert.
Views map to recovery criteria 006.4/6/7/8/9 and task criteria 002.4/5/6/7.
Fixed versus scrolling regions, scope, order, and retained history are requirements.
Copy and spacing are illustrative and must use localized strings and existing tokens.
The two errors coexist only because they represent different failures.

## Tests

- Session criteria 006.4/7/8/9 and task 002.2/3/8: orchestrator failure persistence, repository fencing, processed-message filtering, action-message, and native-list tests.
- Task 002.1/4/5/6/8: task status projector and rebuild tests, normalized client types, shared alert selection, and task layout tests.
- Existing provider recovery and cancellation tests must preserve action semantics.
- Use failure identity, not text, for duplicate tests. Include missing legacy metadata and a newer failed retry after successful recovery.

## E2E tests

Extend `tests/task/launch-failure-recovery.spec.ts` (chromium) and `tests/task/mobile-launch-failure-recovery.spec.ts` (mobile-chrome).
Task 01 adds retained-history and scroll regressions. Task 02 adds shared-alert navigation and no-session regressions.
Task 03 combines both scopes, reload, delayed events, short viewport, touch actions, and representative preview/Quick Chat coverage.
The work orders specify exact commands and scenario names. Mock provider and workspace failures through existing fixtures.
Use causal waits, not arbitrary sleeps. Rebuild through the managed runner.

## Work orders

- [x] [Task 01: Retain session failure entries](task-01-session-history.md)
- [x] [Task 02: Show shared errors across task tabs](task-02-shared-task-errors.md)
- [x] [Task 03: Verify scope and document recovery](task-03-regressions-and-docs.md)

## Verification results

Implementation is complete. Session failures now remain chronological transcript entries with stamp-scoped controls. Task and workspace preparation failures now project independently to one task-shell surface above task tabs, with desktop dialog and phone drawer details.

The review remediation is included in this completed package. Accepted bootstrap failures now write the same idempotent, safe session-history entry as other recoverable failures, including Office sessions. The panel keeps a provisional error notice only until its persisted message arrives. Status-summary restoration honors explicit task/session scope even when a task error carries an originating session ID. Recovery retirement uses a stamp-fenced write and publishes inactive state only after that write wins.
The review fixup also makes bootstrap admission fail closed when an execution-fenced repository commit is unavailable, repairs an accepted terminal state after a transcript write error, and preserves profile-specific launch failures on their originating session. The shared task surface refreshes from the live summary, emits one assertive announcement per task and stamp, and avoids nested mobile top-bar spacing.

Product verification on September 14:

- `pnpm --filter @kandev/web run build:vite`: passed.
- `pnpm run typecheck`: passed.
- `pnpm run lint`: passed.
- `pnpm run i18n:check`: passed for all five complete catalogs.
- `make lint` and the selected Go package tests for orchestrator, executor, SQLite, and status-summary: passed.
- Review regression tests for bootstrap and Office history, persisted/provisional deduplication, scoped projector clearing, and stale recovery retirement: passed.
- The complete selected Go package run passed: orchestrator, executor, SQLite, and status-summary.
- Focused frontend regression tests passed: 6 files, 96 tests.
- Chromium launch-recovery E2E: 4 passed.
- Mobile Chrome launch-recovery E2E: 3 passed.
- Public documentation validators and specification validators: passed.
- `git diff --check`: passed.

Review-fixup verification on September 14:

- `go test ./internal/orchestrator -count=1`: passed.
- The executor and status-summary package suites: passed.
- The five affected frontend suites: 65 tests passed.
- Frontend typecheck and lint: passed.
- Frontend localization checks and Vite build: passed.
- Legacy FAILED Office rows without structured metadata retain their chronological entry and recovery actions; the focused recovery suites pass with 54 tests.
- Desktop and mobile PR watcher failures now exercise the task-shell error strip and both focused E2E specs pass.
- Remaining CI recovery scenarios now assert their owning surface: the GitHub URL launch failure uses the shared task strip, Kubernetes failures use retained session entries with sanitized technical details, and failed mobile resume uses the persisted recovery entry. Focused follow-up runs pass for Chromium, Mobile Chrome, and the applicable Kubernetes container cases.

The implementation changed production and permanent test files as required by the work orders. The provider timeout diagnosis and repair remain outside this package.

Design validation on September 14:

- `python3 scripts/list-docs.py validate`: passed, 268 decisions and 902 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Work-order requirement, acceptance, and system-design reference checks: passed for all three work orders.
- `git diff --check -- docs/specs docs/decisions docs/plans`: passed.

Task 03 updated the public session and task recovery guidance after implementation.

## Risks

- Duplicate websocket or terminal events can create duplicate history unless the durable write is fenced and idempotent.
- Legacy errors lack stamps or retained markers. Preserve known history without inventing missing incidents.
- Mixed-version aggregates cannot recover an untransmitted shared error. New boot/read/update contracts must agree.
- Dockview maximization and phone navigation must not hide task-shell alerts.
- Broad workspace text matching can misclassify session-only errors. Scope comes from the owner.
- Existing completed plans contain historical reveal tests. Replace affected expectations and preserve unrelated guarantees.

## Related decision

[Error scope and history](../../decisions/2026-09-14-error-scope-and-history.md).
