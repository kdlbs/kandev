---
id: "02-unified-add-ui"
title: "Unified Add menu and contents UI"
status: complete
wave: 2
depends_on: ['01-create-workspace-sources']
plan: plan.md
requirements:
  - REQ-TASKS-MIXED-REPOSITORIES-001
  - REQ-TASKS-MIXED-REPOSITORIES-002
  - REQ-TASKS-MIXED-REPOSITORIES-003
  - REQ-TASKS-MIXED-REPOSITORIES-004
acceptance_criteria:
  - AC-TASKS-MIXED-REPOSITORIES-001.4
  - AC-TASKS-MIXED-REPOSITORIES-003.1
  - AC-TASKS-MIXED-REPOSITORIES-003.2
  - AC-TASKS-MIXED-REPOSITORIES-003.4
  - AC-TASKS-MIXED-REPOSITORIES-004.1
  - AC-TASKS-MIXED-REPOSITORIES-004.2
  - AC-TASKS-MIXED-REPOSITORIES-004.3
  - AC-TASKS-MIXED-REPOSITORIES-004.4
  - AC-TASKS-MIXED-REPOSITORIES-004.5
  - AC-TASKS-MIXED-REPOSITORIES-004.6
  - AC-TASKS-MIXED-REPOSITORIES-004.7
  - AC-TASKS-MIXED-REPOSITORIES-004.8
system_design:
  - ../../specs/tasks/system-design/mixed-repository-selection.md
---

# Task 02: Unified Add menu and contents UI

## Summary

Deliver the approved desktop and mobile Add flow over the new creation contract. Users mix folders, repositories and sets without clearing anything; zero contents produces scratch with the exact bottom hint.

## In scope

- Add folder variant and complete-contents operations to the reducer, payload builder, API types, reset state, and New Subtask adapter.
- Replace exclusive folder UI and standalone Sets trigger with the contextual Add button and Repository / Local Folder / Repository Set menu.
- Reuse eligible provider picker, public URL/PR entry, set dedupe, native folder dialog, and host directory browser. Extract controlled picker bodies for same-sheet mobile navigation.
- Count valid selected identities for the button label; no empty placeholder. Folder rows do not trigger Git branch queries or count against repository-only limits.
- Derive bottom executor hint from complete source composition; scratch text appears there only, not at top. Show incompatible combinations and path errors without dropping input.
- Preserve fresh-branch rules, locked inheritance, explicit empty editable subtasks, and provider readiness recovery.
- Add all copy in five locale catalogs; update public task creation instructions through docs-maintainer. Run focused desktop and mobile visual/E2E evidence.

## Out of scope

No commits, publishing, new executors, or live-task attachment redesign. Preserve the preceding work order contracts; do not add folder copying or set persistence for folders.

## Acceptance

- UI-01 through UI-04 match their structural contract, including exact Add labels, menu order, bottom hint and one phone sheet.
- Every Add path appends without changing siblings; cancel leaves the draft; remove-all produces an explicit empty create request.
- Both desktop and phone submit mixed sources and editable subtasks end to end with correct readback, errors, and touch/focus behavior.

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

### UI-02: Desktop picker views

Entry: choose each item from UI-01. Each picker replaces the menu.

```text
+----------------------------------------------------+
| < Add / Repository                                 |
| Local | GitHub | Bitbucket | Azure                  |
| ------                                             |
| [Search repositories...                         ]  |
|----------------------------------------------------|
| kandev                   ~/projects/kandev          |
| website                  ~/projects/website         |
|----------------------------------------------------|
| Paste repository URL                               |
+----------------------------------------------------+

+----------------------------------------------------+
| < Add / Local Folder                               |
| Path: [~/projects/design-assets                  ]  |
|----------------------------------------------------|
| < Parent folder                                    |
| [Folder] brand                                     |
| [Folder] screenshots                               |
|----------------------------------------------------|
|                                  [Add this folder] |
+----------------------------------------------------+

+----------------------------------------------------+
| < Add / Repository Set                             |
| [Search sets...                                 ]  |
|----------------------------------------------------|
| Full stack           api, website    [Add 2 repos]  |
| Documentation        docs, examples  [Add 2 repos]  |
+----------------------------------------------------+
```

Maps to 004.3, 004.6 and existing 002.1-002.7. Only eligible providers appear;
provider names here are examples. Selection appends and returns to the form.
Back returns to the menu. Cancel changes nothing. Header/search and folder action
stay fixed; one results body scrolls. Native desktop folder selection may use the
existing OS dialog. New browser folder selection must not require a second trigger.

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
(cd apps/web && pnpm exec vitest run components/task-create-dialog-workspace-contents.test.tsx components/task-create-dialog-repository-selection.test.ts components/task-create-dialog-repository-picker.test.tsx components/task-create-dialog-options.test.tsx components/folder-picker.test.tsx components/task/use-subtask-submit.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check)
make build-web
make build-backend
(cd apps/web && pnpm e2e:run --no-build --project chromium -- e2e/tests/task/create-task-workspace-contents.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome -- e2e/tests/task/mobile-create-task-workspace-contents.spec.ts)
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

- `apps/web/components/task-create-dialog-types.ts`
- `apps/web/components/task-create-dialog-repositories-state.ts`
- `apps/web/components/task-create-dialog-mixed-repository-chips.tsx`
- `apps/web/components/task-create-dialog-mixed-repository-chips-surfaces.tsx`
- `apps/web/components/task-create-dialog-repository-picker.tsx`
- `apps/web/components/task-create-dialog-options.tsx`
- `apps/web/components/folder-picker.tsx`
- `apps/web/components/task-create-dialog-helpers.ts`
- `apps/web/components/task-create-dialog-prop-builders.ts`
- `apps/web/components/task/new-subtask-form-state.ts`
- `apps/web/components/task/use-subtask-submit.ts`
- `apps/web/lib/api/`
- `apps/web/lib/types/`
- `apps/web/src/locales/`
- `apps/web/e2e/tests/task/`
- `docs/public/`

## Dependencies

Complete Task 01 first.

## Risks

Preserve existing source identity, legacy callers, and user-owned folder data.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/mixed-repository-selection.md), listed IDs.
- [Design](../../specs/tasks/system-design/mixed-repository-selection.md), creation extension.
- Existing mixed selection and attachment tests; plan Tests/E2E tables map scenarios.

## Results

Implemented the unified Add menu, ordered folder/repository/set selection, shared
desktop and mobile picker surfaces, executor-aware hints, explicit empty payloads,
and localized public task-creation guidance.

Verification passed on 2026-09-14:

- The exact focused Vitest command in this work order passed 6 files and 68 tests.
- Web typecheck, lint, i18n checks, and `make build-web` passed.
- `make build-backend` and the refreshed E2E plugin package passed.
- Chromium E2E passed 2/2; mobile-chrome E2E passed 1/1.
- Public-doc validation and `git diff --check` passed.
