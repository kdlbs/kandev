---
created: 2026-08-26
status: done
requirements:
  - REQ-UI-QUICK-TERMINAL-001
system_design:
  - ../../specs/ui/system-design/quick-terminal.md
legacy_specs: []
---

# Implementation Plan: Configuration Chat Launcher Visibility

## Overview

Keep the Configuration Chat floating launcher visible and operable while its Settings popover is
open. The current component deliberately hides and disables the trigger in the open state even
though the popover is already positioned above it. One focused TDD work order adds desktop and
mobile regression coverage before removing that open-state suppression.

The UI system owns this repair because the defect affects the presentation contract of an existing
launcher. Task-backed configuration-chat identity, persistence, and lifecycle remain unchanged.

## Scope

### In scope

- Preserve the floating launcher below the open Configuration Chat panel on Settings routes.
- Let pointer, keyboard, and touch activation of the visible launcher close the open panel through
  the existing popover state transition.
- Prove the same launcher outcome in desktop Chromium and the mobile Chrome project.

### Out of scope

- Configuration-chat creation, persistence, agent-profile selection, or session restoration.
- Popover dimensions, panel content, the header close action, or Quick Chat expansion behavior.
- Settings floating-save placement and behavior.

## Technical approach

### Launcher state

Update `ConfigChatPanel` in
`apps/web/components/config-chat/config-chat-panel.tsx` so the floating `PopoverTrigger` retains
its normal accessibility, focus, pointer, and opacity state while `panel.isOpen` is true. Preserve
the controlled `Popover`, its existing open-change handler, fixed launcher geometry, and tooltip
suppression. Activating the retained trigger then closes the same popover without adding another
state path.

### Responsive contract

Desktop and mobile keep the current fixed bottom-right entry point and top-aligned popover. The
launcher is the primary thumb-reachable floating action on phones. The panel remains the only
scroll surface. The change does not affect safe areas or breakpoints. The nearest mobile exemplar
is `components/kanban/mobile-fab.tsx` for the persistent, touch-sized floating action. The existing
Configuration Chat popover remains the source for the surface geometry.

## Tests

- `AC-UI-QUICK-TERMINAL-001.2`: extend
  `apps/web/e2e/tests/settings/config-chat-popover.spec.ts` to assert that the launcher remains
  visible and enabled after opening the popover, then closes the panel when activated again.

## E2E tests

- Desktop Chromium: run the focused Configuration Chat popover spec and prove launcher visibility,
  operability, and toggle-close behavior.
- Mobile Chrome: extend `apps/web/e2e/tests/settings/mobile-configuration-chat.spec.ts` with the
  same touch path, including full launcher containment in the viewport after the panel opens.

## Work orders

- [x] [Task 01: Keep the Configuration Chat launcher visible](task-01-keep-launcher-visible.md)

## Verification results

Passed on 2026-08-26:

- `cd apps && pnpm install --frozen-lockfile`
- `cd apps/web && pnpm run typecheck`
- `cd apps/web && pnpm e2e:run e2e/tests/settings/config-chat-popover.spec.ts` (6 passed)
- `cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/settings/mobile-configuration-chat.spec.ts` (2 passed)

## Risks

- The visible trigger shares the popover's anchor position. Regression coverage must prove that it
  does not block panel controls or move outside a narrow viewport.
- A tooltip must not open over the active panel when focus returns to or hovers the retained
  launcher.
