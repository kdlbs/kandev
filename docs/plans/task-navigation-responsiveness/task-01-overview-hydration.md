---
id: "01-overview-hydration"
title: "Make overview hydration safe"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TASK-NAVIGATION-RESPONSIVENESS-001
acceptance_criteria:
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.1
system_design:
  - ../../specs/ui/system-design/task-navigation-responsiveness.md
---

# Task 01: Make Overview Hydration Safe

## Summary

Prevent the projection memoization guard from dereferencing an absent cache
entry when a workflow snapshot has not arrived. Keep loaded lanes usable.

## In scope

- Explicit cache-entry presence before equality comparisons.
- Public-hook tests with a real app store and mixed loaded/unloaded workflows.
- Snapshot arrival/removal and workflow-specific visibility regression cases.

## Out of scope

Board layout, filtering semantics, fetching policy, and broad memoization changes.

## Acceptance

1. The missing-cache/missing-snapshot regression fails with the observed TypeError
   before the guard changes, then renders without throwing.
2. Loaded tasks retain correct workflow filters as another snapshot arrives or
   is removed; identical inputs still reuse their projection.

## ASCII UI preview

UI-01: Existing overview during partial hydration. Full [preview](plan.md#ascii-ui-preview).

```text
Desktop overview                Phone overview
| Workflow A: available tasks |  | Workflow A: available tasks |
| Workflow B: loading         |  | Workflow B: loading         |
```

Keep each surface's existing responsive layout and controls; the visible
change is successful rendering rather than a route error (`.1`).

## Implementation sequence

Mark in progress. Write `renders a missing snapshot beside a loaded workflow`
against `useSwimlaneRenderData`; record the expected failure. Require a cache
entry in the equality guard, then cover arrival, removal, and filter changes.
Run the commands below and record results. Task 04 supplies rendered coverage.

## Verification

From the repository root, after the plan's one-time dependency installation:

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/kanban/use-swimlane-render-data.test.tsx)
(cd apps/web && pnpm exec eslint hooks/domains/kanban/use-swimlane-render-data.ts hooks/domains/kanban/use-swimlane-render-data.test.tsx)
(cd apps/web && pnpm run typecheck)
git diff --check
```

## Files likely touched

- `apps/web/hooks/domains/kanban/use-swimlane-render-data.ts`
- `apps/web/hooks/domains/kanban/use-swimlane-render-data.test.tsx` (new)

## Dependencies

None.

## Risks

An overly broad equality shortcut could preserve stale filtered tasks. Use
the actual public hook and state transitions, not a copied predicate alone.

## Parallelism

`sequential`

## Inputs

- [Requirement `.1`](../../specs/ui/requirements/task-navigation-responsiveness.md)
- [Overview hydration design](../../specs/ui/system-design/task-navigation-responsiveness.md#overview-hydration)
- `components/state-provider.tsx` and existing real-store hook-test patterns
- The diagnostic report and served-bundle reproduction summarized in the plan

## Results

- RED: two public-hook regressions reproduced `Cannot read properties of
  undefined (reading 'hiddenStepIds')`; unchanged-input identity already passed.
- GREEN: all three real-store hook tests passed after requiring an existing
  cache entry before field comparisons.
- Targeted ESLint and frontend typecheck passed. Browser integration remains
  assigned to Task 04.
