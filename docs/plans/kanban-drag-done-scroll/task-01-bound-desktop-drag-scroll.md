---
id: "01-bound-desktop-drag-scroll"
title: "Bound desktop Kanban drag scrolling to real columns"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-ADAPTIVE-KANBAN-001
  - REQ-UI-ADAPTIVE-KANBAN-003
acceptance_criteria:
  - AC-UI-ADAPTIVE-KANBAN-001.3
  - AC-UI-ADAPTIVE-KANBAN-001.4
  - AC-UI-ADAPTIVE-KANBAN-001.9
  - AC-UI-ADAPTIVE-KANBAN-001.10
  - AC-UI-ADAPTIVE-KANBAN-003.7
  - AC-UI-ADAPTIVE-KANBAN-003.9
system_design:
  - ../../specs/ui/system-design/adaptive-kanban.md
---

# Task 01: Bound desktop drag scrolling

## Summary

Prevent the Done column from moving away during a held task drag.
Keep the anchor reserve, but limit automatic horizontal travel to actual column content.
Prove the correction with pointer-driven browser evidence before marking this work order done.

## Inputs

- [Plan, root cause, and scenario matrix](plan.md).
- [Adaptive Kanban requirements](../../specs/ui/requirements/adaptive-kanban.md), especially 001.4, 001.9, 001.10, and 003.9.
- [Automatic scrolling and anchor space](../../specs/ui/system-design/adaptive-kanban.md#automatic-scrolling-and-anchor-space).
- [Phone scrolling contract](../../specs/tasks/requirements/mobile-kanban-scroll.md).
- `apps/web/AGENTS.md`, `/tdd`, `/e2e`, `/mobile-parity`, and the requested UI/clear-English skills.
- Existing reserve, anchor, and real pointer examples in `auto-hide-empty-columns.spec.ts` and `swimlane-scroll-affordances.spec.ts`.

## In scope

- A local desktop horizontal drag-scroll hook with content bounds and complete lifecycle cleanup.
- The scoped dnd-kit auto-scroll exclusion and drag-time snap suppression.
- RED/GREEN pointer regression, focused unit/component coverage, and existing mobile parity checks.

## Out of scope

Reserve removal or resizing, changes to the source-anchor formula, global DOM overrides, dependency changes,
task APIs, move semantics, new copy/settings, tablet redesign, and publication.

## Acceptance

1. At 1440px and 1920px, a stationary pointer over the original Done center remains over Done throughout a 1200ms hold. Drop succeeds and survives reload. Column widths and document containment remain correct.
2. An overflowing workflow retains real edge travel in both directions, source anchoring, temporary eligible destinations, cancellation, and vertical drag auto-scroll. The blank reserve does not extend automatic travel.
3. Frames/listeners stop after each terminal gesture and unmount. Snap returns after drag/pan ends. Existing phone menu moves, native card scrolling, and responsive boundaries pass without preference changes.

## UI preview

See [all structural previews](plan.md#ui-preview). Native rich output in the task conversation replaces ASCII drawings as requested.

**UI-01: Desktop Home, task held over Done.**
Before: Done moves to the left and the pointer lies over blank space.
After: existing columns keep their widths and Done remains the drop target.
The sidebar/header retain their position. The lane owns horizontal travel, while each column owns vertical task scrolling.
Covers AC-UI-ADAPTIVE-KANBAN-001.4, 001.9, 001.10, and 003.9.

**UI-02: Desktop long workflow.**
Edge scrolling reveals real offscreen columns in both directions.
Temporary destinations and source anchoring remain available. Cancellation restores normal composition and snapping.
Covers AC-UI-ADAPTIVE-KANBAN-001.3, 001.4, and 003.9.

**UI-03: Phone Home.**
`MobileColumnTabs` stays above the focused task list.
Card-menu Move to selects a destination. Card swipes scroll without pickup.
Dynamic viewport sizing, safe areas, and the existing touch targets stay intact.
Covers the existing phone parity under AC-UI-ADAPTIVE-KANBAN-003.7.

## Implementation sequence

1. Mark this work order `in_progress`. Read the paired design and plan. Preserve existing assertions and fixture cleanup.
2. Add the real pointer regression in `auto-hide-empty-columns.spec.ts`. Seed four inert columns and a Backlog task. Use the original Done-center coordinates throughout the hold. Prove behavioral RED before production changes.
3. Add meaningful hook/component tests. Implement the supported `canScroll` exclusion, bounded desktop horizontal loop, and snap suppression. Keep dnd-kit sensors, collision, overlay, and vertical scroll behavior.
4. Run the exact GREEN checks below. Capture rendered desktop and phone evidence. Record results and limitations, then mark this task `done` and the plan `implemented` when all conditions pass.

The desktop controller must distinguish physical reserve extent from actual lane content.
Use native document pointer/touch events to read client coordinates. Do not derive pointer position from a dragged card's scroll-adjusted rectangle.
Only accepted drag state enables the controller. Keep one frame owner per active desktop lane.
Use live bounds, a bounded elapsed-time step, and cleanup on every exit path.
An anchor beyond the normal content maximum must remain stable until edge intent requests movement.

## Regression test details

Name the primary parameterized scenario `keeps Done under a held drag at the final column`.
Use `dwell(page, 1200, "negative-assertion", reason)` with a geometry/hit-test sampler armed before entering the target.
After movement reaches the unchanged destination coordinates, sample the column under the pointer throughout the hold.
Stop the sampler in `finally`. Assert no observation lost the destination and no observation exceeded the real-content bound.
Record the expected old failure: Done moves away, even though its DOM node remains mounted.
Finish through release, saved step, card placement, and reload persistence.
A short instantaneous assertion before auto-scroll starts is insufficient.

Add an overflowing six-column case with real edge scrolling, direction reversal, and a final persisted move.
Do not programmatically set scrollLeft to make that case pass.
Preserve the existing reserve-size check, all-column width comparisons, cancellation, and manual-hide rules.
Extend the auto-hide fixture to place temporary columns before the source and prove the source position remains anchored.
Check an overflowing task column can still auto-scroll vertically during drag.
Restore mutated user settings and delete disposable workflow/task state in `finally` or existing fixture cleanup.

Hook tests must cover fitting content, partial overflow, reverse travel, resize, changed step extent,
zero geometry, anchor positions beyond the normal bound, elapsed-time capping, and cleanup.
Component tests must prove the desktop exclusion leaves column, outer-container, and tablet candidates eligible.
Tests must assert outcomes and lifecycle, not merely mirror helper calculations.

## Verification

Run from the repository root for RED. This managed command builds the tested code:

```bash
pnpm --dir apps/web e2e:run --project chromium tests/kanban/auto-hide-empty-columns.spec.ts -- --grep "keeps Done under a held drag"
```

After implementation, run these commands sequentially. The first E2E GREEN command creates a fresh production build.
Use `--no-build` for the mobile run only when no source changes followed that build.

```bash
pnpm --dir apps/web exec vitest run hooks/domains/kanban/use-kanban-horizontal-drag-scroll.test.ts hooks/domains/kanban/use-kanban-drag-scroll-anchor.test.ts components/kanban/adaptive-desktop-kanban.test.tsx components/kanban/kanban-drag-surface.test.tsx components/kanban/swimlane-kanban-content.render-stability.test.tsx components/kanban/swimlane-kanban-content.keyboard-reorder.test.tsx
pnpm --dir apps/web run typecheck
pnpm --dir apps/web exec oxlint -c oxlint.config.mjs --deny-warnings hooks/domains/kanban/use-kanban-horizontal-drag-scroll.ts hooks/domains/kanban/use-kanban-horizontal-drag-scroll.test.ts components/kanban/adaptive-desktop-kanban.tsx components/kanban/adaptive-desktop-kanban.test.tsx components/kanban/kanban-drag-surface.tsx components/kanban/kanban-drag-surface.test.tsx components/kanban/swimlane-kanban-content.render-stability.test.tsx e2e/tests/kanban/auto-hide-empty-columns.spec.ts e2e/tests/kanban/swimlane-scroll-affordances.spec.ts
pnpm --dir apps/web run i18n:ratchet
pnpm --dir apps/web e2e:run --project chromium tests/kanban/auto-hide-empty-columns.spec.ts tests/kanban/swimlane-scroll-affordances.spec.ts
pnpm --dir apps/web e2e:run --no-build --project mobile-chrome tests/kanban/mobile-auto-hide-empty-columns.spec.ts tests/kanban/mobile-kanban-reorder.spec.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Retain every process handle and wait for completion. Do not overlap E2E suites or override worker limits.
On this Nix host the cached Chromium executable needs Nix shared-library paths.
The diagnostic run supplied `LD_LIBRARY_PATH` only to its browser process through the ignored CLI config.
Use the managed runner's supported Docker/host setup. If it cannot run, report the exact blocker.
Do not infer GREEN from the old release run or use a dev build as production-build evidence.

## Files likely touched

- `apps/web/components/kanban/adaptive-desktop-kanban.tsx` and its existing test.
- `apps/web/components/kanban/kanban-drag-surface.tsx` and its existing test.
- Proposed `apps/web/hooks/domains/kanban/use-kanban-horizontal-drag-scroll.ts` and `.test.ts`.
- `apps/web/e2e/tests/kanban/auto-hide-empty-columns.spec.ts`.
- `docs/plans/kanban-drag-done-scroll/plan.md` and this work order for results.

`use-kanban-drag-scroll-anchor.ts` is an input and regression boundary, not planned production edit scope.
The mobile suites remain verification inputs. The overflow-affordance suite retains its assertions and adds held-drag geometry diagnostics and a screenshot.
The render-stability fixture adds the new dnd-kit monitor export. The direct desktop fixture now renders within a real DndContext.

## Dependencies

None. The installed dnd-kit 6.3.1 exposes the required supported `canScroll` predicate.
No package installation or API extension is required.

## Risks

A global `autoScroll={false}` would disable necessary vertical and overflow travel.
Removing snap alone delays the failure. Removing reserve space breaks the existing source-anchor design.
Duplicate event owners or stale frames can continue scrolling after drop.
Native desktop webviews and browsers outside the named projects remain unverified.

## Parallelism

`sequential`. No subagent work is authorized.

## Results

Implemented and verified on 2026-11-06. No subagents, commit, push, or PR were used.

### RED evidence

- The unchanged-source held-Done browser regression failed at both 1440px and 1920px. Both observations reported lost target and travel beyond real content.
- The snap test failed because dragging retained the normal snap behavior.
- The activation test failed with no movement before a second pointer event. Synchronous `useDndMonitor` capture corrected this timing boundary.
- A focused mouse-leave test failed with computed `scroll-snap-type: x mandatory`. The pan handler removed the inline override when the drag overlay appeared.
  Conditional drag-state snap classes corrected the conflict without changing the shared pan hook.

### GREEN evidence

All commands ran sequentially where they shared E2E artifacts. Backend/runtime fixtures used the managed runner.
The final source build used `pnpm --dir apps/web run build:e2e` after the last production change.
The test-only capture update did not require another source build.

| Command or check | Final result |
| --- | --- |
| The six-file Vitest command in Verification | 46 tests passed. Includes both real swimlane consumers. |
| Desktop managed command in Verification, using the fresh production build with `--host --no-build` | All 12 Chromium tests passed without retries. |
| Screenshot-enabled desktop capture command below | Three cases passed. |
| Mobile managed command in Verification with `--host --no-build` | All six mobile-chrome tests passed without retries. |
| `pnpm --dir apps/web run typecheck` | Passed on the final source and capture-enabled tests. |
| Scoped Oxlint command in Verification | Passed with warnings denied. |
| `pnpm --dir apps/web run i18n:ratchet` | Passed. No user-facing copy was added. |
| `python3 scripts/list-docs.py validate` | 436 decisions and 1928 specifications validated. |
| `python3 scripts/lint-spec-files.py --all` | Passed. |
| `git diff --check` | Passed. |

The screenshot-enabled capture command was:

```bash
pnpm --dir apps/web e2e:run --host --no-build --project chromium tests/kanban/auto-hide-empty-columns.spec.ts tests/kanban/swimlane-scroll-affordances.spec.ts --grep 'keeps Done under a held drag|keeps horizontal cues'
```

The E2E processes needed a local environment wrapper because this task inherited the live launcher's explicit config file.
The wrapper removed inherited `KANDEV_INTERNAL_*` values, set `KANDEV_SERVER_HOST=127.0.0.1`, and supplied Nix library paths.
The initial run's readiness timeout was environment setup failure, not behavioral RED. No shared runner or live instance configuration changed.

Rendered checks match UI-01 through UI-03. At both desktop widths, all real columns remain in the board and Done stays highlighted under the held card.
Long-board travel reaches actual content in both directions. Earlier temporary columns preserve the source anchor. Vertical dragging still scrolls the source column.
Phone card swipes scroll without pickup at 393px and 767px, menu moves work, and resizing through 768px preserves the intended affordances.
Desktop and phone PNG evidence is retained under the ignored `.playwright-cli/kanban-drag-impl/` directory.

Temporary tracing was removed before final verification. Managed E2E backends and browsers were stopped by their fixtures.
Native desktop webviews and browsers outside Chromium remain unverified. There are no remaining implementation blockers.

## PR review remediation

The auto-scroll exclusion now uses `data-kanban-bounded-drag-scroll`, independent of test IDs.
A new regression renames the test ID while retaining the marker. The vertical-drag browser case confirms that the column scrolls while its lane has no vertical overflow and remains at scrollTop zero.
Post-review local verification passed: 47 focused tests, all seven affected desktop browser cases on a fresh production build, typecheck, scoped lint, catalog validation, specification lint, and diff whitespace.
Historical counts above describe the original implementation. Remote CI and review completion remain pending for the remediation head. The marker does not change rendered geometry or the captured composition.

CodeRabbit follow-up: the registry-ordering fixture now stubs the monitor alongside its stubbed context. Viewport capture retains the current pointer/touch position before activation. The six-column browser scenario now enters the edge with one movement and holds, protecting the reported activation gap. Final local results passed: 50 focused tests across seven files, all 12 desktop/tablet browser cases on the fresh production build, typecheck, and scoped lint. The single-move browser case failed before the capture correction, and the registry fixture failed before its monitor stub was added.
