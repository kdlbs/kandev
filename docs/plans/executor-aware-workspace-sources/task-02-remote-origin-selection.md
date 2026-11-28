---
id: "02-remote-origin-selection"
title: "Clone local repository origins remotely"
status: done
wave: 2
depends_on: ['01-host-folder-policy']
plan: plan.md
requirements:
  - REQ-TASKS-MIXED-REPOSITORIES-006
acceptance_criteria:
  - AC-TASKS-MIXED-REPOSITORIES-006.4
  - AC-TASKS-MIXED-REPOSITORIES-006.5
  - AC-TASKS-MIXED-REPOSITORIES-006.6
  - AC-TASKS-MIXED-REPOSITORIES-006.7
  - AC-TASKS-MIXED-REPOSITORIES-006.8
  - AC-TASKS-MIXED-REPOSITORIES-006.9
  - AC-TASKS-MIXED-REPOSITORIES-006.10
system_design:
  - ../../specs/tasks/system-design/mixed-repository-selection.md
---

# Task 02: Clone local repository origins remotely

## Summary and scope

Deliver origin inspection, explicit remote clone selection and authoritative creation validation as one vertical slice. Local candidate rows remain selectable remotely only after origin eligibility is known.

- Add the proposed read-only workspace-scoped inspection route/action from the design, service-owned origin/ref validation, and API/domain hook with stale-response fencing.
- Extend workspace_sources with the explicit remote-origin intent through HTTP/WS, DTO/service, payload and settings adapters. Preserve omission compatibility and reject contradictory/forged inputs.
- Resolve actual origin and provider credentials server-side; compare expected origin and selected remote branch again on creation before side effects.
- Display Clone from remote in candidates and selected chips; disable no-origin rows; show remote refs and block local-only branch selections without silently changing them.
- Preserve original checkout provenance across executor switches. Apply the same behavior to set-expanded, preset, restored, and editable subtask rows.
- Extend Task 01 browser specs and add real container-project clone evidence with local-only work absent. Update API/public docs and locale catalogs.

## Out of scope

No new executor capabilities, remote host-folder copying, pushing local changes,
running-task redesign, commits, publishing, or delegation.

## Acceptance

- The assigned ACs pass through real creation callers, not only isolated helpers.
- The assigned previews match desktop and phone rendering, including recovery paths.
- Existing source identity, branch choices, locked context, and cleanup guarantees remain intact.

## ASCII UI preview

