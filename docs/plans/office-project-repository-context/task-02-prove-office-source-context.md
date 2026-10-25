---
id: "02-prove-office-source-context"
title: "Prove Office launch and creation"
status: complete
wave: 2
depends_on:
  - "01-resolve-project-sources"
plan: "plan.md"
requirements:
  - REQ-TASKS-PROJECT-REPOSITORIES-001
acceptance_criteria:
  - AC-TASKS-PROJECT-REPOSITORIES-001.1
  - AC-TASKS-PROJECT-REPOSITORIES-001.2
  - AC-TASKS-PROJECT-REPOSITORIES-001.5
  - AC-TASKS-PROJECT-REPOSITORIES-001.8
  - AC-TASKS-PROJECT-REPOSITORIES-001.9
system_design:
  - ../../specs/tasks/system-design/project-repository-context.md
---

# Task 02: Prove Office launch and creation

## Summary

Prove that first launch receives project source files and children inherit that context.
Extend existing desktop and phone task creation tests to assert durable repository attachments.

## In scope

- Integration through real task creation, the production Office task starter, orchestrator launch, lifecycle workspace preparation, and workspace-group materialization.
- `local_pc` and Worktree cases with a unique source-file sentinel and environment inventory.
- An `inherit_parent` child that reads the parent's source sentinel.
- Existing Office form E2E scenarios and a narrow public documentation update.

## Out of scope

- UI composition, executor redesign, container providers, project editing, and backfill.

## Acceptance

- `TestOfficeProjectFirstLaunchSources` launches through the production Office starter and real lifecycle adapter, reads source contents from the persisted environment, and checks `task_environment_repos` identity and executor-specific paths.
- Both `project sources attach before launch` browser tests create a project-backed root through the existing form. Assert returned and persisted attachments in the Office workspace.
- An inheriting child launches through the same production path without a supplied parent workspace path and retains source context. For Worktree, assert the shared group records the materialized path.

## Verification

Run backend and browser checks sequentially. The managed browser runner rebuilds
the production assets and backend. Install workspace dependencies once when absent.

```bash
(cd apps/backend && go test -tags fts5 -race -run '^TestOfficeProjectFirstLaunchSources$' ./internal/backendapp -count=1 -v)
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run --project chromium tests/office/new-task-dialog.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/office/mobile-new-task-dialog.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Confirm discovery includes the new scenario in each browser project.
Do not overlap full suites or change worker limits.

## Files likely touched

- `apps/backend/internal/backendapp/office_project_workspace_test.go` (new)
- `apps/backend/internal/backendapp/office_routine_cron_to_session_test.go` (fixture pattern)
- `apps/web/e2e/tests/office/new-task-dialog.spec.ts`
- `apps/web/e2e/tests/office/mobile-new-task-dialog.spec.ts`
- `apps/web/e2e/helpers/office-api-client.ts` (optional source-list argument)
- `docs/public/office-config-sync.md` (short reference note about project source use)
- `docs/plans/office-project-repository-context/plan.md`

## Dependencies

Task 01 must pass before final source-preparation verification.

## Risks

The Office fixture uses a different workspace from the base repository fixture.
Register the source in the Office workspace before project creation.
Mock agent text and a captured launch request cannot prove file materialization.
Use the real preparation path and read the source bytes.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/tasks/requirements/project-repository-context.md)
- [Design launch section](../../specs/tasks/system-design/project-repository-context.md#launch-and-children)
- `newRoutineCronHarness` for task/Office/orchestrator wiring. Its stub manager needs real preparation for file evidence.
- `office-fixture.ts`, `seedData.repositoryPath`, and existing desktop/phone New Task flows.
- `/e2e`, `/mobile-parity`, `/docs-maintainer`, and scoped backend/frontend guidance.

## Results

Completed. `TestOfficeProjectFirstLaunchSources` drives the production Office task starter and orchestrator through a real lifecycle manager backed by a mock ACP agent. It reads the sentinel from the persisted environment for `local_pc` and Worktree, verifies each environment's `task_environment_repos` identity, and launches an `inherit_parent` child without supplying a parent workspace path. The Worktree case verifies the shared group records the prepared path. The integration also exposed and now covers post-launch workspace-group materialization and persistence of the single-repository Worktree path in lifecycle launch results. Desktop and phone New Task E2E scenarios verify returned and persisted repository attachments; the mobile scenario uses fixed project and task titles for deterministic runs. The public Office configuration guide describes inheritance and source failure behavior.

Validation passed:

- `go test -trimpath -tags fts5 -race -run '^TestOfficeProjectFirstLaunchSources$' ./internal/backendapp -count=1 -v`
- `go test -trimpath -tags fts5 -race ./internal/task/service ./internal/backendapp ./internal/orchestrator -count=1`
- `make -C apps/backend build`
- `make -C apps/backend lint`
- `pnpm e2e:run --project chromium tests/office/new-task-dialog.spec.ts` (3 tests)
- `pnpm e2e:run --project mobile-chrome tests/office/mobile-new-task-dialog.spec.ts` (2 tests)
- `pnpm exec prettier --check e2e/tests/office/mobile-new-task-dialog.spec.ts`
- `node --test scripts/validate-public-docs.test.mjs` (62 tests)
- `node scripts/validate-public-docs.mjs`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`

Integration verification on 2026-10-07 retained the dialog suite's per-test
`dialogSeed` fixture when adding project-source coverage. An unadapted merge
failed discovery because that suite does not provide `officeSeed`. The corrected
desktop test still checks returned and persisted attachments in the selected
Office workspace. No product behavior or assertions were removed.

With retries disabled, the managed desktop run of
`tests/office/new-task-dialog.spec.ts` and
`tests/session/provider-interruption-continuation.spec.ts` passed 18 tests.
The matching mobile Office and provider-continuation files passed 3 tests.
Backend race checks passed for executor, orchestrator, worktree, backendapp,
task service, handlers and lifecycle, selecting resume, relocation, Office
project, workspace-field and settled-disconnect regressions. Full web lint,
typecheck, focused E2E lint, catalog/spec lint and all 62 public-doc validation
tests passed. Hosted CI and existing native release gates remain pending.
