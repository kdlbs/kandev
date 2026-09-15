---
created: 2026-09-14
status: complete
requirements:
  - REQ-TASKS-MIXED-REPOSITORIES-001
  - REQ-TASKS-MIXED-REPOSITORIES-002
  - REQ-TASKS-MIXED-REPOSITORIES-003
  - REQ-TASKS-MIXED-REPOSITORIES-004
  - REQ-TASKS-MIXED-REPOSITORIES-005
system_design:
  - ../../specs/tasks/system-design/mixed-repository-selection.md
legacy_specs: []
---

# Implementation Plan: Workspace contents at task creation

## Overview

Extend the completed mixed-repository feature so folders and repositories coexist
from the first task launch. Deliver creation/runtime support, the shared Add UI,
then full last-used persistence and restoration. These are sequential work orders.
Implementation follows the approved package in this session.

The [requirements](../../specs/tasks/requirements/mixed-repository-selection.md)
and [design](../../specs/tasks/system-design/mixed-repository-selection.md) own the
behavior. Requirements 004 and 005 extend the same Tasks-owned creation contract.

## Scope

### In scope

- Ordered local folders plus local/remote repositories, including folders only.
- Contextual Add labels, three-choice menu, corresponding pickers, and bottom hint.
- Creation transport, durable source storage, first launch, retry, and readback.
- Backend-owned last-used full contents, branches, workflow, agent, and executor.
- Shared editable New Subtask controls, explicit empty versus inherited input.
- Desktop/mobile parity, source errors, localization, and public documentation.

### Out of scope

- Remote/container host-folder transfer or new repository-count capabilities.
- Redesigning live attachment, Quick Chat, or running-task source removal.
- Saving folders or unregistered remote URLs as repository-set members.
- New providers, connection storage, plugin publishing, or new executor types.

## Confirmed intent and grounded constraints

The user approved the menu, mixed contents, complete last-used defaults, scratch
on remove-all, exact contextual Add labels, and bottom-only scratch explanation.
No intent question remains. Existing backend folders are live host sources, not
uploads. Existing attachment storage supports ordered repository/folder batches.
Companion statuses were inspected: mixed selection and attachment are completed;
repository sets is completed and set base branches is complete. Their results
remain historical and are not reset by this package.
The create API and last-used model still need extension; UI-only work is inadequate.
Use the existing local-execution scope for folders, and preserve repository limits.
One folder retains direct CWD; multiple sources reuse the managed sibling root.
These choices reuse existing runtime semantics rather than adding folder copying.

## Technical approach

1. Add presence-aware optional `workspace_sources` to create input, normalize old
   callers, prepare all sources before publication, reuse `WorkspaceSourceBatch`
   storage, and carry folders through initial launch and recovery. Explicit `[]`
   must not be confused with omitted subtask inheritance. Do not call live attachment
   after create. Preserve rollback and user-owned folder boundaries.
2. Extend the canonical selection union with folders and migrate shared consumers.
   Compose Add menu -> repository/folder/set views in existing popover/drawer shells.
   Reuse source readiness, FolderPicker browsing/native selection, and set expansion.
   Move scratch explanation exclusively to the existing executor helper.
3. Extend backend last-used settings with a workspace-scoped contents snapshot.
   Save successful normalized input, including explicit empty, using targeted writes.
   Hydrate untouched drafts only, preserve per-row branches and source provenance,
   revalidate unavailable saved items, and retain existing profile/workflow rules.

No new folder table or SQL column is expected. Existing shared positions preserve
order. See the design for field presence, legacy compatibility, and failure rules.

## ASCII UI preview

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

The four views are structural acceptance references. All displayed copy goes
through locale catalogs. Example names and ASCII spacing are illustrative; menu
order, contextual label, hint placement, and phone flow are requirements. The
desktop and phone E2E specs cover the shipped creation paths.

## Tests

The test matrix maps acceptance criteria to the implemented tests and verification
commands. Map `AC-TASKS-MIXED-REPOSITORIES-` suffixes:

