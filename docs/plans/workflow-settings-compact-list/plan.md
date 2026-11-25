---
created: 2026-11-19
status: implemented
requirements:
  - REQ-UI-WORKFLOW-SETTINGS-COMPACT-LIST-001
  - REQ-UI-WORKFLOW-SETTINGS-COMPACT-LIST-002
system_design:
  - ../../specs/ui/system-design/workflow-settings-compact-list.md
legacy_specs: []
---

# Implementation Plan: Workflow Settings Compact List

## Overview

This change is frontend-only. The Workflows settings list gains a Compact or
Detailed view control, which defaults to Compact and is stored per browser.
In the compact view each workflow is a one-line row that expands in place to
the existing editor card. Work order 01 delivers the working feature with unit
tests. Work order 02 adds desktop and phone E2E coverage and keeps the existing
workflow E2E suites valid under the new default.

## Scope

### In scope

- `useWorkflowListView` preference hook, `WorkflowListViewToggle`,
  `WorkflowCompactRow`, and the `compact` prop on `WorkflowCard` and
  `SortableWorkflowItem`.
- New `workflows:*` copy in all seven catalogs (en, pt-pt, zh-cn, zh-hk,
  zh-tw, ja, ko) plus the regenerated pseudo-locale.
- Desktop and mobile Playwright specs. The workflow settings page object
  seeds the Detailed view for legacy specs that edit card fields.

### Out of scope

- Backend, API, or data changes, and server-side preference storage.
- Persisting expanded rows, expand or collapse all, search, and filtering.
- Changes to save, export, duplicate, delete, or reorder behavior.

## Technical approach

Follow the [system design](../../specs/ui/system-design/workflow-settings-compact-list.md).
Expansion state lives in `WorkflowCard`, so card state and drafts never
unmount. The body mounts on first open and then stays mounted and hidden. Draft
workflows (temporary IDs) start expanded.

## ASCII UI preview

Structure (order, grouping, which data appears) is required. Spacing, icons,
and exact widths are illustrative. All copy is localized.

**UI-01: Workflows list, compact view (default), desktop at 1024 px and wider**

```text
Workflows                         [Workflow Sync] [Export All] [Import] [+ Add Workflow]
Create autonomous pipelines ...
+--------------------------------------------------------------------------------------+
| Syncing from org/repo  dist                                            [Sync now]    |
+--------------------------------------------------------------------------------------+
                                                       View: [=Compact=] [ Detailed ]
+--------------------------------------------------------------------------------------+
| > Development           Default developm...  o Backlog > o In Progress > o Re... 5 steps  Claude  ::|
+--------------------------------------------------------------------------------------+
| > Symphony · Discovery  [Synced]             o Backlog > o Discover > o File Issue 4 steps      ::|
+--------------------------------------------------------------------------------------+
| > Release  [Unsaved]    Ship tagged builds   o Draft > o Build > o Publish   3 steps            ::|
+--------------------------------------------------------------------------------------+
```

`>` is the expand chevron, `o` is the step color dot, and `::` is the existing
drag handle. Clicking anywhere on the row except the handle toggles it.
Criteria: `001.1`, `001.2`, `001.6`, `002.1`, `002.2`.

**UI-02: One row expanded in the compact view**

```text
+--------------------------------------------------------------------------------------+
| v Development           Default developm...  o Backlog > o In Progress > ... 5 steps Claude ::|
|--------------------------------------------------------------------------------------|
| Workflow details                                                                     |
| Workflow Name [Development                         ]  Agent Profile [Claude      v]  |
| Description   [Default development workflow                                     ]   |
| > Workflow Prompt  Optional; expand to add shared step-entry instructions           |
| Workflow Steps  [o Backlog] > [o In Progress] > [o Review] > [o Done] > [+]          |
|                                          [Export] [Duplicate] [Delete Workflow]      |
+--------------------------------------------------------------------------------------+
| > Symphony · Discovery  [Synced]             o Backlog > o Discover > ...  4 steps   ::|
+--------------------------------------------------------------------------------------+
```

The row header stays as the collapse trigger and the body is today's card.
Criteria: `001.3`, `001.4`, `001.5`.

**UI-03: Detailed view**

This is the current page unchanged, with every card expanded and no row
header, plus the view control above the list. Criterion: `002.3`.

**UI-04: Phone (below 768 px), compact view**

```text
+--------------------------------+
| Workflows                      |
| [Sync][Export][Import][+ Add]  |  (wraps, as today)
|         [=Compact=][Detailed]  |
| +----------------------------+ |
| | > Development      [::]    | |  line 1: identity, at least 44 px tap target
| |   o Backlog > o In Pro...  5 | |  line 2: steps, truncated
| +----------------------------+ |
| | > Symphony · D... [Synced] [::]|
| |   o Backlog > o Discover  4  | |
| +----------------------------+ |
+--------------------------------+
```

The row uses two lines. The description is hidden below 1024 px. The page has
no horizontal overflow, and the drag handle keeps its 44 px coarse size.
Criterion: `001.7`.

## Work orders

| Order | Work order | Depends on | Result |
| --- | --- | --- | --- |
| 01 | [Compact list UI](task-01-compact-list-ui.md) | none | Working feature with unit tests and i18n |
| 02 | [Compact list E2E](task-02-compact-list-e2e.md) | 01 | Desktop and phone E2E; legacy workflow specs stay green |

The work is sequential, with no parallel waves.

## Verification strategy

- Unit (Vitest): preference hook defaults, persistence, and storage failure;
  compact row content, markers, and accessible name; card expand and collapse
  keeps drafts; draft workflows start expanded; Detailed view renders no row
  header.
- E2E (Playwright): a desktop and a `mobile-chrome` spec cover the default
  compact view, expand and edit, collapse preserving the unsaved draft, and the
  view persisting across reload. Existing workflow settings specs must still
  pass.
- Gates: `make fmt`, then `make typecheck test lint`, `pnpm run i18n:check`,
  `pnpm run i18n:ratchet`, `python3 scripts/list-docs.py validate`, and
  `python3 scripts/lint-spec-files.py --all`.

## Risks

- **Default change breaks legacy E2E.** About 24 specs use
  `WorkflowSettingsPage`, and these also navigate directly:
  `improve-kandev.spec.ts`, `i18n/pseudo-coverage.spec.ts`,
  `settings/mobile-workspace-clone.spec.ts`,
  `workflow/queue-limit-navigation.spec.ts`,
  `workflow/mobile-queue-limit-navigation.spec.ts`, and
  `helpers/workspace-clone.ts`. Mitigation: page-object `goto` seeds
  `detailed` only when no value is stored. Specs that navigate directly either
  use the same helper or expand the row. Run the whole `tests/workflow`
  directory plus the listed files.
- **Hidden-but-mounted bodies.** Collapsed bodies stay in the DOM, so global
  locators such as `[data-testid$="-profile-session-lifecycle-select"]` may
  match hidden elements. Scope locators to the card and assert visibility.
- **Settings composition rule.** Sections stay expanded. This list of resource
  records follows the collapsible agent blocks precedent and is called out in
  the requirements.

## Results

Both work orders are done. During implementation, one criterion was added: a deep-linked
workflow card expands (`AC-UI-WORKFLOW-SETTINGS-COMPACT-LIST-001.8`), because the WIP-queue settings
link targets `#workflow-card-<id>`. Public docs (`workflow-tips.md` and `workflow-import-export.md`)
describe the Compact and Detailed views. Verification evidence is in each work order.
