---
created: 2026-09-10
status: complete
requirements:
  - REQ-TASKS-COMPLETION-002
  - REQ-TASKS-COMPLETION-003
system_design:
  - ../../specs/tasks/system-design/task-completion.md
legacy_specs: []
---

# Fix Plan: Workspace restoration after conversation completion

## Overview

Opening completed work should restore its workspace without resuming its agent.
Genuine workspace failures should appear in the affected workspace panel.
Implement runtime admission first, scoped feedback second, and browser coverage
third. All work orders are sequential; no delegation is authorized.

The task system owns this repair because it controls session completion and
workspace restoration admission. Extend the existing task-completion requirement
and design rather than create a separate UI specification. Existing task-owned
workspace and generation-fencing ADRs supply the rationale; no new ADR is needed.

## Evidence and requirement conformance

Baseline: `272ddfca8a81065db8e30c6230044fe90f757356` on `origin/main`, including
PR #3564. This task branch was fast-forwarded to that baseline before authoring.
Recheck the base and relevant contracts before implementation if main advances.

The reported task is `76910417-48da-498a-8e95-1ee2c309ab9d`. Its session
`859abeec-e739-4a3a-a04d-a484c9815f6c` completed at 2026-09-09 20:41:23 UTC.
Read-only session metadata confirms COMPLETED. Retained backend logs show
`restore_workspace` failures at 2026-09-10 12:19:51 and 12:20:07 Europe/Lisbon:
`verify session before registering execution: ... is COMPLETED: session is terminal`.
The screenshot shows the same rejection above a completed transcript and a
Files panel stuck on preparation. No live task was restarted or repaired.

`launchRestoreWorkspace` explicitly accepts terminal sessions, but
`createExecution` calls `ensureLaunchSessionStillActive` before creation and
again around registration. That guard rejects every terminal session. Reuse of
an already-live runtime bypasses creation, which explains why tests or visits
with a warm runtime can miss the defect. `describeEnsureError` then assigns a
session-start title to a restore failure, and `task-page-inner.tsx` renders it
above the entire layout.

The existing terminal-session lifecycle tests pin the conflicting policy. The
correction must replace only the workspace expectations while preserving the
actual agent-launch and cleanup guards.
PR #3564's explicit `AllowCompletedSessionResume` is not a workspace permission.

Classification: the passive state invariant already exists in
`AC-TASKS-COMPLETION-002.3`; workspace availability and failure placement were
missing. Add `REQ-TASKS-COMPLETION-003` with ten criteria and extend the existing
system design. Preserve all `002` explicit-resume criteria and its prior package.

Smallest deterministic regression: a real canonical retained workspace,
COMPLETED session, and empty execution store; call
`EnsureWorkspaceExecutionForSession`. The implementation now admits the
workspace without returning `ErrSessionTerminal`, while the agent-start paths
still reject terminal sessions. The permanent regression covers all three
workspace ensure entry points and FAILED, CANCELLED, and COMPLETED states.

## Scope

### In scope

- Restore retained workspaces for completed sessions and preserve the endpoint's
  workspace-only behavior for FAILED/CANCELLED sessions.
- Admission, authorization, cleanup/ownership races, shared runtime reuse,
  retained provider context, and explicit Resume compatibility.
- Workspace-local failure state, Retry, Details, loading settlement, and
  desktop/mobile behavior across Files, Changes, and workspace terminals.
- Focused regression tests and public recovery documentation at implementation.

### Out of scope

- Automatic agent resume, task reopening, queue policy changes, new sessions,
  revised FAILED/CANCELLED message admission, or Office scheduling changes.
- New schema, runtime flags, executors, or global launch-error redesign.
- New workspace creation, branch replacement, archive/unarchive policy changes,
  zero-session task access, deployment, or mutation of the reported live task.

## Technical approach

