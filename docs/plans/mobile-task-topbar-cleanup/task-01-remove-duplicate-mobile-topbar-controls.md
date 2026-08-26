---
id: "01-remove-duplicate-mobile-topbar-controls"
title: "Remove duplicate mobile topbar controls"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-MOBILE-TASK-CHROME-001
acceptance_criteria:
  - AC-UI-MOBILE-TASK-CHROME-001.1
  - AC-UI-MOBILE-TASK-CHROME-001.2
  - AC-UI-MOBILE-TASK-CHROME-001.3
  - AC-UI-MOBILE-TASK-CHROME-001.4
  - AC-UI-MOBILE-TASK-CHROME-001.5
  - AC-UI-MOBILE-TASK-CHROME-001.6
system_design:
  - ../../specs/ui/system-design/mobile-task-chrome.md
---

# Task 01: Remove Duplicate Mobile Topbar Controls

## Summary

Remove phone-only layout and Git action entry points, simplify their orphaned
code, and make the existing task drawer and Changes surface the verified paths
for task and Git actions. Preserve desktop/tablet behavior and the phone title's
branch/diff summary.

## In scope

- Phone top-bar composition and retained task-drawer hitbox.
- Desktop-only simplification of `LayoutPresetSelector`.
- Deletion of orphaned mobile Git menu/dialog/drawer modules.
- Focused component coverage and retargeted mobile Playwright flows.

## Out of scope

- New task action menus or changed task-move behavior.
- Changes-panel visual redesign or Git contract changes.
- Backend, persistence, API, permission, localization, desktop, or tablet work.

## Acceptance

- Phone task chrome renders neither layout-profile nor general Git overflow,
  while the retained 44-by-44 task drawer can still move the active task.
- Applicable Git, change-request, and remote-contribution actions complete from
  Changes without the deleted phone-only Git modules.
- Long-title phone geometry, desktop layout selection, and tablet/desktop task
  chrome retain their existing behavior.
- Changes recovery controls retain 44 CSS-pixel touch targets through the phone
  range below `md`, including widths between `sm` and `md`.

## Verification

```bash
(cd apps && pnpm --filter @kandev/web test -- --run components/task/mobile/session-mobile-top-bar-repository.test.tsx components/task/layout-preset-selector.test.tsx components/task/remote-contribution-header-actions.test.tsx components/vcs/vcs-dialog-fields.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-task-topbar-long-title.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-sidebar-task-actions.spec.ts -- --grep "moves a task to another step from the mobile task drawer")
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/git/mobile-local-base-operations.spec.ts tests/git/mobile-pr-checkout-drift.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/gitlab/mobile-gitlab-parity.spec.ts -- --grep "creates and auto-links an MR with GitLab terminology")
```

## Files likely touched

- `apps/web/components/task/mobile/session-mobile-top-bar.tsx`
- `apps/web/components/task/mobile/session-mobile-top-bar-repository.test.tsx`
- `apps/web/components/task/layout-preset-selector.tsx`
- `apps/web/components/task/layout-preset-selector.test.tsx`
- `apps/web/components/task/mobile/mobile-changes-panel.tsx`
- `apps/web/components/task/remote-contribution-header-actions.tsx`
- `apps/web/components/task/remote-contribution-resolution-dialog.tsx`
- `apps/web/components/task/mobile/mobile-git-actions-dropdown.tsx` (delete)
- `apps/web/components/task/mobile/mobile-git-push-submenu.tsx` (delete)
- `apps/web/components/task/mobile/session-mobile-top-bar-git-controls.tsx` (delete)
- `apps/web/components/task/mobile/session-mobile-top-bar-git-controls.test.tsx` (delete)
- `apps/web/components/task/mobile/session-mobile-top-bar-dialog-parts.tsx` (delete)
- `apps/web/components/task/mobile/mobile-contribution-resolution-drawer.tsx` (delete)
- `apps/web/components/task/mobile/mobile-contribution-resolution-drawer.test.tsx` (delete)
- `apps/web/components/vcs/vcs-dialog-fields.test.tsx`
- `apps/web/eslint.i18n.options.mjs`
- `apps/web/e2e/tests/task/mobile-task-topbar-long-title.spec.ts`
- `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts`
- `apps/web/e2e/tests/git/mobile-local-base-operations.spec.ts`
- `apps/web/e2e/tests/git/mobile-pr-checkout-drift.spec.ts`
- `apps/web/e2e/tests/gitlab/mobile-gitlab-parity.spec.ts`
- `apps/web/e2e/tests/layout/mobile-saved-layout-confirmation.spec.ts` (delete)

## Dependencies

None.

## Risks

- Some focused Git tests currently use the removed trigger as setup. Retarget
  them through Changes without weakening operation, provider, safety, or
  geometry assertions.
- Preserve `useMobileGitMetrics` or equivalent so removing actions does not
  remove visible branch/diff context.

## Parallelism

`sequential`

## Inputs

- `docs/specs/ui/requirements/mobile-task-chrome.md`
- `docs/specs/ui/system-design/mobile-task-chrome.md`
- Existing phone task drawer, Changes, VCS dialog, and mobile E2E patterns.

## Results

- RED: the phone top-bar Playwright test found the existing
  `layout-preset-trigger`; after removal it passed with both retired triggers
  absent, a 44-by-44 task-drawer target, retained title truncation, and no
  horizontal overflow.
- RED: the retargeted remote-contribution test could not find the shared Changes
  warning because the phone panel omitted the shared relation/resolution props.
  Wiring those props and touch-sizing the shared warning and confirmation made
  the recovery flow pass.
- Removed the phone-only Git menu, push submenu, dialogs, contribution drawer,
  and their orphaned tests. Simplified `LayoutPresetSelector` to its surviving
  desktop behavior and removed the obsolete phone saved-layout E2E.
- Focused component tests passed (4 files, 11 tests), and frontend TypeScript
  typecheck passed.
- Phone Playwright verification passed for top-bar composition (1 test), task
  movement through the drawer (1 test), local rebase plus remote recovery
  through Changes (2 tests), and GitLab MR creation through Changes (1 test).
- The task-movement command's first build-backed attempt hit fixture startup
  `ECONNREFUSED` before UI assertions; its immediate no-build retry passed.
- A Pixel 5 Changes capture was inspected for the simplified header, contextual
  recovery placement, and horizontal containment, then cleaned with
  `pnpm e2e:clean`.
- Review remediation replaced premature `sm` size resets with `md` resets and
  added a 700px touch-viewport regression for the recovery warning and both
  confirmation actions.
- CodeRabbit quiet-mode follow-up rounded device-scaled geometry assertions and
  made every verification command run from an independent subshell.
