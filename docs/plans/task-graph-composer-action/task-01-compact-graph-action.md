---
id: "01-compact-graph-action"
title: "Move the graph icon into composer actions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-001
acceptance_criteria:
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-001.1
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-001.2
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-001.3
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-001.5
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-001.6
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-001.7
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-001.8
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-001.9
system_design:
  - ../../specs/tasks/system-design/task-relationship-graph.md
---

# Task 01: Move the graph icon into composer actions

## Summary

Replace the graph pill with an icon after Share in the right composer actions.
Keep its eligibility and graph-opening behavior across desktop and phone layouts.

## In scope

- Own the shared `TaskGraphButton` presentation and its chat status placement.
- Keep the right-control shell available to a task without a session.
- Reuse the existing localized name, tooltip primitives, and graph opener.
- Preserve the shared passthrough entry, with the same compact icon.
- Add focused unit and rendered coverage from the plan's test matrix.
- Update the public task guide after the UI matches the plan.

## Out of scope

- Graph data, renderer, availability algorithm, or API changes.
- Other composer controls, the dependency chip, and task menu entry points.
- New settings, telemetry, dependencies, and translation keys.

## Acceptance

1. One localized graph icon occupies the right utility group after Share.
   A qualifying task keeps it without session controls; an isolated task hides it.
2. Desktop, phone, and coarse-pointer geometry match UI-01 through UI-03.
   The control opens the existing graph surface and preserves phone focus return.
3. The focused tests and public guide agree with the new entry presentation.
   Record every required command result before marking the work order done.

## ASCII UI preview

Excerpts from [the plan](plan.md#ascii-ui-preview), with the same view labels.
`[G]` represents an icon. It is not a visible text label.

```text
UI-01: Desktop
[status chips]                 [nav] [tokens] [share] [G] [Done]

UI-02: Phone, wrapped
                               [Done]
[status chips]
          [nav] [tokens] [share] [G]
[composer]

UI-03: Qualifying task without a session
[dependencies, when present]                           [G]
```

The graph stays in the right utility group. Phone targets measure at least
44 pixels in both dimensions. Desktop fine-pointer targets measure 24 pixels.
The existing phone workflow row stays above utilities. Exact spacing is illustrative.
These views map to `AC-TASKS-TASK-RELATIONSHIP-GRAPH-001.6` through `.9`.

## TDD sequence

1. Add failing button tests for the icon, localized name, tooltip, and keyboard activation.
2. Add failing status-row tests for right-group placement and the no-session case.
3. Implement the shared icon and row placement without another availability hook.
4. Extend the existing desktop and mobile graph E2E files with the plan's geometry cases.
5. Run the focused checks. Compare rendered desktop and phone views with the previews.
6. Update the public guide and record results. Promote the paired specs after all checks pass.

## Verification

Run from the repository root. If this worktree lacks dependencies, first run
`(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/web && pnpm exec vitest run components/task-graph/task-graph-button.test.tsx components/task/chat/chat-status-bar.test.tsx components/task/passthrough-toolbar.test.tsx hooks/domains/task/use-task-graph-available.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings components/task-graph/task-graph-button.tsx components/task/chat/chat-status-bar.tsx components/task-graph/task-graph-button.test.tsx components/task/chat/chat-status-bar.test.tsx e2e/tests/task/task-graph.spec.ts e2e/tests/task/mobile-task-graph.spec.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/task/task-graph.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-task-graph.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run desktop and mobile E2E commands sequentially through the managed runner.
Keep every existing graph scenario. Inspect screenshots at desktop, phone,
and the breakpoint cases. The unit test proves access without a session because
the task page can create a session automatically.

## Files likely touched

- `apps/web/components/task-graph/task-graph-button.tsx`
- `apps/web/components/task-graph/task-graph-button.test.tsx`
- `apps/web/components/task/chat/chat-status-bar.tsx`
- `apps/web/components/task/chat/chat-status-bar.test.tsx`
- `apps/web/e2e/tests/task/task-graph.spec.ts`
- `apps/web/e2e/tests/task/mobile-task-graph.spec.ts`
- `docs/public/tasks-and-workflows.md`
- The paired specs, this work order, and `plan.md` for lifecycle and results.

The passthrough toolbar and availability-hook suites are regression inputs.
Their source behavior does not need a change.

## Dependencies

None. The graph and related-task delivery packages are already implemented.

## Risks

- The existing graph-button mock returns null in status-row tests.
  Use a visible stub in placement cases so they can detect a missing mount.
- The parent status group can hide an empty child. Assert the visible control
  and its right-group containment in the browser.
- Tooltip disclosure must not consume a tap that opens the phone graph.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/task-relationship-graph.md#req-tasks-task-relationship-graph-001-open-the-task-graph).
- [Entry-point design](../../specs/tasks/system-design/task-relationship-graph.md#entry-points).
- [Renderer boundary ADR](../../decisions/2026-10-17-task-graph-renderer-boundary.md).
- `apps/web/AGENTS.md`, `/tdd`, `/e2e`, and `/mobile-parity`.
- Existing `AutoScrollToggleButton`, `TranscriptNavGroup`, and graph E2E tests.

## Results

Implemented on 2026-11-10.

- `TaskGraphButton` now renders a shared ghost Button with `IconHierarchy3`, the
  localized `task:taskGraphTitle` accessible name and tooltip, and the existing
  `data-testid`. Its 24-pixel desktop square expands to 44 pixels at phone
  widths and for coarse pointers. `useTaskGraphAvailable` remains the only
  availability owner, and `useOpenTaskGraph().openTab` remains the action.
- `ChatStatusBar` mounts the button in `ChatStatusBarRightControls` after
  `TranscriptNavGroup`, so it follows Share when Share is present. The left
  status children no longer mount a graph control. `getRightControlVisibility`
  admits the right-control shell whenever a task ID exists, which keeps the
  icon available before a session.
- Added button tests for the icon-only presentation, tooltip on focus, and
  activation. Added status-row tests for right-group placement after Share,
  the no-session case, hidden availability, and one instance across responsive
  reorder. Extended the desktop E2E with the right-group/Share neighbor and
  390-, 767-, and 768-pixel geometry cases. Extended the mobile E2E with
  containment, wrapping, overflow, sheet access, focus return, and a
  coarse-pointer tablet target.
- Updated the graph entry paragraph in `docs/public/tasks-and-workflows.md`.

Validation passed:

- `(cd apps/web && pnpm exec vitest run components/task-graph/task-graph-button.test.tsx components/task/chat/chat-status-bar.test.tsx components/task/passthrough-toolbar.test.tsx hooks/domains/task/use-task-graph-available.test.ts)`: 60 tests.
- `(cd apps/web && pnpm run typecheck)`, Oxlint with `--deny-warnings` on the
  changed files, and `(cd apps/web && pnpm run i18n:check)`.
- `(cd apps/web && pnpm e2e:run --project chromium tests/task/task-graph.spec.ts)`:
  7 passed.
- `(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-task-graph.spec.ts)`:
  3 passed.
- `node --test scripts/validate-public-docs.test.mjs` (63 passed),
  `node scripts/validate-public-docs.mjs` (55 pages),
  `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.test.py`,
  `python3 scripts/lint-spec-files.py --all`, and `git diff --check`.
