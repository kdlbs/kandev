---
created: 2026-08-28
status: done
requirements:
  - REQ-UI-QUICK-CHAT-ELEVATION-001
system_design:
  - ../../specs/ui/system-design/quick-chat-elevation.md
legacy_specs: []
---

# Implementation Plan: Strengthen Quick Chat backdrop

## Overview

Strengthen the backdrop of the shared Quick Chat and Quick Terminal dialog on
tablet and desktop widths. The work is one frontend vertical slice: apply the
surface-local darkening and blur, extend the existing desktop rendered test,
and rerun the existing mobile entry coverage to prove phone behavior remains
unchanged.

## Scope

### In scope

- Make the shared Quick Chat dialog backdrop more visible with darker
  semitransparency and light blur on tablet and desktop widths.
- Keep the existing panel shadow, dimensions, position, content, state,
  dismissal, and focus behavior unchanged.
- Preserve the existing full-screen phone composition and touch close path.

### Out of scope

- Changing the shared `Dialog` or `DialogOverlay` defaults.
- Changing the separate new-Quick-Chat picker dialog.
- Changing Quick Chat or Quick Terminal state, persistence, content, or tab
  behavior.
- Adding a new mobile composition or a user-facing setting.

## Technical approach

Update [`quick-chat-modal.tsx`](../../../apps/web/components/quick-chat/quick-chat-modal.tsx)
to retain `bg-black/20` on phones and use the stronger local overlay class
`sm:bg-black/40 sm:backdrop-blur-sm` from the `sm` breakpoint upward, while
retaining the existing `shadow-2xl` and responsive classes. Extend the existing
elevation test in
[`quick-chat.spec.ts`](../../../apps/web/e2e/tests/chat/quick-chat.spec.ts) to
assert the rendered backdrop filter in addition to its non-transparent
background and the panel shadow. Keep
[`mobile-quick-chat-entry.spec.ts`](../../../apps/web/e2e/tests/chat/mobile-quick-chat-entry.spec.ts)
as the mobile regression suite.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-UI-QUICK-CHAT-ELEVATION-001.1`, `.2`, `.5` | Desktop `Quick Chat` elevation scenario in `apps/web/e2e/tests/chat/quick-chat.spec.ts` |
| `AC-UI-QUICK-CHAT-ELEVATION-001.3`, `.6` | Same desktop scenario after Escape closes the dialog |
| `AC-UI-QUICK-CHAT-ELEVATION-001.4`, `.7` | Existing mobile entry and close scenarios in `apps/web/e2e/tests/chat/mobile-quick-chat-entry.spec.ts` |

## E2E tests

- Desktop: run the existing elevation scenario in `quick-chat.spec.ts` with
  the `chromium` project. It verifies the user-visible backdrop and close
  lifecycle through the real portal.
- Mobile: run `mobile-quick-chat-entry.spec.ts` with the `mobile-chrome`
  project. It verifies the existing phone-native full-screen flow, explicit
  close control, and no horizontal overflow.

## Work orders

- [x] [Task 01: Strengthen Quick Chat backdrop](task-01-quick-chat-backdrop-strength.md)

## Verification results

Task 01 completed.

- `cd apps && pnpm install --frozen-lockfile` - passed.
- `pnpm --filter @kandev/web run typecheck` - passed.
- `pnpm e2e:run --project chromium tests/chat/quick-chat.spec.ts -- --grep
  "stronger elevation backdrop"` - passed, 1 test.
- `pnpm e2e:run --project mobile-chrome
  tests/chat/mobile-quick-chat-entry.spec.ts` - passed, 4 tests.
- RED proof: the desktop test failed before the production class change because
  the overlay computed `backdrop-filter` to `none`.
- Review follow-up: phones retain `bg-black/20` with no filter, while the
  stronger `sm:bg-black/40 sm:backdrop-blur-sm` treatment starts at `sm`.
- Review follow-up RED proof: the mobile assertion failed against the unscoped
  treatment with `oklab(0 0 0 / 0.4)` and `blur(8px)`.
- Focused mobile style regression - passed, 1 test.

## Risks

- A backdrop that is too dark or too blurred can reduce page context. Keep the
  panel readable and use the existing restrained design-system treatment.
- The overlay must remain scoped to the shared Quick Chat surface so other
  dialogs and the separate picker do not change.
