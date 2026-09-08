---
id: "01-correct-repository-grouping"
title: "Correct repository projection and grouping"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-REPOSITORY-GROUPING-001
acceptance_criteria:
  - AC-UI-SIDEBAR-REPOSITORY-GROUPING-001.1
  - AC-UI-SIDEBAR-REPOSITORY-GROUPING-001.2
  - AC-UI-SIDEBAR-REPOSITORY-GROUPING-001.3
  - AC-UI-SIDEBAR-REPOSITORY-GROUPING-001.4
  - AC-UI-SIDEBAR-REPOSITORY-GROUPING-001.5
  - AC-UI-SIDEBAR-REPOSITORY-GROUPING-001.7
system_design:
  - ../../specs/ui/system-design/sidebar-repository-grouping.md
---

# Task 01: Correct Repository Projection and Grouping

## Summary

Project every task repository into desktop and mobile sidebar items. Use the complete ordered slug list for repository group identity and labels.

## In scope

- Add a shared ordered repository-slug helper.
- Update both production sidebar item projectors.
- Add stable named combination keys and labels.
- Keep the generic multi-repository fallback for incomplete metadata.
- Add focused unit tests before production changes.
- Start with the red test `groups a projected multi-repository task by every repository slug`.

## Out of scope

- Playwright coverage.
- Repository filter membership changes.
- Saved-view persistence changes.
- Group-header component changes.

## Acceptance

- A projected two-repository task groups under both canonical slugs in attachment order.
- Different ordered combinations use different keys, and equal combinations share one key.
- Incomplete repository metadata keeps the task in a nonempty generic multi-repository group.

## Verification

```bash
cd apps && pnpm --filter @kandev/web exec vitest run lib/sidebar/sidebar-task-repositories.test.ts components/task/task-session-sidebar-item.test.ts components/task/mobile/session-task-switcher-sheet-item.test.ts lib/sidebar/apply-view.test.ts lib/sidebar/apply-view-labels.test.ts
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
```

## Files likely touched

- `apps/web/lib/sidebar/sidebar-task-repositories.ts`
- `apps/web/lib/sidebar/sidebar-task-repositories.test.ts`
- `apps/web/components/task/task-switcher-types.ts`
- `apps/web/components/task/task-session-sidebar-item.ts`
- `apps/web/components/task/task-session-sidebar-item.test.ts`
- `apps/web/components/task/mobile/session-task-switcher-sheet-item.ts`
- `apps/web/components/task/mobile/session-task-switcher-sheet-item.test.ts`
- `apps/web/lib/sidebar/apply-view.ts`
- `apps/web/lib/sidebar/apply-view.test.ts`
- `apps/web/lib/sidebar/apply-view-labels.test.ts`

## Dependencies

None.

## Risks

- Do not use the display label as the internal key.
- Do not remove the generic label while incomplete metadata can occur.
- Keep single-repository keys unchanged.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-SIDEBAR-REPOSITORY-GROUPING-001`
- `docs/specs/ui/system-design/sidebar-repository-grouping.md`
- Existing `repositorySlug` and sidebar projection patterns.

## Results

RED: the initial projection/grouping regression run failed with five assertions. Both sidebar projectors returned no `repositories`, complete combinations were classified as the generic `__multi__` group, equal and reversed combinations were not separated, and incomplete metadata could fall into `__unassigned__`.

GREEN: added the shared ordered slug resolver, projected `repositories` and retained `repositoryLinks` in desktop and mobile items, and added collision-safe ordered-combination keys with a generic hydration fallback. The final focused run passed 6 files and 111 tests. TypeScript passed, and web lint passed with zero warnings. Repository filters continue to use the primary compatibility value, and complete combinations retain duplicate canonical slugs.

Verification:

```text
cd apps && pnpm --filter @kandev/web exec vitest run lib/sidebar/sidebar-task-repositories.test.ts components/task/task-session-sidebar-item.test.ts components/task/mobile/session-task-switcher-sheet-item.test.ts components/task/mobile/session-task-switcher-sheet-hooks.test.ts lib/sidebar/apply-view.test.ts lib/sidebar/apply-view-labels.test.ts  # 6 files, 111 tests passed
cd apps/web && pnpm run typecheck  # passed
cd apps/web && pnpm run lint  # passed with zero warnings
cd apps/web && pnpm run i18n:check  # passed
```
