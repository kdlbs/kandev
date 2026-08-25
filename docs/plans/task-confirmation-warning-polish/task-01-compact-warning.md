---
id: "01-compact-warning"
title: "Compact warning and stable archive surface"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - ../../specs/ui/requirements/confirmation-warning-hierarchy.md
acceptance_criteria:
  - AC-TASKS-CONFIRMATION-WARNING-001.1
  - AC-TASKS-CONFIRMATION-WARNING-001.2
  - AC-TASKS-CONFIRMATION-WARNING-001.3
  - AC-TASKS-CONFIRMATION-SURFACE-002.1
  - AC-TASKS-CONFIRMATION-SURFACE-002.2
  - AC-TASKS-CONFIRMATION-SURFACE-002.3
system_design:
  - ../../specs/ui/system-design/confirmation-warning-hierarchy.md
---

# Task 01: Compact warning and stable archive surface

## Summary

Reduce shared still-working warning density in archive and delete confirmation
surfaces while preserving text, localization, alert semantics, in-flight
visibility, and action behavior. Widen only the fine-pointer archive popover
and remove its originating sidebar row's extra layout line while preserving
coarse-pointer inline expansion. Prove each contract with failing assertions
before production changes, then verify rendered desktop and phone states
against a rebuilt frontend.

## In scope

- Update the shared warning container and icon classes.
- Add an archive-only wider `ActionConfirmPopover` width contract, retaining
  the `w-64` default for unrelated consumers and constraining it to the
  viewport.
- Mount the fine-pointer archive confirmation outside `TaskItem`'s inline
  action slot at the context-menu adapter boundary; keep coarse-pointer
  confirmation row-owned.
- Add or update compactness assertions in archive and delete warning tests.
- Add a desktop sidebar RED/GREEN assertion for archive popover width,
  viewport containment, and exact source-row height before/open/cancel.
- Run focused unit, lint/typecheck/i18n, production-build, desktop, and phone
  rendered checks.
- Capture and inspect before/after PR screenshots and computed geometry evidence.

## Out of scope

- Warning copy or locale values.
- Global confirmation-popover width changes.
- Archive/delete state, callbacks, detection, dialog composition, action
  dimensions, focus, Escape, safe-area, or animation.

## Acceptance

- The warning uses compact body typography with intentional line height and
  pretty wrapping; spacing and icon scale are optically reduced in proportion.
- Archive and delete tests retain generating/background/idle behavior coverage,
  `role=alert`, localized text, and shared warning identity.
- The archive popover is wider than 256px only for the archive variant, stays
  inside the viewport at compact widths, and keeps title/body hierarchy.
- Fine-pointer sidebar row height is stable before opening, while open, and
  after cancel; coarse-pointer inline confirmation remains intentionally
  expanding.
- Desktop full-dialog and archive popover/inline paths remain contained; phone
  actions stay reachable at 44px or larger and document horizontal overflow is
  zero.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web test -- --run components/task/task-archive-confirm-dialog.test.tsx components/task/task-delete-confirm-dialog.test.tsx components/task/task-archive-confirmation.test.tsx)
(cd apps && pnpm --filter @kandev/web test -- --run components/confirmation/action-confirm-popover.test.tsx components/task/task-switcher-context-menu.test.tsx)
(cd apps && pnpm --filter @kandev/web exec eslint components/task/task-still-working-warning.tsx components/task/task-archive-confirm-dialog.test.tsx components/task/task-delete-confirm-dialog.test.tsx components/task/task-archive-confirmation.test.tsx)
(cd apps && pnpm --filter @kandev/web exec eslint components/confirmation/action-confirm-popover.tsx components/task/task-archive-confirmation.tsx components/task/task-switcher-context-menu.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:ratchet)
(make build-web)
(cd apps/web && pnpm e2e:run --project chromium tests/task/archive-confirmation-preference.spec.ts tests/kanban/card-menu-delete-archive.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-sidebar-task-actions.spec.ts)
```

The compactness regression must fail before the shared class change and pass
after it. The final rendered checks must use the production web build and
record screenshot paths plus computed hierarchy, viewport containment, action
reachability, and document overflow results.

## Files likely touched

- `apps/web/components/task/task-still-working-warning.tsx`
- `apps/web/components/confirmation/action-confirm-popover.tsx`
- `apps/web/components/task/task-archive-confirmation.tsx`
- `apps/web/components/task/task-switcher-context-menu.tsx`
- `apps/web/components/task/task-archive-confirm-dialog.test.tsx`
- `apps/web/components/task/task-delete-confirm-dialog.test.tsx`
- `apps/web/components/confirmation/action-confirm-popover.test.tsx`
- `apps/web/components/task/task-switcher-context-menu.test.tsx`
- `apps/web/e2e/tests/task/archive-confirmation-preference.spec.ts`
- `apps/web/e2e/tests/kanban/card-menu-delete-archive.spec.ts` if a focused
  desktop warning assertion is needed
- `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts` if the existing
  phone scenario needs a warning-state assertion
- `apps/web/.pr-assets/` only as uncommitted capture output

## Dependencies

None.

## Risks

- Shared change intentionally affects delete warning density too; check both
  dialog variants and all supported locale wrapping.
- Do not use stale Vite assets for E2E. Rebuild after every frontend edit.

## Parallelism

`sequential`

## Inputs

- `docs/specs/ui/requirements/confirmation-warning-hierarchy.md`
- `docs/specs/ui/system-design/confirmation-warning-hierarchy.md`
- `apps/web/components/task/task-still-working-warning.tsx`
- Existing archive/delete warning tests and phone exemplar in
  `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts`

## Results

- RED compactness/width assertions failed against current main before the
  production style and width changes. The old fine-pointer flow also grew the
  source row from 54.203125px to 62.203125px while open.
- GREEN focused unit coverage: 5 files, 59 tests. Affected ESLint, web
  typecheck, i18n ratchet, and `make build-web` passed.
- GREEN rendered coverage: desktop sidebar geometry and full dialog 2/2,
  phone idle/in-flight inline archive 2/2, and card archive/delete behavior
  5/5. Width, containment, computed hierarchy, row stability, touch targets,
  and zero-overflow assertions passed.
- Captured and inspected fresh compressed desktop and phone before/after media
  in `apps/web/.pr-assets/`; media is intentionally not committed to the PR
  branch.