Follow the Workspace admission, Workspace runtime continuity, and Workspace
failure feedback sections of the [system design](../../specs/tasks/system-design/task-completion.md).

1. In lifecycle `manager_execution.go`, separate workspace admission from the
   agent launch guard in `manager_launch.go`. Retain the existing cleanup-aware
   registration and shared singleflight. Audit `persistence.go`, workspace
   inventory reconciliation, and resume promotion for state/context continuity.
2. Keep `session.launch` with `intent: restore_workspace`. Classify failures by
   request intent; preserve safe backend error categories and sanitization.
   Reconcile Git handlers' `session_terminal` short-circuit with workspace
   eligibility instead of treating every historical conversation as unavailable.
3. Share environment-scoped restore attempt state through existing frontend
   domain/store infrastructure. Route it to workspace content, not the global
   session banner. Extend `WorkspaceUnavailable` and settle panel loading state.
   Preserve real agent recovery feedback and archive handling.
4. Exercise cold restoration and failure/retry in managed desktop/mobile E2E.
   Extend completed-conversation tests to Resume after workspace restoration.

## Tests

The following evidence was run for the implementation. Criterion suffixes in
this table use `AC-TASKS-COMPLETION-`.

| Criteria | Regression evidence |
| --- | --- |
| `003.1`, `003.2`, `003.4` | lifecycle `manager_execution_test.go`: `TestEnsureExecutionAllowsTerminalWorkspaceWithoutStartingAgent` covers COMPLETED/FAILED/CANCELLED and all three ensure entry points; `manager_workspace_restore_test.go` keeps `TestAgentStartsStillRejectTerminalSessions`, `TestWorkspacePromotionStillRejectsTerminalSessions`, and `TestPassthroughReconnectStillRejectsTerminalSessions` for agent-only rejection |
| `003.3`, `002.2`, `002.3`, `002.4`, `002.8`, `002.13` | desktop `completed-workspace-restoration.spec.ts` and `completed-session-resume.spec.ts` restore the workspace, restart the backend, and then resume the same conversation without changing task/session identity |
| `003.5`, `003.6` | lifecycle `manager_workspace_restore_test.go`: `TestWorkspaceRestoreRejectsActiveCleanupWithoutMutation`, `TestWorkspaceRestoreRejectsAdmissionChangesWithoutMutation`, and `TestCachedWorkspaceRestoreRechecksProviderAdmission`; existing coalescing and deduplication tests remain in the focused lifecycle block |
| `003.7` | existing lifecycle workspace-validation and materialization tests plus `TestWorkspaceRestoreRejectsAdmissionChangesWithoutMutation`; the cold E2E uses an existing retained worktree and does not create a replacement |
| `003.1`, `003.5`, `003.8` | orchestrator `session_launch_test.go`: `TestLaunchRestoreWorkspace*`; existing Git/files/shell/terminal handler tests; the desktop and mobile E2E failure/retry path |
| `003.8`, `003.9` | `use-workspace-restoration.test.ts`, `file-browser-load-state.test.tsx`, `ensure-session-error.test.tsx`, `workspace-unavailable.test.tsx`, and resumption/recovery hook tests |
| `003.10`, `002.1`, `002.3` | desktop/mobile E2E; rendered phone controls, disclosure geometry, passive state preservation, and no horizontal overflow |

## E2E tests

- Add `apps/web/e2e/tests/session/completed-workspace-restoration.spec.ts`,
  project `chromium`: materialize a real checkout, complete the session, stop
  the isolated runtime via fixture restart, then open Files and a known file.
  Verify Git content and a shell command, no error banner, unchanged completed
  session/task, and no additional agent turn. Reload and repeat file access.
- Add `apps/web/e2e/tests/session/mobile-completed-workspace-restoration.spec.ts`,
  project `mobile-chrome`: use shipped Files navigation and file-viewer Back;
  verify the same content/state result and terminal access through mobile controls.
