---
id: "01-dev-paths"
title: "Dev paths, repo root, and dev backend env"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/go-dev-launcher.md"
---

# Task 01: Dev paths, repo root, and dev backend env

Port the pure path/env logic from `apps/cli/src/{constants,kandev-env,dev}.ts` into Go.
No process spawning in this task.

## Acceptance

- `findRepoRoot(startDir)` returns the nearest ancestor containing both `apps/backend`
  and `apps/web`, also handling a start directory that *is* `.../apps`; it returns an
  error (not a fallback path) when no such ancestor exists.
- `resolveDevBackendEnv(repoRoot)` reproduces the three-way rule in
  `apps/cli/src/dev.ts:125-167`: inside a task workspace it sets
  `KANDEV_HOME_DIR=<repoRoot>/.kandev-dev` and clears `KANDEV_DATABASE_PATH`; outside
  one an explicit `KANDEV_DATABASE_PATH` wins; otherwise it uses the `.kandev-dev`
  default. `KANDEV_DEBUG_DEV_MODE=true` is set in all three branches, and the reported
  display DB path matches what the backend will derive.
- `isInsideKandevTask(repoRoot)` is true when `KANDEV_TASK_ID` is set, or when
  `repoRoot` is under `<userHome>/.kandev/tasks/`.

## Verification

Use TDD. Add `apps/backend/internal/launcher/devpaths_test.go` covering each branch with
`t.TempDir()`-rooted fake trees (never the real filesystem root — see
`apps/backend/CLAUDE.md`, "Path/security tests"), then:

~~~bash
cd apps/backend && go test ./internal/launcher/ -run 'TestFindRepoRoot|TestResolveDevBackendEnv|TestIsInsideKandevTask' -race
~~~

## Files

- `apps/backend/internal/launcher/devpaths.go` (new)
- `apps/backend/internal/launcher/devpaths_test.go` (new)
- `apps/backend/internal/launcher/constants.go` — add `devKandevDotdir = ".kandev-dev"`,
  `devKandevHome(repoRoot)`, `kandevTasksDir()`, and `healthTimeoutDevMS = 600000`
  alongside the existing `healthTimeoutReleaseMS`.

## Inputs

- `apps/cli/src/dev.ts:112-167` — `resolveDevBackendEnv`, including the comments
  explaining why dev always roots under `.kandev-dev` and why a task-workspace
  `KANDEV_DATABASE_PATH` is treated as leaked. Carry that reasoning across.
- `apps/cli/src/kandev-env.ts` — note its caveat that the path-prefix check is a
  defensive secondary signal, is case-sensitive, and does not resolve symlinks;
  `KANDEV_TASK_ID` remains the primary guarantee.
- `apps/cli/src/constants.ts:47-56` — `DEV_KANDEV_DOTDIR`, `devKandevHome`.
- Existing `apps/backend/internal/launcher/constants.go` for the resolve-* naming style.

## Risks

- Do not change `resolveHomeDir` / `resolveDataDir` / `resolveDatabasePath`; `start`,
  `run`, and `service` all depend on their current env-reading behavior.
- `os.UserHomeDir()` can fail; mirror the existing `.kandev` relative fallback rather
  than panicking.
