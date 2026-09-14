---
id: "03-picker-confirmations"
title: "Session and terminal picker confirmations"
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
  - AC-UI-MOBILE-CONFIRMATION-001.4
  - AC-UI-MOBILE-CONFIRMATION-001.5
  - AC-UI-MOBILE-CONFIRMATION-002.1
  - AC-UI-MOBILE-CONFIRMATION-002.3
  - AC-UI-MOBILE-CONFIRMATION-002.5
  - AC-UI-MOBILE-CONFIRMATION-003.1
  - AC-UI-MOBILE-CONFIRMATION-003.2
  - AC-UI-MOBILE-CONFIRMATION-003.5
  - AC-UI-MOBILE-CONFIRMATION-003.6
system_design:
  - ../../specs/ui/system-design/mobile-action-confirmations.md
---

# Task 03: Session and Terminal Picker Confirmations

## Summary

Use one focused confirmation step in the existing session or terminal picker.
Retain terminal immediate dismissal and session deletion's current feedback.

## In scope

- Adapt terminal close and session delete to the shared picker host, preserving
  normal rows, explicit target names, primary/only-session warnings and focus.
- Keep target selection stable through menu dismissal and live updates.
- Replace row-scoped mobile confirmation selectors and prove preserved sibling
  usability while terminal teardown is held pending.

## Out of scope

Terminal teardown, server reconciliation, session deletion/primary-selection
semantics, Quick Terminal, desktop tab-X dialogs, and generic progress feedback.

## Acceptance

1. Cancel/Back restores the same picker state and target focus, and parent
   dismissal clears the decision without a mutation or stale reopen.
2. Confirm invokes the captured action once; terminal UI disappears before
   teardown and its sibling remains usable, including the existing failure path.
3. Session delete retains its warnings, eligibility, selection fallback, and
   owner feedback while the confirmation uses the shared phone layout.

## Verification

Use `/tdd` and `/e2e`. Add the focused terminal adapter test file named below;
extend session component tests and the existing real mobile flows.

```bash
cd apps/web
pnpm exec vitest run components/task/terminal-close-inline-confirmation.test.tsx components/task/mobile/mobile-sessions-section.test.tsx components/task/close-terminal-confirm-popover.test.tsx
pnpm e2e:run --project mobile-chrome tests/terminal/mobile-terminal-close.spec.ts tests/session/mobile-session-deletion.spec.ts
pnpm e2e:run --project chromium tests/session/session-tab-close-guard.spec.ts
pnpm run typecheck
pnpm run i18n:check
pnpm run i18n:ratchet
```

Use the existing causal teardown pause helper; no new sleeps or timeout growth.
Keep the target row mounted during confirmation and assert the active host,
not a now-hidden inline group. Inspect a rendered picker confirmation.

## Files likely touched

- `apps/web/components/task/terminal-close-inline-confirmation.tsx` and new test.
- `apps/web/components/task/mobile/mobile-{terminals,sessions}-section.tsx`, existing session tests, and picker host wiring.
- The mobile specs above and task locale catalogs if target/title copy is needed.

## Dependencies

01 supplies the picker host and shared renderer.

## Risks

A shared pending spinner would violate terminal feedback. Session and terminal
IDs differ; the adapter must pass the same stable target as the existing owner.

## Parallelism

`sequential`

## Inputs

Mobile design on hosted content and domain failure behavior;
[terminal close requirements](../../specs/ui/requirements/terminal-close-feedback.md)
and [session delete requirements](../../specs/ui/requirements/session-tab-delete-feedback.md).

## Results

Implemented named session/terminal steps in the opted-in picker host. Normal
phone rows and triggers stay mounted; menu close transfers to the session step,
and Back/Cancel restore the initiating control. Session confirmation is a small
separate adapter to keep the existing section within the file-length limit.
The shared mobile adapter accepts an explicit owner `onClose` for consumers whose
pre-dispatch dismissal differs from cancellation. Parked-terminal desktop call
sites now supply terminal identity; their presentation remains unchanged.

Validation: 18 focused picker/popover tests passed, followed by 93 affected
shared/archive/picker tests after the final adapter and focus changes. Both
mobile E2E flows passed, including a sibling shell command while target teardown
was held. The desktop last-session tab-close regression passed. Typecheck,
changed-file ESLint and i18n check/ratchet passed. No new copy keys or teardown
policy changes were required.
