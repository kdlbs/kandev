---
id: "02-local-cwd"
title: "Grow Local folder and scratch workspaces"
status: completed
wave: 2
depends_on: ['01-plus-menu']
plan: "plan.md"
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-006
acceptance_criteria:
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-006.1
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-006.2
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-006.3
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-006.4
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-006.7
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-006.8
system_design:
  - ../../specs/tasks/system-design/current-workspace-sources.md
---

# Task 02: Grow Local folder and scratch workspaces

## Summary and scope

Implement the Local flow end to end: zero-repository admission, typed persisted root/placement base, preview for mixed sources, owned child links, authenticated clone reuse, rescan compensation, first-repository projection, and resume/additional-session behavior. Extend the existing UI with UI-02 and preserve all source state on error.

Acceptance: folder-only and mixed batches succeed under user-folder and scratch roots without restart; the first repository enables its own Git surfaces without changing the outer root; failure, cleanup, and resume preserve original files and source targets.

Proposed regression cases: TestAttachWorkspaceSources_RepositorylessMixedBatch; TestLocalWorkspaceSources_PreservesUserRoot; TestLocalWorkspaceSources_ResumesAfterFirstRepository; TestLocalWorkspaceSources_RollbackPreservesTargets. Add real Git coverage for a Git-backed starting folder and nested repository links.

Out of scope: root expansion, native-session replacement, active-turn admission, host-to-remote mounts/sync, and unrelated refactors.

## ASCII UI preview

Relevant views copied from the [combined preview](plan.md#ascii-ui-preview). Required order and capability semantics must match rendered UI; spacing is illustrative.

### UI-02: Local folder or scratch, desktop

Entry: Files > + > Add repositories or folders. The current root may have zero repositories.

```text
+------------------------------------------------------------+
| Add repositories or folders                            [X] |
| Workspace: /home/me/research   Executor: Local               |
| [+ Repository v]  [+ Folder]                               |
|                                                            |
| Repository  [payments-api v]  Branch [main v]           [x] |
| Folder      [/home/me/reference          ] [Browse]     [x] |
|                                                            |
| Where should the sources go?                               |
| (*) Directly inside the current folder                     |
|     Short paths. Adds entries beside your existing files.   |
| ( ) Inside ./kandev/                                        |
|     Groups sources together. Adds one path level.           |
|                                                            |
| Result                                                     |
| research/                         Agent CWD stays here     |
|   notes.md                        Existing file            |
|   payments-api/                   Repository               |
|   reference/ -> /home/me/reference Live folder link         |
|                                                            |
| Session and running processes stay unchanged.               |
| Folder edits affect the original. Agent access rules apply. |
| Parent folder instructions can apply to added sources.      |
|                                        [Cancel] [Add sources]|
+------------------------------------------------------------+
```

Preview uses exact server paths and actual materialization semantics: a Local repository link is labelled as a link too. Names above are illustrative.
Scratch shows its existing scratch path instead of research. No repository prerequisite, Git initialization, or root-expansion selector.
Maps to AC-006.1/.2/.3/.4/.8. Keep the earlier Worktree placement cards for Worktree tasks.

### UI-04: Phone

```text
Files   /workspace/...   [+] [...]
                         | New file
                         | Upload files
                         | Upload folder
                         | Add repositories or folders

+----------------------------------+
| Add repositories or folders  [X] | fixed header
| Local / research                 |
|----------------------------------|
| [+ Repository v] [+ Folder]      | one scroll body
| [payments-api v]                 |
| Branch [main v]                  |
| [/home/me/reference] [Browse]    |
|                                  |
| (*) Current folder               |
|     Short paths; more entries.   |
| ( ) Inside ./kandev/             |
|     Grouped; longer paths.       |
|                                  |
| Result and unchanged CWD         |
| Folder links edit original files.|
| Access rules still apply.        |
|----------------------------------|
| [Cancel]          [Add sources]  | fixed safe-area footer
+----------------------------------+
```

Use the existing full-height source drawer for the form, not a compressed desktop dialog. Dynamic viewport height, vertical-only content, wrapping paths, visible disabled reasons, >=44px touch targets, and no page overflow are required. Return focus to + after close. Remote phones use UI-03 capabilities in this same composition.
Maps to AC-005.3 and AC-006.1/.5/.6/.7. The menu/dialog transition must not steal focus or close the new surface.

## Verification

Use TDD for changed behavior. Commands run from repository root; install workspace dependencies from apps if absent. E2E requires built assets and the existing guarded runner. Record actual results and environment blockers; do not mark unavailable provider checks passed.

```bash
(cd apps/backend && go test ./internal/task/service ./internal/task/repository/... ./internal/backendapp ./internal/worktree ./internal/agent/runtime/lifecycle)
(cd apps/web && pnpm exec vitest run components/task/add-workspace-sources)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
make build-web
make build-backend
(cd apps/web && pnpm e2e:run --host --project chromium tests/task/add-workspace-sources.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/task/mobile-add-workspace-sources.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/task/service/service_workspace_sources.go`
- `apps/backend/internal/task/service/workspace_source_placement.go`
- `apps/backend/internal/task/models`
- `apps/backend/internal/task/repository/sqlite`
- `apps/backend/internal/backendapp/workspace_source_materializer.go`
- `apps/backend/internal/agent/runtime/lifecycle/env_preparer_local.go`
- `apps/backend/internal/agent/runtime/lifecycle/workspace_sources_reconcile.go`
- `apps/web/components/task/add-workspace-sources`
- `apps/web/lib/types/http.ts`
- `apps/web/e2e/tests/task/add-workspace-sources.spec.ts`

Also update affected locale catalogs and focused tests beside changed code. Read scoped AGENTS.md before implementation. Use docs-maintainer for public documentation changes.

## Dependencies

01-plus-menu. Read the requirements, current-workspace design, and earlier placement package. Do not clear its expansion gate.

## Risks

See the package risks; verify ownership and effective root before filesystem changes. No automatic rebind is permitted for unchanged-CWD additions.

## Parallelism

`sequential`

## Inputs

[Requirements](../../specs/tasks/requirements/attach-workspace-sources.md), [design](../../specs/tasks/system-design/current-workspace-sources.md), existing attachment tests, and the assigned previews.

## Results

Implemented. Local folder and scratch workspaces preserve their established root and CWD while
repository, folder, and mixed batches are linked into that root. Root-origin and relative destination
metadata persist through reload, resume, and additional sessions. Rollback restores the prior root
and rescan state without rebinding the session, and the first repository enables repository-aware
projections without manufacturing a new outer repository.

Verification passed:

- `go test ./internal/task/service ./internal/task/repository/sqlite ./internal/orchestrator/executor` and the changed lifecycle/backendapp packages passed.
- Focused source-picker, placement, dialog, file-tree, and toolbar tests passed, 43 tests across five files.
- Desktop and mobile Local folder, scratch, mixed-batch, retry, resume, and focus flows passed.
- Git staging protection, persistence, rollback, and additional-session coverage passed in the backend tests.

## Dialog refinement (2026-09-16)

Desktop uses a viewport-constrained 960px dialog and full-width stacked placement rows. Source selection comes first, followed by locations and impact information. Only selected root expansion shows the prominent restart warning; unchanged-CWD additions show a short neutral continuity note. Use Source location for repository or folder placement and omit expansion for folder-only batches. The phone retains its full-height drawer and fixed footer. These refinements apply to the saved UI-02/UI-03/UI-04 previews.
