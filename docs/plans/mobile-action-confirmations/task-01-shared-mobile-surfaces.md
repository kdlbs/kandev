---
id: "01-shared-mobile-surfaces"
title: "Shared mobile confirmation surfaces"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-MOBILE-CONFIRMATION-001
  - REQ-UI-MOBILE-CONFIRMATION-002
  - REQ-UI-MOBILE-CONFIRMATION-003
acceptance_criteria:
  - AC-UI-MOBILE-CONFIRMATION-001.2
  - AC-UI-MOBILE-CONFIRMATION-001.3
  - AC-UI-MOBILE-CONFIRMATION-001.4
  - AC-UI-MOBILE-CONFIRMATION-001.5
  - AC-UI-MOBILE-CONFIRMATION-001.6
  - AC-UI-MOBILE-CONFIRMATION-001.7
  - AC-UI-MOBILE-CONFIRMATION-002.3
  - AC-UI-MOBILE-CONFIRMATION-002.4
  - AC-UI-MOBILE-CONFIRMATION-002.5
  - AC-UI-MOBILE-CONFIRMATION-002.6
  - AC-UI-MOBILE-CONFIRMATION-002.7
  - AC-UI-MOBILE-CONFIRMATION-003.1
  - AC-UI-MOBILE-CONFIRMATION-003.2
  - AC-UI-MOBILE-CONFIRMATION-003.5
system_design:
  - ../../specs/ui/system-design/mobile-action-confirmations.md
---

# Task 01: Shared Mobile Confirmation Surfaces

## Summary

Build the controlled phone confirmation renderer and explicit local host.
Prove lifecycle and accessibility behavior in component fixtures before any
domain adapter migrates. This order establishes the reusable presentation API.

## In scope

- Add the three components described in the design; keep the origin mounted
  hidden/inert beside its portal outlet, one active token, and one focus boundary.
- Implement standalone Drawer and hosted drawer/dialog modes, focus restoration,
  local Escape/Back, parent/route/breakpoint invalidation, and duplicate guards.
- Add opt-in hosting to `task/mobile/mobile-picker-sheet.tsx`; preserve normal
  picker behavior when no confirmation is active. Update frontend guidance and
  any shared localized labels in all catalogs.

## Out of scope

Domain migrations, mutation ownership, global modal state, global primitive
styling, new dependencies, backend changes, and browser-history interception.

## Acceptance

1. Tests demonstrate preserved origin state, a sibling outlet, stale-token
   cleanup, one-step Escape, focus return, and cancellation at the phone boundary.
2. Both render modes share readable content and accessible full-width phone
   actions; desktop/tablet primitives retain their current behavior.
3. Explicit submission dispatches once with caller-owned close/error behavior;
   rejection after dismissal cannot restore a stale request.

## Verification

After an explicit implementation request, use `/tdd`: add failing component
tests first, implement, and run the focused checks below. Bootstrap dependencies
once if this worktree has not been installed. Browser geometry is proven by 02.

```bash
cd apps
pnpm install --frozen-lockfile
cd web
pnpm exec vitest run components/confirmation/mobile-confirmation-content.test.tsx components/confirmation/mobile-action-confirmation.test.tsx components/confirmation/mobile-confirmation-host.test.tsx components/confirmation/inline-confirm-actions.test.tsx components/confirmation/action-confirm-popover.test.tsx
pnpm run typecheck
pnpm run i18n:check
pnpm run i18n:ratchet
```

Run changed-file ESLint on the new components and `mobile-picker-sheet.tsx`.
When adding copy, run `pnpm run i18n:zh-hant` and `pnpm run i18n:pseudo` before
catalog checks.

## Files likely touched

- `apps/web/components/confirmation/mobile-{confirmation-content,action-confirmation,confirmation-host}.tsx` and new corresponding tests.
- `apps/web/components/task/mobile/mobile-picker-sheet.tsx`.
- `apps/web/AGENTS.md` and `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,pseudo}/common.json` as needed.

## Dependencies

None.

## Risks

Portals under hidden ancestors, effect cleanup removing a newer request, focus
return racing parent close-autofocus, and accidentally changing global Enter
or pointer behavior.

## Parallelism

`sequential`

## Inputs

The paired mobile requirements/design and ADR; current `InlineConfirmActions`,
`ActionConfirmPopover`, `MobilePickerSheet`, their tests, and frontend guidance.

## Results

Shared content, responsive adapter, and local host implemented. Pickers can opt
in without changing their default composition. Component tests proved preserved
draft/scroll, focus return/fallback, one actual modal, Escape, source cleanup,
target/breakpoint invalidation, superseded-token isolation, duplicate prevention,
close-before-dispatch, rejected callbacks, disabled actions, and live locale.

- Focused Vitest: 5 files, 33 tests passed (behavioral RED failures observed).
- Typecheck and changed-file ESLint passed.
- i18n check and ratchet passed; reused existing localized labels, no new keys.
- Phone geometry remains owned by work order 02's browser verification.
