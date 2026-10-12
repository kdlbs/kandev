---
created: 2026-10-02
status: implemented
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
system_design:
  - ../../specs/tasks/system-design/additional-session-workspace-reuse.md
  - ../../specs/tasks/system-design/environment-owned-git-status.md
  - ../../specs/platform/system-design/workspace-git-status.md
legacy_specs: []
---

# Fix Plan: Persist the workspace binding after resume

## Overview

Resume must save the session's final workspace binding before it reports success
or starts the agent. This restores canonical workspace continuity and makes
Changes usable after opening or refreshing a recovered task.

Tasks owns the repair because it owns the durable session/environment binding.
Existing requirements define the intended behavior. No new requirement or ADR
is needed. The design update clarifies an existing persistence obligation.

## Confirmed root cause

The affected task is `d6a08c2a-4435-4f2d-a72c-906c219d6fea`.
Its only session has an empty raw `task_sessions.workspace_path`.
Its linked task environment is ready and has the materialized workspace path.

Retained logs show initial launch failure, followed by successful resume.
`Executor.resumeSession` saves its session before `persistTaskEnvironment`
returns the final binding. Agent resume saves `STARTING` at the credential
boundary. Workspace-only resume also saves its session before environment
persistence. Neither path saves the binding after materialization.

`bindSessionToTaskEnvironment` updates only the launch-local session object.
Ordinary session reads project the environment path and conceal the missing
raw value. Git source selection reads the raw value and rejects it with
`workspace_unverified`. Fresh and recovery requests then return unavailable.

The temporary `TestReproResumePersistsGitWorkspaceBinding` failed for both
`workspace_only` and `agent_resume`: the persisted write snapshot remained
empty after successful materialization. The reproduction used frozen write
snapshots because the executor mock stores mutable pointers on full-row writes.
Checking its stored pointer alone produced a false pass. The temporary file
was removed after diagnosis.

## Scope

### In scope

- Persist the final session/environment binding after successful resume materialization.
- Preserve credential admission, startup ownership, cancellation, and cleanup guards.
- Prove raw persistence with value snapshots and repository integration evidence.
- Prove that desktop and phone Changes survive a task reload after recovery.

### Out of scope

- Changes to Git source eligibility, status ordering, timeout budgets, or polling.
- A schema migration, broad historical backfill, or live database patch.
- New UI controls, copy, layout, runtime flags, or public API fields.
- Restarting, stopping, or modifying the user's current agent or workspace.

## Technical approach

### Resume persistence

Keep the existing pre-credential `STARTING` transition. After
`persistTaskEnvironment` succeeds, persist the final `task_environment_id` and
`workspace_path` before resume reports success or starts the agent. Both agent
and workspace-only resume use this final persistence boundary.

The SQLite implementation adds a narrow guarded update that changes only the
two binding columns and timestamp. It predicates on task, session state, and
the startup-attempt ID for agent resumes. Workspace-only resume retains the
state predicate without creating an agent attempt. The final write runs inside
the existing per-session lock and worktree-recovery admission path.

If the same agent attempt advances from `STARTING` to `RUNNING` or
`WAITING_FOR_INPUT` while `LaunchAgent` returns, persistence retries against
the observed state with the same atomic startup-attempt predicate. A replaced
attempt, cancellation, or terminal transition remains ineligible.

Final-write failure propagates, prevents agent startup, and cleans up only the
execution admitted by that resume. Agent-resume rollback restores state and the
prior credential snapshot in one conditional write guarded by the captured
startup attempt. The orchestrator publishes the state transition and releases
the reservation only after that write succeeds. A failed or superseded write
cannot change a different session or attempt's workspace binding.

The repair leaves `eligibleGitStatusSession`, execution/workspace checks,
async source revalidation, and live-only foreground recovery intact.
Existing affected records recover through a later authorized lifecycle resume
with the correction. Page reads do not repair records.

### Compatibility matrix

