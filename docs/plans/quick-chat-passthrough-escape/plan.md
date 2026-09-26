---
created: 2026-08-28
status: done
requirements:
  - REQ-UI-QUICK-TERMINAL-001
system_design:
  - ../../specs/ui/system-design/quick-terminal.md
legacy_specs: []
---

# Implementation Plan: Quick Chat Passthrough Escape

## Overview

Make the existing configurable Quick Chat action a safe dialog toggle, then let a focused agent
passthrough terminal keep unmodified Escape for its TUI. Both changes ship as one vertical repair
because terminal Escape cannot be freed until the shared dialog has a discoverable keyboard close
path.

## Scope

### In scope

- Toggle the shared Quick Chat dialog from the existing configurable `QUICK_CHAT` action.
- Keep other Quick Chat launchers open/select-only.
- Prevent Radix dismissal only while a connected xterm terminal with an installed AttachAddon and
  open WebSocket owns unmodified Escape.
- Preserve xterm's normal Escape-byte delivery to the agent terminal WebSocket.
- Preserve nested Quick Chat escape guards and the existing mobile close action.

### Out of scope

- A new "Escape closes Quick Chat" preference.
- Changes to agent cancellation APIs or provider-specific key mappings.
- Changes to bottom terminal, mobile terminal, or Settings terminal composition.
- Changes to pointer dismissal or Quick Chat geometry.

## Technical approach

### Configurable dialog toggle

Extend `useQuickChatLauncher` with an explicit toggle option. When enabled and the live shared
dialog state is open, request the registered modal close handler and fall back to `closeQuickChat`
when no handler exists; otherwise retain the current kind-scoped session/setup selection. Enable
this option only for the ordinary Quick Chat action registered by
`GlobalCommands`, which already resolves the user's `QUICK_CHAT` override.
Register that binding in capture phase with propagation stopped after a match so it wins before
xterm and remains an application command rather than terminal input.

### Focus-scoped terminal Escape

Add a stable Escape predicate to `PassthroughTerminal` and register it through
`useClarificationEscapeGuard`. Arm it only while the terminal is connected, its AttachAddon is
installed, and its WebSocket is open; then match only unmodified Escape directed at xterm's helper textarea.
The dialog prevents its own dismissal while allowing the same keyboard event to continue to xterm
and `AttachAddon`.

### Responsive behavior

The desktop outcome is a keyboard-toggleable shared dialog whose focused passthrough terminal owns
Escape. The mobile entry remains the existing Home/task-switcher Quick Chat action, and the nearest
shipped exemplar is `mobile-quick-chat-entry.spec.ts`: the full-height dialog keeps its visible
close control, internal scroll ownership, dynamic viewport sizing, and safe-area behavior. No
mobile composition or touch target changes are required.

## Tests

- `apps/web/hooks/use-quick-chat-launcher.test.ts`: cover opt-in close behavior, retained default
  open behavior, and session preservation through the existing close action.
- `apps/web/components/quick-chat/quick-chat-focus.test.ts` and
  `apps/web/components/quick-chat/use-quick-chat-modal.test.ts`: cover the registered close
  lifecycle and cancellation of a deferred agent start.
- `apps/web/components/global-commands.test.tsx`: prove the configurable ordinary Quick Chat action
  opts into toggle behavior while Configuration Chat does not, and that the keyboard listener has
  capture-phase precedence over xterm.
- `apps/web/components/task/use-passthrough-terminal.test.ts`: cover the focus- and modifier-scoped
  Escape predicate.
- `apps/web/components/quick-chat/quick-chat-modal.test.tsx`: retain the existing nested-widget
  escape-guard regressions.

## E2E tests

- `apps/web/e2e/tests/chat/quick-chat.spec.ts`: for
  `AC-UI-QUICK-TERMINAL-001.10` and `AC-UI-QUICK-TERMINAL-001.11`, create a passthrough Quick Chat,
  focus xterm, observe an Escape frame on its terminal WebSocket, assert the dialog remains open,
  then use the configured Quick Chat shortcut to close it.
- `apps/web/e2e/tests/chat/mobile-quick-chat-entry.spec.ts`: existing
  `AC-UI-QUICK-TERMINAL-001.12` coverage proves the phone entry and explicit touch close path.

## Work orders

- [x] [Task 01: Repair Quick Chat passthrough cancellation](task-01-passthrough-cancellation.md)

## Verification results

- Focused Vitest suites: 6 files and 71 tests passed, including the registered close lifecycle and
  deferred-start cancellation regression.
- Desktop Chromium E2E: passthrough Escape delivery and Quick Chat shortcut dismissal passed.
- Mobile Chrome E2E: Home-header launch and explicit touch dismissal passed.
- Targeted ESLint, specification lint, and `git diff --check` passed.

## Risks

- A predicate broader than xterm's helper textarea could swallow Escape from terminal search or
  other nested controls.
- Calling `stopPropagation()` would starve xterm; the dialog guard must use `preventDefault()` only.
- Applying toggle semantics to every launcher would change existing kind-selection behavior.