See the [complete preview](plan.md#ascii-ui-preview). The relevant views follow.

### UI-03: Remote executor and local-origin choices

Entry: SSH selected, Add menu, then Repository > Local.

```text
[Executor: SSH v]
[+ Add Repository/Folder v]
+------------------------------------------------------+
| Repository                                         > |
| Local Folder                              [disabled] |
| Local folders require a Local or Worktree executor.   |
| Repository Set                                     > |
+------------------------------------------------------+

+------------------------------------------------------+
| < Back / Repository                                  |
| Local | GitHub | Bitbucket | Azure                    |
| ------                                               |
| [Search repositories...                            ] |
|------------------------------------------------------|
| api                      Clone from remote           |
| github.com/acme/api                                  |
|                                                      |
| offline-project                           [disabled] |
| No usable remote origin.                             |
|                                                      |
| another-project                           [checking] |
| Checking remote origin...                            |
+------------------------------------------------------+

After selecting api:
[Repo: api | Clone from remote | main v | x] [+ Add]
[Prompt...]
[Agent v] [Workflow v] [Executor: SSH v]
Repositories will be cloned from their remote origins.
Uncommitted changes and unpushed commits are not included.
                                         [Start task]
```

Maps to 006.4-006.6, 006.8-006.9. Provider tabs still require configured/enabled/tested
integrations. The Local tab names where the candidate was found; the explicit
label identifies how it will run. Unsupported sources are disabled, not hidden.
Search/header remain fixed; results alone scroll. No local-only branch auto-fallback.

### UI-04: Retained incompatible selections

Entry: switch a mixed host workspace to SSH, or retain an unpushed branch.

```text
[Repo: api | Clone from remote | feature/local-only v | x]
This branch is not available remotely. [Choose remote branch]
[Folder: ~/design-assets | x]  [+ Add]
Local folders require a Local or Worktree executor.
[Executor: SSH v]                         [Start: disabled]
```

Maps to 006.6-006.8. Errors are per row. Switching back to a supported host executor
restores valid checkout behavior. Removing incompatible rows or selecting a remote
branch enables Start when all other validation passes. Origin changes show a
refresh-required row error; access failures show Retry/settings without losing the
selection. These are resolved error states, never indefinite executor waiting text.

### UI-05: Phone source navigation and recovery

Entry: phone New Task, then Add under the chosen executor.

```text
+--------------------------------------+
| [Repo: api                        x] |
| [Clone from remote | main v]         |
| [+ Add]                             |
| [Prompt...                        ] |
| [Agent v] [Workflow v] [SSH v]       |
| Clone remote contents only.         |
| Local uncommitted/unpushed work     |
| is not included.                    |
| [Start task]                        |
+--------------------------------------+

+--------------------------------------+
| Add to workspace                 x  |
| Repository                       >  |
| Local Folder             [disabled] |
| Requires Local or Worktree.         |
| Repository Set                   >  |
+--------------------------------------+

+--------------------------------------+
| < Back / Repository                 |
| Local | GitHub | ...                |
| [Search...                        ] |
|--------------------------------------|
| api                                |
| Clone from remote                  |
| github.com/acme/api                 |
|                                    |
| offline-project         [disabled] |
| No usable remote origin.           |
+--------------------------------------+
```

Maps to 006.9 plus 006.1-006.8. With Worktree, the same folder option is enabled
and navigates within this sheet to the folder picker. Reuse MobilePickerSheet:
inset bottom drawer, fixed navigation/search, one vertical results scroller,
dynamic viewport sizing and safe-area clearance. Back replaces the sheet body;
selection returns focus to Add. No stacked popovers. Recovery is visible beside
rows, not tooltip-only. At least 44px touch targets; no page horizontal overflow.
Desktop menus use the surrounding 12px text and normal compact controls. Copy in
these sketches is illustrative except approved labels; all copy uses five locales.

## Verification

Use TDD for changed logic. Commands run from the repository root; install workspace
dependencies once if missing. New test files in the plan are deliverables of this
work order. Record actual results; do not reuse predecessor counts.

```bash
(cd apps/backend && go test -tags fts5 ./internal/task/... ./internal/user/...)
(cd apps/backend && go test -tags fts5 ./internal/orchestrator/executor -run 'WorkspaceSources|WorkspaceFolders|RemoteOrigin')
(cd apps/web && pnpm exec vitest run hooks/domains/repositories/use-repository-clone-source.test.tsx components/task-create-dialog-executor-source-policy.test.ts components/task-create-dialog-executor-source-menu.test.tsx components/task-create-dialog-workspace-defaults.test.tsx components/task-create-dialog-workspace-contents.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check)
make build-web
make build-backend
(cd apps/web && pnpm e2e:run --no-build --project chromium -- e2e/tests/task/create-task-executor-sources.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome -- e2e/tests/task/mobile-create-task-executor-sources.spec.ts)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --no-build --project containers -- e2e/tests/task/create-task-executor-sources-container.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

The clone-source hook and its `hooks/domains/repositories/` location are proposed
new files; reuse an existing repository-domain location if present at execution.

- `apps/backend/internal/task/handlers/`
- `apps/backend/internal/task/dto/`
- `apps/backend/internal/task/service/service_branches.go`
- `apps/backend/internal/task/service/repository_selection.go`
- `apps/backend/internal/task/service/service_workspace_creation.go`
- `apps/backend/internal/task/service/service_requests.go`
- `apps/backend/internal/user/models/models.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/pkg/api/v1/task.go`
- `apps/web/lib/types/http.ts`
- `apps/web/lib/types/http-workspace-sources.ts`
- `apps/web/lib/types/http-user-settings.ts`
- `apps/web/lib/api/`
- `apps/web/hooks/domains/repositories/`
- `apps/web/components/task-create-dialog-repository-picker.tsx`
- `apps/web/components/task-create-dialog-helpers.ts`
- `apps/web/components/task-create-dialog-workspace-defaults.ts`
- `apps/web/e2e/playwright.config.ts`
- `apps/web/e2e/tests/task/`
- `docs/public/`

## Dependencies

Task 01. Extend its existing E2E files rather than creating duplicate UI coverage.

## Risks

Host-origin access does not prove executor reachability. Use real remote clone evidence, preserve private-origin credential boundaries and report unavailable container infrastructure as a verification gap.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/mixed-repository-selection.md), requirement 006.
- [Design](../../specs/tasks/system-design/mixed-repository-selection.md), executor-aware source policy.
- Existing workspace-contents creation and executor workspace-source tests.
- Plan test matrix and shared desktop/mobile previews.

## Results

Implemented server-owned local-origin inspection over HTTP and WebSocket, bounded
and generation-fenced frontend inspection, remote branch selection, persisted clone
intent, authoritative creation reinspection, HTTP/WS serializer parity, selected-row
submission blocking, and recovery copy for unavailable origins.

Verification passed on 2026-09-15:

- Task service tests: `go test -tags fts5 ./internal/task/service`.
- Task handler and dto tests: `go test -tags fts5 ./internal/task/handlers ./internal/task/dto`.
- Web clone-source hook, source-policy, repository-picker, and complete task-create
  unit suite: 55 files and 706 tests.
- Backend lint/build and web typecheck/lint/i18n/build checks.
- A real-container clone transport test remains deferred because the standard E2E
  fixture has only an ineligible `file://` origin and no reachable credentialed Git
  origin. The server authority and clone eligibility paths are covered by focused
  service and handler tests.
