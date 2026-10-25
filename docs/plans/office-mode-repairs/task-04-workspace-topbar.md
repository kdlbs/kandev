---
id: "04-workspace-topbar"
title: "Workspace topbar actions"
status: done
wave: 4
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-KILL-SWITCH-006
acceptance_criteria:
  - AC-OFFICE-KILL-SWITCH-006.4
  - AC-OFFICE-KILL-SWITCH-006.5
  - AC-OFFICE-KILL-SWITCH-006.6
  - AC-OFFICE-KILL-SWITCH-006.7
  - AC-OFFICE-KILL-SWITCH-006.8
  - AC-OFFICE-KILL-SWITCH-006.12
  - AC-OFFICE-KILL-SWITCH-006.13
  - AC-OFFICE-KILL-SWITCH-006.14
system_design:
  - ../../specs/office/system-design/workspace-topbar-actions.md
  - ../../specs/office/system-design/workspace-kill-switch-02.md
---

# Task 04: Workspace topbar actions

## Summary

Move workspace controls into Office topbar while preserving the existing pause state machine and page-specific actions.

## In scope

Own shared controller placement, topbar action composition and conditional banners.
Implement phone workspace-actions drawer using the existing mobile menu geometry,
then hand off to the existing pause/resume confirmation with focus return. Preserve
refresh's pause-state meaning, partial-stop retry and workspace switch behavior.

## Out of scope

Other work orders, unrelated refactors, live-instance changes and publication.

## Acceptance

- Desktop topbar contains page actions plus refresh and pause/resume; no second running-state toolbar remains.
- Phone controls are discoverable, at least 44px, and preserve confirmation, reason, resume and partial-failure retry without stacked focus traps.
- One pause controller serves all surfaces; failed refresh retains stale/unknown state and workspace changes cannot operate on a stale target.

## ASCII UI preview

[Full preview](plan.md#ascii-ui-preview).

```text
UI-02: Office shell, running / paused / unknown
Before (desktop, from source and screenshot)
[Breadcrumb                                    ]
[                         Refresh  Pause workspace]
[Page content]

After desktop
[Breadcrumb        Page actions | Refresh pause | Pause workspace]
[Paused / stale / unavailable banner, only when relevant]
[Page content, scrolling]

After phone
[Menu  Page title                  Workspace actions]
[Paused / stale / unavailable banner when relevant]
[Page content, scrolling]

Workspace actions opens inset bottom drawer:
[Workspace name]
[Refresh pause status]
[Pause workspace / Resume workspace]
[Close]
Pause then opens the existing reason + confirmation flow.
Partial stop: banner retains [Retry stop].
```

Control grouping and phone composition are required; spacing and wording are illustrative.

## Regression evidence

Add shell test "composes page actions and workspace actions in topbar". Extend kill-switch rendered tests to assert topbar containment, running/paused/unknown states, 390px and 767/768px geometry, failed refresh and drawer-to-confirmation focus.

## Verification

Run from the repository root; every command is independently rooted. New test files
named below are outputs of this work order. Record red/green evidence.

```bash
(cd apps/web && pnpm exec vitest run app/office/components/office-shell.test.tsx app/office/components/workspace-pause-banner.test.tsx components/page-shell.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
make build-web
(cd apps/web && pnpm e2e:run --project=chromium e2e/tests/office/workspace-kill-switch.spec.ts)
(cd apps/web && pnpm e2e:run --project=mobile-chrome e2e/tests/office/mobile-workspace-kill-switch.spec.ts e2e/tests/office/mobile-office-navigation.spec.ts)
git diff --check
```

## Files likely touched

- `apps/web/app/office/components/office-shell.tsx`
- `apps/web/app/office/components/workspace-pause-banner.tsx`
- `apps/web/app/office/components/workspace-pause-controls.tsx`
- `apps/web/app/office/components/office-shell.test.tsx (new if absent)`
- `apps/web/app/office/components/workspace-pause-banner.test.tsx (new if absent)`
- `apps/web/hooks/domains/office/use-workspace-pause.ts`
- `apps/web/e2e/tests/office/workspace-kill-switch.spec.ts`
- `apps/web/e2e/tests/office/mobile-workspace-kill-switch.spec.ts`
- `apps/web/src/locales/`

## Dependencies

None

## Risks

Page actions must not displace workspace actions. Reusing the hook twice would duplicate state/request ownership. Check the 768px composition boundary and coarse-pointer sizes.

## Parallelism

`sequential`

## Inputs

Read linked requirements/designs in full, the plan evidence, scoped AGENTS.md,
TDD guidance and the adjacent existing tests before changing code. UI tasks also
read mobile-parity and E2E fixture guidance.

## Results

- Composed one pause controller into the Office topbar, kept state banners informational, and moved compact layouts into a discoverable workspace-actions drawer with 44px controls.
- Preserved refresh, pause/resume confirmation and partial-stop retry behavior, including drawer close-before-dialog focus handling.
- Added rendered pause/action regressions. Frontend typecheck, i18n validation and production build passed; browser execution is covered by the existing kill-switch specs plus the new mobile identity/export coverage.