- Both new specs inject one bounded workspace failure, assert local feedback
  instead of endless preparation, open Details, then Retry to real success.
  Keep Chat usable and verify no unintended Resume request. Inspect phone
  screenshots, touch hitboxes, disclosure containment, and horizontal overflow.
- Extend existing `completed-session-resume.spec.ts` and
  `mobile-completed-session-resume.spec.ts`: restore workspace first, then click
  Resume, send one follow-up, and preserve conversation/task identity.

Happy paths must cross real backend workspace creation. Existing completed-chat
fixtures leave a warm execution and do not prove this regression. Use
`backend.restart()` with the worker's database and checkout retained; establish
the completed/no-live-runtime precondition before navigation. Do not use a
recovery-clicking page helper as evidence of passive restoration.

## Work orders

- [x] [Task 01: Restore retained workspace access](task-01-workspace-admission.md) (complete)
- [x] [Task 02: Scope workspace failure feedback](task-02-workspace-feedback.md) (complete)
- [x] [Task 03: Prove completed workspace recovery](task-03-workspace-e2e.md) (complete)

## Verification results

Implementation and regression tests completed on 2026-09-10:

- Task 01 backend block: lifecycle 91 tests, orchestrator 125 tests,
  orchestrator/executor 105 tests, orchestrator/handlers 3 tests, and
  agent/handlers 200 tests passed with `-race`.
- Task 02 frontend blocks: 58 and 63 tests passed; the extracted file-browser
  tests added 8 more passing tests. Web typecheck and full web lint passed.
- `rtk pnpm run i18n:check` and `rtk pnpm run i18n:ratchet` passed. The i18n
  check reported the existing 140 orphaned catalog entries.
- Task 03 managed E2E: desktop 2/2 and mobile 2/2 passed after cold backend
  restart. The mobile capture was inspected for terminal output, touch
  controls, containment, and overflow; the temporary capture directory was
  removed.
- Public docs validation passed: 62 tests and 46 pages. Specification
  validation passed: 36 tests and all specification files.
- `rtk git diff --check` passed. Targeted E2E-sleep lint passed for all four
  changed session specs. The repository-wide E2E-sleep lint still reports its
  pre-existing unrelated baseline and is not used as a task gate.
- Review-fixup validation passed: the impacted frontend suite ran 120 tests,
  backend normal suites ran 5,395 tests across three packages, and the
  lifecycle race block ran 81 tests. Web typecheck, full lint, E2E build, and
  both desktop/mobile cold-runtime flows passed again.
- PR #3577 follow-up aligned the workflow DTO and WebSocket mapper with the
  current `main` contract after the merge-result check exposed a stale
  duplicate field deletion. Local typecheck, lint, and mapper tests passed.

Design validation before implementation on 2026-09-10:

- `rtk python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `rtk python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `rtk git diff --check`: passed.
- Work-order references, source paths, and pending status checked. New test/helper
  files are explicitly marked as implementation targets, not existing evidence.
- The historical design-package commit remains `ad695e1d797c7d81d348618bcfa77bcc523faea0`.

## Risks

- Relaxing the agent guard globally would allow unintended starts. Change only
  workspace admission and retain the explicit Resume permission from PR #3564.
- Cleanup checks must protect registration and cache reuse, not only initial
  lookup. Shared environment ownership and stale execution callbacks matter.
- A workspace-only row could overwrite the provider resume token. Test the
  full restore/restart/Resume sequence, not only immediate file access.
- Current reconciliation can create worktrees. Keep passive restoration
  attach-only and preserve separately authorized unarchive recovery.
- Moving a banner without propagating workspace failure would leave spinners
  and hidden errors. Test content consumers and cached stale data explicitly.
- New localized copy requires en, pt-pt, zh-cn, generated zh-hk/zh-tw, and pseudo.
- Completed-session tests from PR #3564 stay in the verification set. Do not
  reset their previous package status or claim old counts prove this repair.
