---
id: "03-render-layer-specific-changes"
title: "Render layer-specific changes"
status: completed
wave: 3
depends_on:
  - "02-publish-mixed-change-facets"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
acceptance_criteria:
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.10
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.11
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.12
system_design:
  - ../../specs/platform/system-design/workspace-git-status.md
---

# Task 03: Render layer-specific changes

## Summary

Consume mixed facets as two presentation rows while retaining one raw file identity and one mutation
target. Carry the selected layer through desktop Dockview and the existing mobile diff drawer, then
turn the red browser regressions green.

## In scope

- Mirror and project the optional facet contract in frontend state.
- Derive staged/unstaged sections and action availability from projected facets.
- Keep overall counts, Review aggregation, and mutation dispatch unique by repository/path.
- Include facet data in status equality, editor synchronization, and focus fingerprints.
- Add layer to desktop and mobile diff targets and pinned identities.
- Add focused unit/component tests and satisfy both Playwright regressions.

## Out of scope

- New layout, copy, controls, or localization keys.
- Hunk staging.
- Changing the combined Review source.

## Acceptance

- One mixed raw entry produces accurate Staged and Unstaged rows without doubling the overall file
  count or Git mutation.
- Selecting either row renders only its facet and may coexist with the other layer as a pinned diff.
- Desktop and mobile targeted E2E tests pass using the same backend snapshot contract.

## Verification

```bash
cd apps
pnpm --filter @kandev/web test -- hooks/domains/session/git-change-facets.test.ts hooks/domains/session/use-session-git-grouping.test.ts hooks/domains/session/use-session-git-summary.test.ts lib/state/slices/session-runtime/set-git-status-return.test.ts components/task/changes-panel-focus.test.ts components/task/task-changes-panel.test.ts lib/state/dockview-panel-actions.test.ts components/task/mobile/mobile-changes-panel.test.tsx

cd apps/web
pnpm run typecheck
pnpm e2e:run tests/git/git-changes-panel.spec.ts -- --grep "same path in staged and unstaged sections"
pnpm e2e:run --project mobile-chrome tests/task/mobile-changes-panel.spec.ts -- --grep "same path in staged and unstaged sections"
pnpm run lint:e2e-sleeps
```

## Files likely touched

- `apps/web/lib/state/slices/session-runtime/types.ts`
- `apps/web/lib/types/backend.ts`
- `apps/web/hooks/domains/session/git-change-facets.ts`
- `apps/web/hooks/domains/session/git-change-facets.test.ts`
- `apps/web/hooks/domains/session/use-session-git.ts`
- `apps/web/hooks/domains/session/use-session-git-summary.ts`
- `apps/web/lib/state/slices/session-runtime/session-runtime-slice.ts`
- `apps/web/components/task/changes-panel-focus.ts`
- `apps/web/components/task/changes-diff-target.ts`
- `apps/web/components/task/task-changes-panel.tsx`
- `apps/web/components/task/dockview-panel-content.tsx`
- `apps/web/components/task/dockview-shared.tsx`
- `apps/web/lib/state/dockview-panel-actions.ts`
- `apps/web/lib/state/dockview-store.ts`
- `apps/web/components/task/mobile/mobile-changes-panel.tsx`
- `apps/web/components/task/mobile/mobile-diff-sheet.tsx`
- `apps/web/e2e/tests/git/git-changes-panel.spec.ts`
- `apps/web/e2e/tests/task/mobile-changes-panel.spec.ts`

## Dependencies

Task 02 additive wire contract and green backend regression.

## Risks

- Preview and pinned panel IDs must include the layer without breaking legacy file and commit targets.
- Selected-diff compatibility paths can reopen combined-diff behavior if they omit the layer.

## Parallelism

`sequential`

## Inputs

- Requirement acceptance criteria `.10`, `.11`, and `.12`.
- Mixed-change and mobile-parity sections of the workspace Git status design.
- Existing shared Changes body, Dockview preview machinery, and mobile diff drawer.

## Results

- Added shared facet projection and layer-qualified desktop/mobile diff targets while preserving one
  aggregate Review file and one repository/path mutation target.
- Included facet updates in state equality, editor synchronization, focus fingerprints, summary
  projection, and Dockview identity.
- Coarse-pointer tree rows now expose an always-visible 44px stage action; fine-pointer hover layout
  remains unchanged.
- Focused Vitest coverage, frontend typecheck, targeted ESLint, i18n ratchet, and E2E sleep ratchet
  passed. The repository-wide `lint:e2e-sleeps` command still reports unrelated baseline violations;
  both changed E2E files pass its dedicated ESLint configuration.
- Fresh desktop Chromium and mobile Chrome regressions passed, proving independent row counts and
  diffs plus the unstage transition that removes only the staged facet.
