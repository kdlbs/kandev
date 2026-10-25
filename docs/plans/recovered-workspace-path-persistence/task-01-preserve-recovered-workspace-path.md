---
id: "01-preserve-recovered-workspace-path"
title: "Preserve recovered workspace path"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002
acceptance_criteria:
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002.1
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002.2
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002.3
system_design:
  - ../../specs/tasks/system-design/additional-session-workspace-reuse.md
---

# Task 01: Preserve Recovered Workspace Path

## Summary

Make the lifecycle execution workspace authoritative when launch or resume
persistence selects the task environment path. Prove the correction with a
failing unit regression and the existing browser-level backend-restart flow.

## In scope

- Add the recovered workspace-only precedence case to
  `TestResolveTaskEnvWorkspacePath` and confirm RED.
- Change `computeWorkspacePath` to prefer the lifecycle execution workspace,
  while retaining worktree-only and source-repository fallbacks.
- Add a stable selector for the visible Files workspace path.
- Add a Worktree-executor restart/resume scenario to the existing session
  resume Playwright specification.
- Record RED and final command results in this work order and `plan.md`.

## Out of scope

- Schema or data migrations.
- Lifecycle promotion redesign or new worktree metadata requirements.
- Frontend state derivation, layout, mobile composition, or copy changes.
- Filesystem-based repair of missing canonical worktree inventory.
- Manual repair of the reported live task.

## Acceptance

- A response with the recovered execution workspace and no worktree metadata
  persists that workspace instead of the request's source repository.
- Existing worktree-only, SSH, quick-chat, and empty-response fallbacks retain
  their intended paths.
- After backend restart and automatic resume, the Files path shown for a
  Worktree task is unchanged from its original canonical worktree.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/backend && go test ./internal/orchestrator/executor -run '^TestResolveTaskEnvWorkspacePath$'
cd apps/backend && go test ./internal/orchestrator/executor
cd apps/web && pnpm e2e:run tests/session/session-resume.spec.ts -- --grep 'preserves the worktree Files path after backend restart'
```

The new unit subtest and E2E assertion must fail for the diagnosed path mismatch
before the production correction and pass afterward. Confirm Playwright
collects and executes the named `chromium` test.

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/executor/executor_execute_workspace_path_test.go`
- `apps/web/components/task/file-browser-toolbar.tsx`
- `apps/web/e2e/tests/session/session-resume.spec.ts`
- `docs/plans/recovered-workspace-path-persistence/plan.md`
- `docs/plans/recovered-workspace-path-persistence/task-01-preserve-recovered-workspace-path.md`

## Dependencies

None.

## Risks

- An E2E test that reloads into Chat instead of Files may miss the
  workspace-only reconstruction and fail to exercise the production sequence.
- Adding a selector must not change the Files toolbar's responsive structure or
  accessible name.

## Parallelism

`sequential`

## Inputs

- `REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002` and its acceptance
  criteria.
- `docs/specs/tasks/system-design/additional-session-workspace-reuse.md`.
- `docs/decisions/2026-08-08-task-owned-worktree-lifetime.md`.
- `computeWorkspacePath`, `backendapp.lifecycleAdapter.LaunchAgent`, and
  `lifecycle.Manager.promoteWorkspaceExecution`.
- Existing restart patterns in
  `apps/web/e2e/tests/session/session-resume.spec.ts`.

## Results

- Made `LaunchAgentResponse.WorkspacePath` authoritative, with
  `WorktreePath` and `RepositoryPath` retained as legacy fallbacks.
- Added the defect-specific Go regression for a recovered execution that has
  no worktree metadata and confirmed it failed before the production change.
- Added the Files path selector and a Chromium restart/resume test that checks
  the original worktree path before restart and after promotion plus a fresh
  boot-payload reload.
- The first E2E draft used a DOM relationship and stayed green before the
  backend correction under normal fixture timing. The final E2E RED was the
  missing stable selector; the Go test remains the direct regression for the
  response-precedence defect.
- All verification commands in this work order passed. The executor package
  reported 527 passing tests, and the named Chromium run reported 1 passing
  test.
- No mobile-only test was added because the change does not alter Files layout,
  controls, touch behavior, or responsive composition; both desktop and mobile
  render the same normalized path component.
