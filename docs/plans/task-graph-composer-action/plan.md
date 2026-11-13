---
created: 2026-11-10
status: implemented
requirements:
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-001
system_design:
  - ../../specs/tasks/system-design/task-relationship-graph.md
legacy_specs: []
---

# Implementation Plan: Task graph composer action

## Overview

Replace the Task graph pill above the composer with a compact icon in the
right-aligned actions. Keep graph availability and navigation intact.
One sequential work order covers the control, placement, tests, and public guide.

Tasks owns this change because it owns task relationships and graph access.
The existing [requirements](../../specs/tasks/requirements/task-relationship-graph.md)
and [design](../../specs/tasks/system-design/task-relationship-graph.md) remain
the authoritative pair.

## Scope

### In scope

- Icon presentation after Share, near token stats, in the right action group.
- The same availability for parents, children, dependency links, and related tasks.
- Graph access before a task has a session.
- Localized accessibility, a tooltip, and keyboard activation.
- Phone wrapping and 44-pixel touch targets, including narrow fine-pointer windows.
- The shared icon in the existing passthrough action area.
- A location update in the public task guide during implementation.

### Out of scope

- Changes to graph data, rendering, modes, availability, or graph navigation.
- Changes to dependency chips, task context menus, or the board Graph view.
- New APIs, settings, feature flags, dependencies, or telemetry.
- Adding token stats or Share to the passthrough toolbar.

## Technical approach

1. Change `TaskGraphButton` to the shared Button and Tooltip primitives.
   Reuse `IconHierarchy3`, `task:taskGraphTitle`, the existing test ID, and
   `useOpenTaskGraph().openTab`. Preserve `useTaskGraphAvailable` as its only
   availability owner.
2. Move the button from `ChatStatusBar`'s status children into
   `ChatStatusBarRightControls`, after `TranscriptNavGroup`. Include task
   presence in `getRightControlVisibility` so the shell mounts without a session.
   Keep the empty-shell CSS and existing session gates for Usage and Share.
3. Use a 24-pixel compact desktop square. Apply 44-pixel minimum width and
   height at phone widths and for coarse pointers. The adjacent
   `AutoScrollToggleButton` is the sizing exemplar. Keep the existing phone
   order: banners, workflow action, status items, then right-aligned utilities.
4. Keep the existing graph sheet as the phone destination. Its depth calls for
   a full-height surface under the mobile UI language. Reuse its navigation,
   canvas scroll owner, safe-area behavior, and focus return.
5. Update the graph entry paragraph in `docs/public/tasks-and-workflows.md`
   after implementation. The section is a how-to entry within the task guide.
   Explain the icon location and its relationship-based visibility.

The completed [graph package](../task-relationship-graph/plan.md) and
[relationship package](../task-create-relationships/plan.md) are historical
delivery records. This package supersedes only their composer entry placement.
Their completed work-order statuses and recorded test results remain intact.

## ASCII UI preview

The drawings cover the affected row only. `[G]` means the hierarchy icon,
not a visible letter. Labels for tokens and Share identify existing icons.
Control order and grouping are requirements. Spacing is illustrative.
The drawings are design proposals, not rendered verification.

### UI-01: Desktop chat status row

Entry: task detail with relationships. State: token stats and Share available.

```text
Before:
[todos] [dependencies] [Task graph] [CI]     [nav] [tokens] [share] [Done]
After:
[todos] [dependencies] [CI]                 [nav] [tokens] [share] [G] [Done]
```

The source establishes the Before grouping. The status group stays left.
The graph joins the right utility group and uses the adjacent 24-pixel density.
The workflow action remains after utilities on desktop.

### UI-02: Phone chat status row

Entry: the same task on a phone. State: all optional controls visible and wrapped.

```text
                             [Done]
[todos] [dependencies] [CI]
       [nav] [tokens] [share] [G]
+---------------------------------+
| Composer                        |
+---------------------------------+
```

Progression keeps its existing row above status and utilities. The utility group
aligns right and can wrap within the available width. `[G]` has a 44 by 44-pixel
target. The composer and transcript retain their scroll and safe-area owners.
The graph opens directly into the existing full-screen sheet.

### UI-03: Qualifying task without session controls

```text
[dependencies, when present]                         [G]
```

