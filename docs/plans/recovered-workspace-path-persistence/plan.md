---
created: 2026-08-30
status: done
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002
system_design:
  - ../../specs/tasks/system-design/additional-session-workspace-reuse.md
legacy_specs: []
---

# Implementation Plan: Recovered Workspace Path Persistence

## Overview

Preserve a Worktree task's actual execution workspace when backend restart
recovery creates a workspace-only execution and later promotes it during agent
resume. The work starts with a focused failing precedence regression, applies
the smallest orchestrator correction, and then proves the user-visible Files
path through the existing restart E2E flow.

The confirmed failure is:

1. Restart recovery reconstructs an execution at the correct worktree path.
2. Workspace-only promotion returns that path as `WorkspacePath`, but the
   recovered metadata has no `WorktreePath`.
3. `computeWorkspacePath` prefers the request's source `RepositoryPath` over
   the response's actual `WorkspacePath`.
4. `persistTaskEnvironment` overwrites the correct environment path with the
   source checkout.
5. Session reads prefer the environment path, so Files displays the source
   checkout even though agentctl still runs in the worktree.

## Scope

### In scope

- Treat the lifecycle response's execution workspace as authoritative over
  source repository input.
- Preserve legacy fallbacks for responses without an execution workspace.
- Add a focused Go regression that fails under the current precedence.
- Extend the existing session-restart E2E coverage to assert that the Files
  path remains the original Worktree task path after automatic resume.
- Add only the stable selector needed for that UI assertion.

### Out of scope

- Database schema changes or startup migrations.
- Files layout, navigation, touch behavior, or responsive composition changes.
- Reconstructing missing worktree inventory from filesystem state.
- Changing the environment-first task-session API projection.
- Direct mutation of a live user's database as part of the code change.

## Technical approach

### Workspace-path precedence

Update `computeWorkspacePath` in
`apps/backend/internal/orchestrator/executor/executor_execute.go` so a non-empty
`LaunchAgentResponse.WorkspacePath` wins. Retain `WorktreePath` as the fallback
for legacy lifecycle responses and `LaunchAgentRequest.RepositoryPath` as the
last repository-backed compatibility fallback.

The response workspace is `AgentExecution.WorkspacePath`, which is the actual
agentctl working directory surfaced by `backendapp.lifecycleAdapter`. This
keeps fresh launch, workspace-only promotion, SSH recovery, and quick-chat
recovery on one authority rule.

### Regression boundaries

Add a subtest to
`apps/backend/internal/orchestrator/executor/executor_execute_workspace_path_test.go`
with a source repository request, an empty response `WorktreePath`, and a
correct response `WorkspacePath`. It must expect the response workspace and
fail before the production correction.

Extend
`apps/web/e2e/tests/session/session-resume.spec.ts` with a Worktree-executor
scenario that records the original session worktree, keeps Files active,
restarts the backend, waits for automatic resume, and asserts the visible Files
workspace path remains unchanged. Add a stable path selector in
`apps/web/components/task/file-browser-toolbar.tsx`; no presentation behavior
changes.

## Tests

- `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002.1`: the unit regression
  proves recovered execution workspace precedence over the source checkout.
- `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002.2`: the restart E2E proves
  the Files projection remains the canonical Worktree path after promotion.
- `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002.3`: existing repository
  fallback coverage remains green when the response has neither workspace nor
  worktree path.

## E2E tests

Add `preserves the worktree Files path after backend restart` to
`apps/web/e2e/tests/session/session-resume.spec.ts` in the `chromium` project.
It uses the existing `backend.restart()` fixture and session recovery page
object patterns. The shared Files component supplies the same normalized path
to desktop and phone; because this fix does not alter layout or interaction,
no new mobile-only scenario is required.

## Work orders

- [x] [Task 01: Preserve recovered workspace path](task-01-preserve-recovered-workspace-path.md)

## Verification results

- RED: the new `recovered execution keeps lifecycle workspace` subtest failed
  because `computeWorkspacePath` returned `/source/kandev` instead of
  `/tasks/task-1/kandev`.
- RED: Chromium collected the named restart scenario and failed on the absent
  `file-browser-workspace-path` selector before the selector was added. A
  DOM-coupled prototype already kept the path in the normal E2E timing; the Go
  regression is the defect-specific precedence gate.
- `cd apps && pnpm install --frozen-lockfile` passed.
- `cd apps/backend && go test ./internal/orchestrator/executor -run '^TestResolveTaskEnvWorkspacePath$'`
  passed with 7 tests.
- `cd apps/backend && go test ./internal/orchestrator/executor` passed with 527
  tests.
- `cd apps/web && pnpm e2e:run tests/session/session-resume.spec.ts -- --grep 'preserves the worktree Files path after backend restart'`
  collected and passed 1 Chromium test.
- `cd apps/web && pnpm exec prettier --check components/task/file-browser-toolbar.tsx e2e/tests/session/session-resume.spec.ts`
  passed.
- `cd apps/web && pnpm run typecheck` passed.
- `python3 scripts/lint-spec-files.py --all` and `git diff --check` passed.

## Risks

- `WorkspacePath` and `WorktreePath` historically overlap. The test matrix must
  preserve legacy responses that supply only `WorktreePath`.
- The E2E stimulus must open Files before restart so reload reconstructs the
  workspace-only execution before automatic resume promotes it.
- Previously corrupted rows are repaired only after a successful resume under
  the corrected binary and only while canonical worktree inventory remains.
