---
id: "02-render-configured-task-rows"
title: "Render configured task rows"
status: complete
wave: 2
depends_on: ["01-persist-task-row-presentation"]
plan: "plan.md"
spec: "../../specs/ui/requirements/sidebar-task-row-presentation.md"
---

# Task 02: Render Configured Task Rows

## Acceptance

- Sidebar rows apply the effective view's detail visibility, order, details toggle, and right-side
  choice without changing Kanban cards or the rich task list.
- Relative time and repository suppression follow the spec, and missing values reserve no space.
- A moved provider status remains the single interactive indicator and coexists with the task menu.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web test -- --run components/task/task-item.test.tsx components/task/task-item-repository.test.tsx components/task/task-item.mr-guard.test.tsx components/task/task-row-presentation.test.ts
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint -- components/task/task-item.tsx components/task/task-item-stats-row.tsx components/task/task-item-leading-badges.tsx components/task/task-contribution-icons.tsx components/task/task-row-presentation.ts
```

## Files Likely Touched

- `apps/web/components/task/task-item.tsx`
- `apps/web/components/task/task-item-stats-row.tsx`
- `apps/web/components/task/task-item-leading-badges.tsx`
- `apps/web/components/task/task-contribution-icons.tsx`
- `apps/web/components/task/task-row-presentation.ts`
- `apps/web/components/task/task-switcher-row.tsx`
- `apps/web/components/task/task-switcher.tsx`
- Focused tests beside these files

## Dependencies

Task 01.

## Parallelism

`parallel-safe` with Task 03 after Task 01. This task owns task-row rendering and contribution-icon
placement. Task 03 owns the settings surface.

## Inputs

- The display rules in the sidebar task-row presentation spec.
- The normalized effective presentation from Task 01.
- Existing time selection, repository grouping suppression, contribution indicators,
  `TaskRowMetadata`, queue indicators, and task-menu behavior.

## TDD Sequence

1. Add pure resolver tests for ordered visible details, relative-time deduplication, repository
   grouping, and missing trailing data. Record the expected failures.
2. Add render tests for details enabled and disabled, queue and plugin metadata, each trailing
   choice, and one moved contribution indicator.
3. Add a keyboard test that opens the provider summary and the task menu as separate targets.
4. Implement presentation plumbing and the pure resolver.
5. Split configurable details from appended queue and debug indicators and gate the full metadata
   block with `details_enabled`.
6. Generalize the trailing slot and move, rather than duplicate, the change-request indicator.
7. Run focused tests, typecheck, and changed-file lint. Record exact results.

## Risks

- Moving the icon can mount two tooltip state owners or remove the focused node. Render one location
  from one resolved placement.
- Hiding metadata can leave vertical gaps if title-container spacing remains unconditional.
- The row menu can cover an interactive provider indicator if it reuses the passive-value overlay.
- Subtask toggles also occupy the trailing area and must remain reachable.

## Output Contract

Report RED failures, rendering and focus changes, files changed, exact unit, typecheck, and lint
results, mobile-impact notes, blockers, risks, and synchronized task and plan status.

## Results

RED rendering tests covered ordered details, grouping suppression, details hiding, trailing choices,
provider-status placement, missing values, and menu coexistence. The effective presentation is now
threaded through sidebar rows, with plugin and queue/debug metadata gated by the details master
toggle and one provider indicator rendered in its selected location. Task-row coverage passed with
78 tests across 5 files; the final focused cleanup set passed with 60 tests across 5 files; typecheck
and changed-file ESLint passed.
