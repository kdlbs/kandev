---
id: "05-prove-task-row-presentation"
title: "Prove task-row presentation"
status: complete
wave: 3
depends_on:
  - "02-render-configured-task-rows"
  - "03-build-responsive-task-row-settings"
  - "04-align-change-request-summary-rows"
plan: "plan.md"
spec: "../../specs/ui/requirements/sidebar-task-row-presentation.md"
---

# Task 05: Prove Task-Row Presentation End to End

## Acceptance

- Desktop E2E proves live preview, compact-section behavior, save, save-as, discard, independent
  views, reload persistence, field order, each right-side choice, and aligned status summaries.
- Mobile E2E proves the native drawer, touch controls, reorder, safe-area and scroll behavior,
  passive status indicator, row navigation, persistence, and no horizontal overflow.
- Focused backend and frontend checks, typecheck, lint, localization checks, and `git diff --check`
  pass, and every design artifact records the exact results.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/backend && go test -tags fts5 ./internal/user/... ./internal/backendapp
make -C apps/backend test
cd apps && pnpm --filter @kandev/web test -- --run lib/state/slices/ui/sidebar-view-wire.test.ts lib/state/slices/ui/ui-slice-migration.test.ts lib/state/slices/ui/sidebar-view-actions.test.ts components/task/task-item.test.tsx components/task/task-item-repository.test.tsx components/task/task-row-presentation.test.ts components/task/sidebar-filter/sidebar-filter-popover.test.tsx components/task/sidebar-filter/task-row-settings.test.tsx components/integrations/change-request-task-status-summary.test.tsx components/integrations/registered-change-request-task-icon.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check
cd apps/web && pnpm run i18n:ratchet
cd apps/web && pnpm e2e:run tests/task/sidebar-filter.spec.ts -- --grep "task row"
cd apps/web && pnpm e2e:run tests/pr/pr-status-badge.spec.ts -- --grep "readable task PR summary|task row"
cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-sidebar-views.spec.ts tests/task/mobile-task-status-summary.spec.ts -- --grep "task row|status summary"
git diff --check
```

## Files Likely Touched

- `apps/web/e2e/pages/sidebar-filter-popover.ts`
- `apps/web/e2e/tests/task/sidebar-filter.spec.ts`
- `apps/web/e2e/tests/task/mobile-sidebar-views.spec.ts`
- `apps/web/e2e/tests/task/mobile-task-status-summary.spec.ts`
- `apps/web/e2e/tests/pr/pr-status-badge.spec.ts`
- `docs/specs/ui/requirements/sidebar-task-row-presentation.md`
- `docs/specs/ui/requirements/pr-task-status-summary.md`
- `docs/plans/sidebar-task-row-presentation/plan.md`
- All task files in this plan

## Dependencies

Tasks 02, 03, and 04.

## Parallelism

`sequential`. This task validates the integrated persistence, editor, renderer, provider summary,
and mobile behavior after all production tasks finish.

## Inputs

- Every scenario in both linked specs.
- Production and unit changes from Tasks 01 through 04.
- Existing sidebar filter, PR status badge, mobile sidebar view, and mobile status-summary fixtures.
- The mobile parity reference for touch targets, scroll ownership, safe areas, and passive row icons.

## TDD Sequence

1. Extend the sidebar-filter page object with task-row section and field controls.
2. Add desktop E2E for collapsed state, live preview, save, save-as, discard, reload, and independent
   views. Record the expected failure before relying on the implementation.
3. Add PR E2E geometry assertions for shared status-value left edges and queue-detail alignment.
4. Add mobile E2E for the drawer, touch reorder, persistence, primary row tap, safe-area clearance,
   internal scroll, and overflow.
5. Run the focused backend, unit, typecheck, i18n, desktop E2E, and mobile E2E commands.
6. Inspect desktop and mobile results for title truncation, trailing-menu overlap, wrapped
   translations, and unexpected empty space.
7. Update both spec statuses to `shipped`, mark all plan tasks complete, and record exact results.

## Risks

- Drag-and-drop E2E can be input-device sensitive. Assert the persisted order after the gesture,
  not only the transient drag overlay.
- Geometry assertions need a small pixel tolerance for browser rounding while still detecting the
  current column misalignment.
- Seeded tasks need repository, pull request, diff, queue, and missing-data variants to cover all
  layout branches.
- Mobile screenshots or measurements can include browser safe-area differences. Assert against the
  drawer content and viewport, not a device-specific absolute offset.

## Output Contract

Report the E2E RED failures, final scenarios, desktop and mobile evidence, files changed, every exact
verification result, public-docs impact, blockers, risks, and synchronized spec, plan, and task
status.

## Results

Desktop task-row E2E passed with 1 test in 6.1 seconds, mobile task-row E2E passed with 1 test in
10.2 seconds, and PR task-summary E2E passed with 1 test in 3.2 seconds. The mobile status-summary
regression passed with 1 test in 4.7 seconds. Final capture runs also passed the desktop and mobile
task-row scenarios. The scenarios cover
collapsed/open state, live preview, ordering, visibility, trailing choices, save-as, overwrite,
discard, reload persistence, independent views, drawer bounds, safe-area/scroll ownership, and
summary alignment. Unit coverage, full backend suite, full workspace lint, typecheck, focused
ESLint, i18n checks, and `git diff --check` passed. Fresh PR evidence was captured for the desktop
settings, mobile drawer, and PR summary surfaces, inspected, compressed, and validated against its
three-entry manifest. The repository Traditional Chinese wrapper remains blocked by its pre-existing
`agents` namespace residual warning; the scoped task conversion passed with zero task residual
warnings.