| Resume shape | Expected result | Evidence |
| --- | --- | --- |
| Agent resume after initial failure | Final raw binding saved before agent start | Executor regression and repository integration |
| Workspace-only resume | Final binding saved before success, no agent start | Executor regression |
| Existing ready environment | Preserve exact canonical workspace | Existing and new continuity tests |
| Inherited environment | Preserve shared environment identity and owner | Existing guest binding tests |
| Promoted root with active repository worktree | Preserve exact allowed path matching | Existing Git source tests |
| Persistence error or replaced attempt | No startup or stale full-row write | Controlled failure and ownership regressions |
| Container or remote workspace | Preserve returned path without host path resolution | Shared contract tests, no real remote run claimed |

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002.1` | `TestResumeSessionPersistsMaterializedWorkspaceBinding`, frozen writes and raw repository reads |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002.2` | Desktop and phone reload regressions |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.5` | Environment identity and changed-file assertions after reload |
| `AC-PLATFORM-WORKSPACE-GIT-STATUS-001.27` | Existing mismatch, unrecorded-source, root-promotion, and replaced-source tests |
| `AC-PLATFORM-WORKSPACE-GIT-STATUS-001.28` | Real fresh refresh response and visible file membership after reload |

Permanent tests go in new focused files. The existing resume and helper test
files are already large. Tests must assert captured values rather than aliasing
the session object that the implementation later mutates.

## E2E tests

Add `tests/git/changes-panel-resume-workspace-binding.spec.ts` for Chromium and
`tests/git/mobile-changes-panel-resume-workspace-binding.spec.ts` for mobile Chrome.
Both use an isolated backend and a disposable worktree task.

Reproduce the lifecycle sequence: failure before initial workspace binding,
successful resume through the real service, dirty files, and task reload.
Use the existing fixture's owned database only for deterministic failure seeding
or raw-state assertions where public fixture APIs cannot express the precondition.
Never change a developer database. Do not intercept a refresh with fake success.

After reload, assert that the real fresh response contains the target file and
the panel shows it without Retry or the clean empty state. Assert the unchanged
environment/worktree identity and preserved dirty file contents.
Use the shipped phone Changes navigation and file selection surface.

This package changes backend persistence and adds browser coverage.
It makes no rendered UI changes, so no new layout preview is required.

## Work orders

- [x] [Task 01: Persist the final resume binding](task-01-persist-resume-binding.md)
- [x] [Task 02: Verify Changes after recovery and reload](task-02-reload-regression.md)

Task 02 depends on Task 01. Execute sequentially in the primary session.

## Verification results

The pre-implementation reproduction failed in both resume modes with an empty
raw binding. The permanent executor and SQLite regressions now pass, and the
narrow guarded write persists the final binding before resume success or agent
startup. Desktop and phone regressions each pass against a real fresh Git
refresh response after reload.

Verification passed:

- Focused executor race tests and the full executor plus SQLite race suites.
- Full orchestrator race suite (`124.532s`), including cancellation, queued
  prompt acceptance, idle-session focus, and unarchive recovery regressions.
- Deterministic successor-attempt coverage proves a zero-row binding write
  remains superseded after a one-shot diagnostic read error and that an
  ordinary final-write error cannot roll back successor state, credentials,
  binding, execution, or reservation.
- The rollback commits state and credential restoration in one
  startup-attempt-guarded update; orchestrator event publication and reservation
  release occur only after that update succeeds.
- Environment-gated PostgreSQL raw write/read-back tests cover workspace
  binding and attempt-fenced rollback, including replaced attempts and metadata
  preservation.
- SQL guard and persistence store conformance tests.
- Negative Git source validation tests in `internal/backendapp`.
- Desktop Chromium and phone `mobile-chrome` E2E regressions, one test each.
- `make -C apps/backend build`, web typecheck, and Vite build.
- Specification catalog validation, full spec lint, and `git diff --check`.

The PostgreSQL tests were invoked but skipped because
`KANDEV_TEST_POSTGRES_DSN` was not configured and no owned test instance was
available. Exact commands and results are recorded in the work orders. No live
runtime or database was changed.

## Risks

- Moving `STARTING` after credentials breaks the existing lease contract.
- An unconditional final write can overwrite cancellation or a successor attempt.
- Mutable mock pointers can falsely certify persistence.
- A repaired writer does not retroactively change an already-running old session.
- Local evidence does not certify every remote executor's runtime behavior.

## Related delivery records

- [Environment-owned Git status](../environment-owned-git-status/plan.md) established raw source provenance and matching safeguards.
- [Environment-scoped Git status](../environment-scoped-git-status/plan.md) changed snapshot ownership and remains a completed record.
- [Changes panel Git refresh](../changes-panel-git-refresh/plan.md) owns progressive status and bounded recovery.

These packages retain their completed implementation results. This repair adds
the missing resume write and reload coverage without reopening their scope.
