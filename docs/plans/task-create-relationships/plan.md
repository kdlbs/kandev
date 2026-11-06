---
created: 2026-11-05
status: implemented
requirements:
  - REQ-TASKS-TASK-CREATE-RELATIONSHIPS-001
  - REQ-TASKS-TASK-CREATE-RELATIONSHIPS-002
  - REQ-TASKS-TASK-CREATE-RELATIONSHIPS-003
  - REQ-TASKS-TASK-CREATE-RELATIONSHIPS-004
  - REQ-TASKS-TASK-CREATE-RELATIONSHIPS-005
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-001
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-011
system_design:
  - ../../specs/tasks/system-design/task-create-relationships.md
  - ../../specs/tasks/system-design/task-relationship-graph.md
legacy_specs: []
---

# Implementation Plan: Task-create relationship establishment

## Overview

Let a creator attach a new task to a parent, a dependency, or a new
non-blocking "related to" link at creation time, everywhere a task is created
(main New Task dialog, New Subtask dialog, `create_task_kandev`), with smart
candidate suggestions so the picker rarely needs manual search. Every selector
is optional. The task-relationship graph then renders the new related-to edge
kind (REQ-TASKS-TASK-RELATIONSHIP-GRAPH-011), so a task linked only by
related-to is no longer a lone node.

The related-to link is stored in a new `task_relations` table per
[ADR-2026-11-05](../../decisions/2026-11-05-task-related-to-storage.md), kept
fully separate from the blocking `task_blockers` table.

The plan has six work orders across three waves. All use TDD.

## Scope

### In scope

- `task_relations` storage, repository, and `Service.CreateTask` integration
  (relation creation plus rollback compensation), for the main dialog, the
  subtask dialog, and `create_task_kandev`.
- `related_task_ids` on `create_task_kandev` and the create-dialog payload.
- A `kind` discriminator (`dependency` | `related`) on the task-graph edge
  response, model, legend, and renderer.
- New Parent and Related-to selectors in the main create dialog's advanced
  settings, and a Related-to selector in the New Subtask dialog.
- A shared candidate-ranking function reused by the Depends-on, Parent, and
  Related-to selectors.
- A third "Related" section in the dependency chip.
- Mobile parity for every new selector.

### Out of scope

- Editing or removing an established relationship from the task graph. The
  graph stays read-only.
- Changing `task_blockers` AND-blocking semantics, cycle checks, or auto-start.
- A forced or warned relationship step; every new selector stays skippable.
- Bulk relationship editing.

## Technical approach

1. **Backend storage and creation flow.** A new `RelationRepository` and
   `task_relations` table, wired into `Service.CreateTask`/`finalizeCreatedTask`/
   `rollbackPartialTask`, `BuildDependencyViews`, and `create_task_kandev`.
2. **Graph edge-kind plumbing.** The task-graph read, model, and board-graph
   builder gain the `kind` discriminator and include related-to edges under
   the existing display-inclusion rules.
3. **Create-dialog UI and ranking.** Parent and Related-to selectors in the
   advanced-settings option grid, and the shared ranking function.
4. **Visibility.** The dependency chip's third section and the graph's edge
   style, legend, and visibility gate.
5. **Subtask dialog and mobile parity.** The Related-to selector in the New
   Subtask dialog, and Playwright coverage for every new control.

## ASCII UI preview

### UI-01: Main create dialog, advanced settings expanded

Entry point: the existing collapsed "Advanced settings" disclosure
([`task-dependencies-create-dialog-advanced-settings.md`](../../specs/tasks/requirements/task-dependencies-create-dialog-advanced-settings.md)).
State: no caller-supplied parent, nothing selected yet.

```text
┌ Advanced settings ▾ ───────────────────────────────────────────┐
│ Depends on ⓘ          [ No dependency        ▾]                │
│ Parent                [ No parent            ▾]                │
│ Related to ⓘ          [ No related tasks     ▾]                │
└──────────────────────────────────────────────────────────────┘
```

Opening the Related-to selector (same contained command-list popover as
Depends-on today):

```text
┌ Related to ──────────────────────────────────────────────┐
│ 🔎 Search tasks…                                      ⓘ  │
├────────────────────────────────────────────────────────┤
│ Same column                                             │
│  ☐ ⬚ Write API docs                                     │
│  ☑ ⬚ Update onboarding checklist                        │
│ Recently viewed                                          │
│  ☐ ⬚ #42 · Billing retry logic                          │
│ All tasks                                                │
│  ☐ ⬚ Seed test fixtures                                 │
└────────────────────────────────────────────────────────┘
```

With a parent and a related task selected, the row triggers read:

```text
│ Parent                [ Ship billing         ▾]                │
│ Related to ⓘ          [ 1 related task       ▾]                │
```

