---
id: "01-working-tree-window"
title: "Bound working-tree rendering"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-BOUNDED-CHANGES-001
acceptance_criteria:
  - AC-UI-BOUNDED-CHANGES-001.1
  - AC-UI-BOUNDED-CHANGES-001.3
  - AC-UI-BOUNDED-CHANGES-001.4
  - AC-UI-BOUNDED-CHANGES-001.8
system_design:
  - ../../specs/ui/system-design/bounded-changes-rendering.md
---

# Task 01: Bound working-tree rendering

## Summary

Introduce the data-only timeline model and measured viewport. Integrate staged
and unstaged files in both layouts while preserving existing actions.

## In scope

- Replace eager working-file JSX arrays with descriptors and virtual rows.
- Flatten repository and directory headers into the same range.
- Keep expansion and selection above row lifetime, with scoped identities.
- Optimize directory construction without changing sorting or chain collapse.
- Establish the shared PanelBody ref and positive-measurement pattern.

## Out of scope

Historical/provider adapters belong to Task 02. Full lifecycle stress belongs
to Task 03. Do not change Git transport or the generic selection hook contract.

## Acceptance

1. A 50,000-file tree or list mounts at most 120 descriptors in the test viewport. First and last files remain reachable.
2. Complete counts, range selection, and bulk actions retain repository and layer identity. Offscreen entries remain eligible under existing rules.
3. Desktop and phone reuse existing controls. Wide/deep trees preserve ordering without recursive stack or argument-spread failure.

## ASCII UI preview

UI-01 and UI-02, from the [combined preview](plan.md#ascii-ui-preview):

```text
Desktop                        Phone
Changes [tools]                Changes [tools]
Unstaged (50000) [Stage all]    Unstaged (50000)
v cache/                       file-00000       ...
  file-00000 [actions]          cache/         +0 -0
  ... viewport window ...      ... viewport window ...
```

Each surface has one content scroller. Counts cover all files. Phone names
wrap and actions retain 44px targets. Covers criteria .1, .3, .4, and .8.

## Verification

First add `bounds 50000 working files` to the viewport component suite. Record
its failure before implementation. Use a controllable ResizeObserver and the
real virtualizer, not a mock that preselects a bounded range.

From the repository root:

```bash
cd apps/web
pnpm exec vitest run components/task/changes-timeline-model.test.ts components/task/changes-timeline-viewport.test.tsx components/task/changes-timeline-selection.test.ts components/task/changes-file-tree-model.test.ts components/task/changes-panel-tree.test.tsx components/task/changes-panel-file-row.test.tsx components/task/changes-panel-timeline-grouping.test.tsx
pnpm run typecheck
pnpm e2e:run --project chromium tests/git/git-changes-panel.spec.ts tests/changes-panel-multi-select.spec.ts tests/git/changes-panel-multi-repo-indent.spec.ts
pnpm e2e:run --project mobile-chrome tests/task/mobile-changes-panel.spec.ts
```

Install dependencies once from `apps/` if this worktree lacks them. The managed
E2E runner builds fresh assets. Do not overlap these runs.

## Files likely touched

All component paths use `apps/web/components/task/`:

- New `changes-timeline-model.ts`, `changes-timeline-viewport.tsx`, and their tests.
- New `changes-timeline-selection.ts` and its tests.
- `changes-panel-body.tsx`, `changes-panel-timeline.tsx`, `changes-panel-tree.tsx`.
- `changes-panel-repo-groups.tsx`, `changes-panel-file-row.tsx`.
- `changes-file-tree-model.ts` and its tests.
- `panel-primitives.tsx` only if existing forwardRef support is insufficient.

## Risks

A header containing an eager descendant list defeats virtualization. Selection
must not depend on mounted rows or collide across repositories.

## Dependencies

None.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/ui/requirements/bounded-changes-rendering.md).
- [System design](../../specs/ui/system-design/bounded-changes-rendering.md).
- [Plan](plan.md), including evidence, exclusions, and browser matrix.
- Existing Files virtualizer and Changes/commit tests named in this work order.

## Results

Completed.

RED evidence:

- The former eager tree mounted all 50,000 file rows, exceeding the 120-row budget.
- The former recursive directory sort raised `RangeError` for a path 12,000 segments deep.

GREEN results:

- `pnpm exec vitest run components/task/changes-timeline-model.test.ts components/task/changes-timeline-viewport.test.tsx components/task/changes-timeline-selection.test.ts components/task/changes-file-tree-model.test.ts components/task/changes-panel-tree.test.tsx components/task/changes-panel-file-row.test.tsx components/task/changes-panel-timeline-grouping.test.tsx`: 7 files and 49 tests passed.
- `pnpm run typecheck`: passed.
- `pnpm e2e:run --project chromium tests/git/git-changes-panel.spec.ts tests/changes-panel-multi-select.spec.ts tests/git/changes-panel-multi-repo-indent.spec.ts`: 31 tests passed.
- `pnpm e2e:run --project mobile-chrome tests/task/mobile-changes-panel.spec.ts`: 9 tests passed.
- Both managed E2E runs built the production Vite bundle, with pseudo-locale coverage enabled.

The deep-tree phone regression now scrolls the single Changes panel owner before
targeting an offscreen row. No known Task 01 limitation remains. Task 02 owns
historical and provider collections; Task 03 owns panel-wide stress and lifecycle.
