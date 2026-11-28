---
id: "03-restore-workspace-defaults"
title: "Restore complete last-used contents"
status: complete
wave: 3
depends_on: ['02-unified-add-ui']
plan: plan.md
requirements:
  - REQ-TASKS-MIXED-REPOSITORIES-004
  - REQ-TASKS-MIXED-REPOSITORIES-005
acceptance_criteria:
  - AC-TASKS-MIXED-REPOSITORIES-004.4
  - AC-TASKS-MIXED-REPOSITORIES-005.1
  - AC-TASKS-MIXED-REPOSITORIES-005.2
  - AC-TASKS-MIXED-REPOSITORIES-005.3
  - AC-TASKS-MIXED-REPOSITORIES-005.4
  - AC-TASKS-MIXED-REPOSITORIES-005.5
system_design:
  - ../../specs/tasks/system-design/mixed-repository-selection.md
---

# Task 03: Restore complete last-used contents

## Summary

Persist the complete successful contents selection and restore it in the next task dialog. Preserve workspace-specific workflow memory and existing agent/executor settings; distinguish saved empty from missing preferences.

## In scope

- Extend backend user model/DTO, targeted store/service updates, HTTP/WS recorders, boot/settings projection, and frontend state types with workspace-scoped full source snapshots.
- Save successful normalized contents and branches, including source provenance, policy choices and explicit empty. Preserve unrelated settings and other workspace snapshots under concurrent writes.
- Extend untouched-only hydration, preset normalization and reopen logic. Current edits win even when settings arrive late; cancelled/failed create does not update defaults.
- Revalidate saved folders, repository IDs, provider readiness and branches; retain invalid rows with recovery/removal rather than silently selecting scratch.
- Keep legacy single-repo fallback only when no new snapshot exists. Keep inherited New Subtask context ahead of fresh-task defaults.
- Extend desktop/phone E2E with last-used restoration and race cases; reconcile plan/spec/public docs without rewriting historical results.

## Out of scope

No commits, publishing, new executors, or live-task attachment redesign. Preserve the preceding work order contracts; do not add folder copying or set persistence for folders.

## Acceptance

- A successful mixed task restores all contents in order with branches, workflow and profiles; a successful empty task restores empty.
- Late defaults cannot overwrite edits or presets, and unavailable saved sources remain identifiable and block invalid submission.
- Cross-workspace snapshots and unrelated settings survive targeted concurrent updates; legacy records remain readable.

## ASCII UI preview