### UI-02: New Subtask dialog, Related-to row

Entry point: the New Subtask dialog's existing option area. Parent remains
externally supplied and is not shown as a selector here.

```text
┌ New subtask ───────────────────────────────────────────────┐
│ ...title, prompt, repository, profile fields unchanged...  │
│ Related to ⓘ          [ No related tasks     ▾]             │
└──────────────────────────────────────────────────────────┘
```

### UI-03: Dependency chip, third section

Entry point: the existing dependency chip popover.

```text
┌ Waiting on ───────────────────┐
│ ✓ Scaffold schema             │
├ Blocks ───────────────────────┤
│   Wire UI                     │
├ Related ───────────────────────┤
│ ⬚ Update onboarding checklist │
├───────────────────────────────┤
│                 [View graph →]│
└───────────────────────────────┘
```

### UI-04: Task graph legend, related-to style

```text
│ ── Resolved   - - Pending   ━━ Failed (red)   ···· Related   ┆ External │
```

A related-to connector draws with no arrowhead, distinguishing it from a
directed dependency edge at a glance.

### UI-05: Phone layout

Entry point: the same full-screen create form and New Subtask sheet. The
Parent and Related-to rows stack to one column below Depends-on, following the
existing advanced-settings responsive grid; each selector keeps its
44-CSS-pixel touch target and contained popover.

```text
┌─────────────────────────────┐
│ Advanced settings ▾         │
│ Depends on         ▾        │
│ Parent             ▾        │
│ Related to         ▾        │
└─────────────────────────────┘
```

Preview mapping: UI-01 → `AC-...-CREATE-RELATIONSHIPS-001.1`–`.5`,
`002.1`–`.5`, `003.*`; UI-02 → `002.1`–`.5` (subtask dialog path); UI-03 →
`005.1`–`.2`; UI-04/UI-05 → `RELATIONSHIP-GRAPH-011.1`–`.2`.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `CREATE-RELATIONSHIPS-002.2`–`.4`, `004.1`–`.4` | Go repository, service, and MCP handler tests |
| `CREATE-RELATIONSHIPS-001.1`–`.5` | Parent selector component tests; desktop E2E |
| `CREATE-RELATIONSHIPS-002.1`, `002.5` | Related-to selector component tests |
| `CREATE-RELATIONSHIPS-003.1`–`.5` | `rankCreateDialogCandidates` unit tests |
| `CREATE-RELATIONSHIPS-005.1`–`.2` | Dependency chip third-section tests |
| `RELATIONSHIP-GRAPH-011.1`–`.4` | Go graph-read tests; `buildTaskGraphModel` tests; renderer/legend tests |

## E2E tests

- Extend `apps/web/e2e/tests/task/task-dependencies.spec.ts` (or a new
  `task-create-relationships.spec.ts`): select a parent and a related task
  from the main create dialog, submit, and assert the chip's three sections
  and the task graph's related-to connector and legend entry.
- Extend the mobile dependency spec for the Parent and Related-to rows'
  touch targets and contained pickers.

## Work orders

- [x] [Task 01: Add task_relations storage and wire it into task creation](task-01-relation-storage-and-creation.md)
- [x] [Task 02: Add the related-to edge kind to the task graph read](task-02-graph-edge-kind-backend.md)
- [x] [Task 03: Render related-to edges in the graph and board view](task-03-graph-edge-kind-frontend.md)
- [x] [Task 04: Add Parent and Related-to selectors with smart ranking](task-04-create-dialog-selectors.md)
- [x] [Task 05: Add the dependency chip's Related section](task-05-dependency-chip-related-section.md)
- [x] [Task 06: Add the subtask-dialog selector and mobile parity](task-06-subtask-dialog-and-mobile-parity.md)

## Verification

Run the commands listed in each work order. The final work order also runs:

```bash
make fmt
make typecheck test lint
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Risks

- Task creation is non-transactional today; adding a second edge-write loop
  (relations, after blockers) widens the window for a partial failure between
  task insert and edge writes. `rollbackPartialTask` must cover both loops.
- A pair of tasks could end up with both a `task_blockers` row and a
  `task_relations` row if a dependency is added later through the Edit
  dialog. This is an accepted consequence of
  [ADR-2026-11-05](../../decisions/2026-11-05-task-related-to-storage.md), not
  a bug to guard against in this plan.
- Reusing `compareDependencyCandidates` as a wrapped fallback must not change
  the existing Depends-on selector's observable order when no new ranking
  signal applies, since `task-dependencies-create-dialog-dependency-selector.md`
  is still the active contract for that selector.
- The task-graph live-refresh signature and the board-graph builder both need
  the new `related` IDs; missing either leaves a stale graph after a
  related-to link is added, without a loud failure.
