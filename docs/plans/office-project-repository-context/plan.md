---
created: 2026-10-07
status: complete
requirements:
  - REQ-TASKS-PROJECT-REPOSITORIES-001
system_design:
  - ../../specs/tasks/system-design/project-repository-context.md
legacy_specs: []
---

# Implementation plan: Office project repository context

## Overview

Resolve project repositories during root task creation, before the first Office wakeup.
Implement the shared creation boundary first. Then prove source preparation and
desktop/phone creation through real endpoints.

Issue: [kdlbs/kandev#4289](https://github.com/kdlbs/kandev/issues/4289).
Assigned GitHub user: `carlosflorencio`.

The task system owns creation and launch context. Office provides project configuration.
Existing Office design already describes inheritance, but lacks creation-time acceptance criteria.
The new requirement supplies those criteria without changing executor selection.

## Diagnosis and evidence

Confirmed cause: `Service.prepareTaskForCreation` reads parent attachments but
never reads a root task's project sources. Office's dialog only sends `project_id`.

The temporary `TestReproIssue4289RootProjectRepositories` seeded real Office
tables, a project, and a registered Git checkout containing `README.md`.
The resulting task had zero attachments.

```bash
(cd apps/backend && go test -tags fts5 -run '^TestReproIssue4289RootProjectRepositories$' ./internal/task/service/ -count=1 -v)
```

Observed result: FAIL at the attachment assertion.
The failure was `task attachments = []*models.TaskRepository(nil); want repo-4289`.
The temporary reproduction was removed after diagnosis; its failure is now covered by the permanent regressions below.

## Scope

### In scope

- Root project defaults through the shared task service.
- Registered/unregistered local Git sources and existing supported remote URL resolution.
- Source ordering, deduplication, ownership, and failure handling.
- Preservation of explicit sources, parent context, and existing executor behavior.
- Regression, wiring, first-launch, inherited-child, desktop, and phone evidence.

### Out of scope

- Backfilling existing tasks or changing attachments when a project changes.
- Changing executor preference precedence or forcing `local_pc` to use Worktree.
- New remote providers, plain-folder project support, schema changes, or issue #3227 remediation.
- UI composition, new controls, localization changes, and unrelated refactors.

## Technical approach

Task 01 adds the neutral project reader and selection helper to
`internal/task/service`. A backendapp adapter reads Office projects and is wired
into the shared task service. The helper feeds normal repository preflight and
resolution before creation publication. Every source passes the existing
resolver before automatic selections deduplicate by resolved repository ID.
Explicit multi-branch selections keep their existing duplicate rules.

Automatic defaults apply to root requests with nil repositories and no explicit
workspace source. Explicit empty arrays bypass defaults. Parent inheritance wins
for all subtasks, including parents without repositories.

The [system design](../../specs/tasks/system-design/project-repository-context.md#resolution)
contains source compatibility and refusal behavior. No provider credential path changes.

Task 02 exercises actual source preparation and inherited workspaces. It extends
the existing Office New Task browser scenarios without changing their layout.
No ASCII UI preview is required because rendered UI is unchanged.

## Tests

Planned names are new regression tests, not completed evidence.

| Criteria | File and test |
| --- | --- |
| .1, .2 | `service_project_repositories_test.go`: `TestCreateTaskRootProjectRepositories` verifies attachments exist before `task.created` publication |
| .2, .3 | Same file: `TestCreateTaskProjectSourcesReuseDeduplicateAndResolve`, including aliases, multiple repositories, defaults, and supported remote inputs; `TestCreateTaskProjectSourcesDeduplicateLocalRemoteRepositoryAliases` covers both alias orders |
| .4, .5, .7 | Same file: `TestCreateTaskProjectRepositorySourceSelectionPrecedence`, with explicit subset, empty list, workspace path/group, inherited child, repositoryless parent, empty project, and no project |
| .6 | Same file: `TestCreateTaskProjectRepositorySourceFailuresPrecedeTaskCreation`, with missing reader/project, read/decode errors, foreign workspace, unsupported URL, mixed valid/invalid sources, and both GitLab trusted/untrusted origin orders; asserts no task row or `task.created` event |
| .1, .6 | `backendapp/office_project_repositories_test.go`: `TestOfficeProjectRepositoryReaderWiring`, through the real composition seam |
| .8 | `backendapp/office_project_workspace_test.go`: `TestOfficeProjectFirstLaunchSources`, with `local_pc`, Worktree, and inheriting child cases |

All abbreviated criteria use prefix `AC-TASKS-PROJECT-REPOSITORIES-001`.
Existing external-ID retry and subtask regressions run with the affected package tests.

## E2E tests

Extend `apps/web/e2e/tests/office/new-task-dialog.spec.ts` and
`mobile-new-task-dialog.spec.ts` with `project sources attach before launch`.
Map both to .1, .2, and .9. Register the fixture repository in
`officeSeed.workspaceId`, not the distinct `seedData.workspaceId`.

Create a project with that source, select it through the existing form, and
assert attachment IDs on the creation response and the persisted task.
The first-launch integration proves file contents under .8.
Browser success alone does not prove materialization.

## Work orders

- [x] [Task 01: Resolve project repository defaults](task-01-resolve-project-sources.md) (complete)
- [x] [Task 02: Prove Office launch and creation](task-02-prove-office-source-context.md) (complete)

Execute Task 01 before Task 02. Both are sequential. No delegation is authorized.
Exact product verification commands are in each work order.

## Verification results

- Diagnostic reproduction: failed as expected with zero attached repositories; covered by `TestCreateTaskRootProjectRepositories`.
- Temporary reproduction cleanup: complete.
- `python3 scripts/list-docs.py validate`: passed (360 decisions, 1427 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `.github/scripts/pr-docs.cjs.validateCoverage`: passed for both work orders, using the planned task-service change as the coverage trigger.
- `git diff --check -- docs/specs docs/plans/office-project-repository-context`: passed.
- `git status --short`: five new package files and one Office design link, all unstaged.
- GitHub issue assignment: confirmed as `carlosflorencio`.
- `go test -trimpath -tags fts5 -race ./internal/task/service ./internal/backendapp -count=1`: passed after mapping two legacy fake projects to their actual workspaces in tests.
- `go test -trimpath -tags fts5 -race -run '^TestOfficeProjectFirstLaunchSources$' ./internal/backendapp -count=1 -v`: passed.
- Review remediation: source validation precedes automatic repository-ID deduplication; GitLab HTTPS/HTTP mixed origins fail in both orders, and local-path/remote-URL aliases attach one registered repository in both orders.
- PR review remediation: supported scheme-less host/path URLs such as `github.com/acme/api` now pass through the existing provider parser and attach normally. Foreign workspace IDs are omitted from task creation errors.
- PR review cleanup: the test traceability table and work-order source path match the implementation; public docs describe invalid-source and unreadable-project failure behavior; the mobile source test uses deterministic titles and the Windows-path test uses a realistic raw string.
- The create/update request handlers share repository-field conversion while preserving omitted versus explicit-empty behavior at their service boundaries.
- Review remediation: `TestOfficeProjectFirstLaunchSources` now uses the production Office task starter, orchestrator, lifecycle manager, and HandoffService; it checks persisted `task_environment_repos`, the Worktree group path, and an inherited child with no supplied parent workspace path.
- Review remediation required materializing workspace groups after executor preparation and returning the prepared single-repository Worktree path through the lifecycle adapter.
- `go test -trimpath -tags fts5 -race ./internal/task/service ./internal/backendapp ./internal/orchestrator -count=1`: passed (100.5s, 114.8s, and 163.1s respectively).
- `make -C apps/backend build`: passed for the backend and its bundled Go tools.
- `make -C apps/backend lint`: passed after lowercasing the Office reader error strings flagged by staticcheck.
- `go test -trimpath -tags fts5 ./internal/task/handlers -count=1`: passed, including the HTTP nil-versus-empty repository request regressions and shared conversion helper.
- `go test -trimpath -tags fts5 -race ./internal/task/service -count=1`: passed after PR review remediation.
- `go test -trimpath -tags fts5 -race ./internal/mcp/handlers -count=1`: passed after making project-backed legacy fixtures explicit about their empty repository selection.
- `make -C apps/backend build`: passed after PR review remediation.
- `golangci-lint run ./... --new-from-rev=330e02a47808c11ca315ae30456fcce7f4806db5 --timeout=5m`: passed with 0 issues after PR review remediation.
- `pnpm exec prettier --check e2e/tests/office/mobile-new-task-dialog.spec.ts`: passed.
- `pnpm e2e:run --project chromium tests/office/new-task-dialog.spec.ts`: passed again after the shared handler conversion refactor (3 tests).
- `pnpm e2e:run --project mobile-chrome tests/office/mobile-new-task-dialog.spec.ts`: passed after deterministic-title and formatting updates (2 tests).
- `pnpm e2e:run --project chromium tests/office/new-task-dialog.spec.ts`: passed (3 tests); backend and Vite production assets built.
- `pnpm e2e:run --project mobile-chrome tests/office/mobile-new-task-dialog.spec.ts`: passed (2 tests); backend and Vite production assets built.
- `node --test scripts/validate-public-docs.test.mjs`: passed (62 tests).
- `node scripts/validate-public-docs.mjs`: passed (47 published docs pages).
- `python3 scripts/list-docs.py validate`: passed (360 decisions, 1427 specifications).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed after remediation.
- Implementation verification: complete. The user authorized implementation on 2026-10-07.

## Risks

- Supported remote URLs depend on existing provider preflight. Do not add a fallback that treats unsupported URLs as paths.
- Existing tests use fake project IDs. Adapt their fixtures without weakening production missing-project errors.
- Some creation adapters can serialize empty arrays. Verify payload semantics before assuming an omitted selection.
- `local_pc` uses the source checkout. Isolation requires the existing Worktree executor choice.
- A first-launch failure after attachment needs fresh diagnosis before adding executor changes.