See the [full combined preview](plan.md#ascii-ui-preview).

### UI-01: Desktop restored contents and Add menu

Entry: New Task with last-used sources restored, then click Add.

```text
+--------------------------------------------------------------------+
| New task                                                       x   |
| [Repo: kandev | Local  | feature/ux v | x]                           |
| [Repo: api    | GitHub | main v       | x]                           |
| [Folder: ~/projects/design-assets    | x]  [+ Add v]                |
|                                           +--------------------+   |
|                                           | Repository       > |   |
|                                           | Local Folder     > |   |
|                                           | Repository Set   > |   |
|                                           +--------------------+   |
| +----------------------------------------------------------------+ |
| | Write a prompt for the agent...                                | |
| +----------------------------------------------------------------+ |
| [Claude v]  [Development workflow v]  [Executor v]                  |
| Workspace contents will be available side by side.   [Start task]  |
+--------------------------------------------------------------------+
```

Maps to 004.1-004.3 and 005.1. Content chips wrap, folders have no branches.
The bottom helper describes the effective executor. Names and non-scratch helper
wording are illustrative. Menu order, chip source distinctions, and placement are
required. Set application appends missing members; it does not replace the draft.

### UI-03: Empty, loading, and recovery

Entry: remove the final folder/repository, or restore a saved empty snapshot.

```text
+--------------------------------------------------------------------+
| [+ Add Repository/Folder v]                                        |
|                                                                    |
| +----------------------------------------------------------------+ |
| | Write a prompt for the agent...                                | |
| +----------------------------------------------------------------+ |
| [Claude v]  [Development workflow v]  [Executor v]                  |
| An empty scratch workspace will be created.          [Start task]  |
+--------------------------------------------------------------------+
```

Maps to 004.2, 004.4, 005.2-005.4. No top scratch text, empty repository chip,
standalone Sets button, or separate exclusive folder control. Clearing contents
does not reset workflow/agent settings. Delayed hydration cannot undo removal.

```text
[+ Add Repository/Folder v]   Loading saved selections...
...
[Start task: disabled until initial defaults are resolved]

[Folder: ~/missing-assets | x] [+ Add v]
Folder unavailable. [Choose another folder] [Retry]
...
[Start task: disabled]

[+ Add v]
+-----------------------------------------------+
| Repository                                  > |
| Local Folder                       [disabled] |
| Available with a local executor.              |
| Repository Set                              > |
+-----------------------------------------------+
```

Loading has no false scratch claim. An intentional edit supersedes loading and
releases that defaults gate. Missing saved repositories/branches and disconnected
providers use analogous row recovery; no silent row removal. Folder browser errors
show Retry and retain the path. Empty local/provider/set lists keep navigation and
show their existing empty messages. An executor switch preserves invalid rows.

### UI-04: Phone contents and single-sheet navigation

Entry: New Task on a phone, then tap Add.

```text
+----------------------------------+
| New task                     x   |
| [Repo: kandev                x]  |
| [Local | feature/ux v]           |
| [Folder: ~/design-assets     x]  |
| [+ Add]                         |
| [Write a prompt...          ]   |
| [Claude v] [Workflow v]          |
| Workspace contents side by side.|
| [Start task]                    |
+----------------------------------+

+----------------------------------+
| Add to workspace             x  |
| Repository                   >  |
| Local Folder                 >  |
| Repository Set               >  |
+----------------------------------+

+----------------------------------+
| < Back          Local Folder    |
| Path: [~/design-assets        ] |
|----------------------------------|
| < Parent folder                 |
| [Folder] brand                  |
| [Folder] screenshots            |
|                                 |
|----------------------------------|
| [Add this folder]               |
+----------------------------------+
```

Maps to 004.7, 003.4, 004.3. Repository tabs/search, set search, branch, and URL
selection are sibling views within this same sheet. Back returns to the menu or
owning row; selection closes the picker and returns focus to Add. Use the shipped
`MobilePickerSheet` fixedContent/header/body pattern: an inset bottom drawer fits
these temporary choices. One internal results scroller, dynamic viewport height,
safe-area footer, and keyboard clearance. Touch targets at least 44px; normal
fine-pointer desktop controls 28px. Only the provider strip may scroll horizontally.
Desktop/phone share selection state, eligibility, serialization, and error logic.

Phone empty state uses the same hierarchy with `+ Add Repository/Folder` above the
prompt and the scratch sentence below settings, immediately before Start task.
There is no extra repository-count management step.

## Verification

Run from the repository root. The listed tests are the implementation evidence for
this work order. Install workspace dependencies once if missing.

```bash
(cd apps/backend && go test -tags fts5 ./internal/user/... ./internal/task/handlers/...)
(cd apps/web && pnpm exec vitest run components/task-create-dialog-workspace-defaults.test.tsx components/task-create-dialog-workspace-contents.test.tsx components/task-create-dialog-repository-selection.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check)
make build-web
make build-backend
(cd apps/web && pnpm e2e:run --no-build --project chromium -- e2e/tests/task/create-task-workspace-contents.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome -- e2e/tests/task/mobile-create-task-workspace-contents.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

- `apps/backend/internal/user/models/models.go`
- `apps/backend/internal/user/dto/dto.go`
- `apps/backend/internal/user/store/sqlite.go`
- `apps/backend/internal/user/service/service.go`
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/handlers/task_ws_handlers.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/web/lib/state/slices/settings/types.ts`
- `apps/web/components/task-create-dialog-form-reset.ts`
- `apps/web/components/task-create-dialog-repository-autopick.ts`
- `apps/web/components/task-create-dialog-repositories-state.ts`
- `apps/web/components/task-create-dialog-state.ts`
- `apps/web/e2e/tests/task/`
- `docs/plans/workspace-contents-creation/`

## Dependencies

Complete Task 02 first.

## Risks

Empty arrays can disappear through omitempty or truthiness checks. Save canonical host source paths, never generated runtime paths. Loading/preset races must use the actual reducer integration, not static mocked touched flags.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/mixed-repository-selection.md), listed IDs.
- [Design](../../specs/tasks/system-design/mixed-repository-selection.md), creation extension.
- Existing mixed selection and attachment tests; plan Tests/E2E tables map scenarios.

## Results

Implemented workspace-scoped complete last-used source snapshots, targeted store
updates, source provenance and branch restoration, untouched-only hydration, and
legacy/inherited compatibility. The public guide and paired specifications now
describe the shipped behavior.

Verification passed on 2026-09-14:

- `go test -tags fts5 ./internal/user/... ./internal/task/handlers/...` passed.
- The exact focused Vitest command in this work order passed 3 files and 14 tests.
- Web typecheck, lint, i18n checks, and `make build-web` passed.
- `make build-backend` and both workspace-contents E2E specs passed.
- Specification catalog/lint, public-doc validation, and `git diff --check` passed.