| AC | File and scenario |
| --- | --- |
| 004.1, 004.5, 004.6, 004.8 | `apps/backend/internal/task/service/service_workspace_creation_test.go`: ordered sources, folder-only, explicit empty, omitted inheritance, invalid path/collision, batch rollback |
| 004.1, 004.8 | Existing HTTP/WS task handler tests: omitted vs empty, legacy requests, mixed-field rejection, complete attachment response |
| 004.5, 004.6 | New lifecycle `manager_create_workspace_sources_test.go`: first-turn sources, multiple folders, resume, cleanup retains user files, incompatible executor |
| 004.1-004.4, 004.6 | Web `components/task-create-dialog-workspace-contents.test.tsx` plus the focused dialog suites: labels, mixed serialization, explicit empty, set append, folder count, bottom hint, and invalid-row behavior |
| 004.8 | Existing `components/task/use-subtask-submit.test.ts`: editable empty, mixed input, locked inheritance |
| 005.1-005.5 | Web `components/task-create-dialog-workspace-defaults.test.tsx`: full restore, deliberate empty, delayed response, failed create, invalid saved source, legacy fallback |
| 005.1, 005.2, 005.5 | User store/service and HTTP/WS handler tests: full snapshots, empty array retention, targeted writes, workspace isolation, unrelated-setting races |

## E2E tests

New files under `apps/web/e2e/tests/task/`, using existing seeded directories and
mock providers. Assert authoritative task readback and first-agent filesystem access,
not merely chip visibility. Tests must not depend on personal host directories.

| File / project | Covered scenarios |
| --- | --- |
| `create-task-workspace-contents.spec.ts` / chromium | Restores a folder/repository order, verifies the Repository / Local Folder / Repository Set menu order, appends a folder without removing the seeded repository, creates the mixed task, reads back shared positions, and verifies the empty Add label after remove-all. |
| `mobile-create-task-workspace-contents.spec.ts` / mobile-chrome | Uses one contents sheet, verifies the same menu order, appends a folder while retaining the seeded repository, checks page overflow, creates the mixed task, and reads back both sources. |
| Focused unit and backend suites | Cover explicit empty payloads, set append, source restoration, workspace-scoped last-used snapshots, provider readiness, legacy compatibility, and lifecycle/runtime behavior. |

Run desktop and phone sequentially with the guarded runner after rebuilding
artifacts.

## Work orders

- [x] [Task 01: Create and launch with workspace sources](task-01-create-workspace-sources.md)
- [x] [Task 02: Unified Add menu and contents UI](task-02-unified-add-ui.md)
- [x] [Task 03: Restore complete last-used contents](task-03-restore-workspace-defaults.md)

## Verification results

Implementation verification on 2026-09-14: catalog validation passed (267
decisions, 878 specifications); full specification lint, public-doc validation,
and whitespace checks passed. The web focused suites passed 68 tests for Task 02
and 14 tests for Task 03. Desktop E2E passed 2/2 and mobile E2E passed 1/1.
Backend task, lifecycle, orchestrator, user-settings, SQL guard, race conformance,
web typecheck, lint, i18n, and production build checks passed. Work-order results
contain the exact command groups. Before any pnpm command in a fresh worktree, run
`(cd apps && pnpm install --frozen-lockfile)` once.

## Companion packages

- [Mixed repository selection](../mixed-repository-selection/plan.md) is completed;
  retain provider-readiness fixes and historical checks. This package replaces its
  exclusive folder and repository-count mobile previews for future creation work.
- [Attach workspace sources](../attach-workspace-sources/plan.md) provides runtime
  storage/materialization; its live-attachment restrictions remain unchanged.
- [Repository sets](../repository-sets/plan.md) and
  [set base branches](../repository-set-base-branches/plan.md) retain ID-only storage.
- ADR 0028 and ADR 0041 retain backend preference ownership. This extends the payload,
  not its authority. No separate ADR is needed for that existing-boundary extension.

## Risks

- Repo-less early returns can omit folder-only sources during initial launch.
- `omitempty` and length checks can confuse explicit scratch with omitted inheritance.
- Settings patches can lose another workspace's snapshot without targeted merge logic.
- UI-only compatibility checks cannot enforce server-side folder/executor constraints.
- Source cleanup must remove owned links without deleting live user folders.
- Shared picker refactoring must preserve native desktop folder selection, provider
  readiness, branch defaults, sets, and the live attachment consumer.

## Executor-aware source follow-up

[New plan](../executor-aware-workspace-sources/plan.md) extends folder browsing, visible folder-only executor adjustment, and remote cloning from local origins. Existing completed results remain historical; new work is pending.
