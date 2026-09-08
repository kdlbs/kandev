---
id: "01-keep-launcher-visible"
title: "Keep the Configuration Chat launcher visible"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-QUICK-TERMINAL-001
acceptance_criteria:
  - AC-UI-QUICK-TERMINAL-001.2
system_design:
  - ../../specs/ui/system-design/quick-terminal.md
---

# Task 01: Keep the Configuration Chat launcher visible

## Summary

Add a failing desktop and mobile regression for the Settings Configuration Chat launcher, then
remove the component's open-state visibility and interaction suppression. Keep the existing
controlled popover and responsive geometry as the only open/close mechanism.

## In scope

- Add desktop E2E assertions that the launcher stays visible and enabled with the popover open and
  closes the panel when activated again.
- Add the equivalent touch and viewport-containment assertions to the mobile Configuration Chat
  flow.
- Remove the launcher's open-state `aria-hidden`, tab-order removal, pointer-event suppression, and
  zero-opacity styling while preserving tooltip suppression during the open state.

## Out of scope

- Changing Configuration Chat content, session behavior, or the header close action.
- Repositioning or resizing the popover, floating-save host, or launcher.
- Adding new user-facing copy or translation keys.

## Acceptance

- The floating Configuration Chat launcher remains visible, enabled, keyboard-focusable, and
  touch-operable while its Settings popover is open.
- Activating the retained launcher closes the popover through the existing `onOpenChange` path.
- Focused desktop and mobile E2E tests pass without document horizontal overflow or launcher
  clipping.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm run typecheck
cd apps/web && pnpm e2e:run e2e/tests/settings/config-chat-popover.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/settings/mobile-configuration-chat.spec.ts
```

## Files likely touched

- `apps/web/components/config-chat/config-chat-panel.tsx`
- `apps/web/e2e/tests/settings/config-chat-popover.spec.ts`
- `apps/web/e2e/tests/settings/mobile-configuration-chat.spec.ts`

## Dependencies

None.

## Risks

- Radix trigger focus and tooltip behavior can overlap the open popover if tooltip suppression is
  removed accidentally.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-QUICK-TERMINAL-001` and `AC-UI-QUICK-TERMINAL-001.2`.
- `docs/specs/ui/system-design/quick-terminal.md` launcher and responsive behavior.
- Existing Configuration Chat desktop and mobile Playwright specs.

## Results

Implemented on 2026-08-26. The floating trigger no longer receives open-state `aria-hidden`,
tab-order removal, pointer-event suppression, or zero-opacity styling. Tooltip suppression remains
active while the popover is open. Desktop and mobile E2E coverage verifies visibility, enabled state,
accessibility attributes, viewport containment on mobile, and toggle-close behavior.

Verification passed:

- `cd apps && pnpm install --frozen-lockfile`
- `cd apps/web && pnpm run typecheck`
- `cd apps/web && pnpm e2e:run e2e/tests/settings/config-chat-popover.spec.ts` (6 passed)
- `cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/settings/mobile-configuration-chat.spec.ts` (2 passed)