The icon remains on the right before a session exists. An isolated task omits
the icon in all three views. Visibility and graph-loading errors keep their
existing behavior; this revision adds no new loading or error surface.

UI-01 through UI-03 cover `AC-TASKS-TASK-RELATIONSHIP-GRAPH-001.1`, `.2`,
`.3`, and `.5` through `.9`. The work order names their rendered checks.

## Tests

- `task-graph-button.test.tsx`: the accessible icon name, no visible pill label,
  tooltip, keyboard activation, and the correct task ID on activation (`001.7`).
- `chat-status-bar.test.tsx`: one graph control in the right group, including
  a null session with other utilities absent (`001.6`, `001.9`). Cover desktop
  to phone transitions without duplicate controls.
- `use-task-graph-available.test.ts`: retain existing eligibility cases (`001.1`,
  `001.2`). Run the existing passthrough toolbar suite for the shared consumer.

## E2E tests

- `task-graph.spec.ts`, project `chromium`: extend the entry scenario with the
  right-group location, icon name, compact geometry, and opening the same tab.
  Use a started mock session for the visible token stats/Share neighbor case.
  Keep isolated-task and existing graph-navigation coverage (`001.1` to `.3`,
  `.6`, `.7`). Do not rely on `.first()` to hide duplicate entry controls.
- The same desktop project covers a narrow fine-pointer window at 390 and
  767 pixels, then 768 pixels. Measure the phone target below the breakpoint
  and the compact target above it (`001.8`).
- `mobile-task-graph.spec.ts`, project `mobile-chrome`: verify right-group
  containment, wrapping, a 44-pixel square target, no horizontal overflow,
  direct sheet access, and focus return (`001.3`, `.5` to `.8`). Retain the
  existing 700-pixel case and add a coarse-pointer tablet sizing check.
- Check control bounds against their action group and status row. Inspect
  desktop and phone screenshots against UI-01 and UI-02 during these runs.
- The no-session case uses the component test. The full task page can create a
  session automatically, so a route assertion alone does not prove `001.9`.

## Work orders

- [x] [Task 01: Move the graph icon into composer actions](task-01-compact-graph-action.md)

## Verification results

Design checks on 2026-11-10:

- `python3 scripts/list-docs.py validate`: passed, 446 decisions and 1,963 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- PR documentation preflight through `validateCoverage`: the current docs-only
  diff is exempt. The planned runtime scope passes with this work order.
- Work-order references: the requirement and all eight acceptance criteria
  exist. Its design declares the requirement and appears in the plan.
- `git diff --check -- docs/specs docs/plans/task-graph-composer-action`: passed.

Implementation on 2026-11-10:

- Focused Vitest: `components/task-graph/task-graph-button.test.tsx`,
  `components/task/chat/chat-status-bar.test.tsx`,
  `components/task/passthrough-toolbar.test.tsx`, and
  `hooks/domains/task/use-task-graph-available.test.ts`: passed, 60 tests.
- `pnpm run typecheck`: passed.
- Oxlint with `--deny-warnings` on the changed source, test, and E2E files:
  passed.
- `pnpm run i18n:check`: passed, including key parity across all locales.
- `pnpm e2e:run --project chromium tests/task/task-graph.spec.ts`: passed,
  7 tests. The new entry case measured 24 pixels at 768 and 44 pixels at 390
  and 767 with a fine pointer.
- `pnpm e2e:run --project mobile-chrome tests/task/mobile-task-graph.spec.ts`:
  passed, 3 tests, including right-group containment, wrapping, and the
  coarse-pointer tablet target.
- `node --test scripts/validate-public-docs.test.mjs`: passed, 63 tests.
  `node scripts/validate-public-docs.mjs`: validated 55 pages.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.test.py`,
  `python3 scripts/lint-spec-files.py --all`, and `git diff --check`: passed.

Production, test, and public-guide files match the plan. The requirement is
back to `active`, the design to `current`, and this plan to `implemented`.

## Risks

- A session-only right-control gate can hide graph access for a blocked task.
- A coarse-pointer-only size rule misses phones with a fine pointer.
- Wrapping can clip a square target unless the rendered group also contains it.
- Availability reads can duplicate if the row adds another availability hook.

## Delivery checkpoint

Delivered. The single work order records its results, and the paired
requirements and design are promoted to `active` and `current`.
