---
id: "02-archive-surfaces"
title: "Phone archive confirmation surfaces"
status: done
wave: 2
depends_on:
  - "01-shared-mobile-surfaces"
plan: "plan.md"
requirements:
  - REQ-UI-MOBILE-CONFIRMATION-001
  - REQ-UI-MOBILE-CONFIRMATION-002
  - REQ-UI-MOBILE-CONFIRMATION-003
acceptance_criteria:
  - AC-UI-MOBILE-CONFIRMATION-001.1
  - AC-UI-MOBILE-CONFIRMATION-001.2
  - AC-UI-MOBILE-CONFIRMATION-001.3
  - AC-UI-MOBILE-CONFIRMATION-001.4
  - AC-UI-MOBILE-CONFIRMATION-001.5
  - AC-UI-MOBILE-CONFIRMATION-001.6
  - AC-UI-MOBILE-CONFIRMATION-001.7
  - AC-UI-MOBILE-CONFIRMATION-002.1
  - AC-UI-MOBILE-CONFIRMATION-002.2
  - AC-UI-MOBILE-CONFIRMATION-002.3
  - AC-UI-MOBILE-CONFIRMATION-002.4
  - AC-UI-MOBILE-CONFIRMATION-002.5
  - AC-UI-MOBILE-CONFIRMATION-002.6
  - AC-UI-MOBILE-CONFIRMATION-002.7
  - AC-UI-MOBILE-CONFIRMATION-003.1
  - AC-UI-MOBILE-CONFIRMATION-003.2
  - AC-UI-MOBILE-CONFIRMATION-003.3
  - AC-UI-MOBILE-CONFIRMATION-003.4
  - AC-UI-MOBILE-CONFIRMATION-003.5
  - AC-UI-MOBILE-CONFIRMATION-003.6
system_design:
  - ../../specs/ui/system-design/mobile-action-confirmations.md
  - ../../specs/ui/system-design/confirmation-warning-hierarchy.md
  - ../../specs/tasks/system-design/archive-confirmation.md
---

# Task 02: Phone Archive Confirmation Surfaces

## Summary

Deliver the screenshot's primary flow: Archive becomes a dedicated step in the
Tasks drawer, with Cancel restoring the list. Use the same phone renderer from
Kanban, detail, bulk, and command-panel entry points.

## In scope

- Share archive state/content, route phones before pointer/legacy hints, and
  mount row confirmation beside the normal row. Preserve preference,
  classification/cascade, cleanup content, in-flight warning and navigation.
- Add Tasks and command-dialog hosts at stable ownership boundaries; retain
  close-on-confirm callbacks and restore list/query context on Cancel.
- Add geometry/lifecycle E2E and update existing mobile archive assertions;
  document the implemented behavior in the existing public how-to pages.

## Out of scope

Task deletion presentation, discard consent, archive API/cleanup/Undo, desktop
popover changes, and tablet composition changes.

## Acceptance

1. Tasks uses one active sheet with unchanged origin row geometry; Cancel/Back
   restores scroll/filter/focus. Page archive opens one compact bottom drawer.
2. Delayed/failed classification, bypass preference, active/non-active targets,
   cascade/bulk operations and navigation retain their existing domain outcomes.
3. Phone controls pass long-copy, contrast, safe-area, keyboard, touch, viewport
   and breakpoint checks; retained task-delete and wider layouts still work.

## Verification

Use `/tdd`, `/mobile-parity`, and `/e2e`. First add failing cases to existing
component tests and new `mobile-action-confirmations.spec.ts`. Keep existing
mobile outcome assertions while replacing inline/centered-archive selectors.

```bash
cd apps/web
pnpm exec vitest run components/task/task-archive-confirmation.test.tsx components/task/task-archive-confirm-dialog.test.tsx components/task/mobile/session-task-switcher-sheet.test.tsx components/task/task-switcher-context-menu.test.tsx hooks/use-task-archive-confirm.test.ts
pnpm e2e:run --project mobile-chrome tests/task/mobile-action-confirmations.spec.ts tests/kanban/mobile-card-archive-confirmation.spec.ts tests/task/mobile-archive-confirmation-preference.spec.ts tests/task/mobile-archive-task-redirect.spec.ts tests/task/mobile-confirmation-text-hierarchy.spec.ts
pnpm e2e:run --project chromium tests/task/confirmation-text-hierarchy.spec.ts tests/task/archive-confirmation-preference.spec.ts tests/task/archive-task-redirect.spec.ts tests/task/command-palette-archive.spec.ts tests/kanban/dialog-enter-confirms.spec.ts
pnpm run typecheck
pnpm run i18n:check
pnpm run i18n:ratchet
```

Capture and inspect hosted and standalone phone confirmations. Cover 320px,
767px, landscape, long localized content, and a touch-enabled 768px regression
case. Use finite-animation waits and `elementFromPoint` on footer actions.
Run `node --test scripts/validate-public-docs.test.mjs` and
`node scripts/validate-public-docs.mjs` from the repo root after public edits.

## Files likely touched

- Archive components and task-row/Tasks hosts listed in the manifest; `command-panel-confirmation.tsx` and its stable command-panel owner.
- Existing archive tests above; new `apps/web/e2e/tests/task/mobile-action-confirmations.spec.ts` and scoped geometry helper if shared.
- `apps/web/src/locales/` task catalogs if needed; `docs/public/mobile-remote-access.md` and `docs/public/tasks-and-workflows.md`.

## Dependencies

01 supplies the host and mobile renderer.

## Risks

The row-owned source can unmount itself, old `surfaceAction` wrappers can close
the drawer too early, and late classification can resurrect a cancelled target.
Screenshot cleanup wording must not override current domain content.

## Parallelism

`sequential`

## Inputs

Mobile design sections on archive/routing/lifetime; existing archive preference,
task cleanup and mobile navigation requirements; current archive and geometry tests.

## Results

Implemented the phone archive renderer, Tasks drawer host, menu handoff, and
command-dialog host. CommandDialog gained an optional content-props passthrough;
existing default dialog behavior is unchanged. Shared archive options were
extracted to keep the existing component within its complexity limit.

Validation:

- 77 focused component/hook tests passed before the additional disabled-parent
  regression; that regression and the shared action suite subsequently passed
  (13 tests).
- All 12 distinct mobile archive scenarios passed across the focused runs,
  including 320px Portuguese, 767px dark, landscape pseudo, rotation/breakpoint
  cancellation, footer hit-testing, preference, redirect, and retained full delete.
- All 11 desktop/tablet scenarios passed. The added 768px coarse-pointer case
  uses the existing context menu, not a phone-only menu trigger.
- Typecheck, changed-file ESLint, i18n check/ratchet, and public-doc validation
  passed. No new translation keys were required.
- Inspected hosted, compact dark, narrow Portuguese and landscape pseudo captures.
  Tasks preserves its height and scroll; standalone uses content height.

The sandbox prohibits local sockets, so isolated browser runs used approved
local-socket access. A test title exceeding the existing 60-character limit,
locale setup before the initial server boot, and a phone-only tablet selector
were corrected in the tests. No backend or policy changes were needed.
