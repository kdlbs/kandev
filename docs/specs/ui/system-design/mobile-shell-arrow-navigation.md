---
status: current
system: ui
requirements:
  - REQ-UI-MOBILE-SHELL-ARROWS-001
---

# Mobile Shell Arrow Navigation System Design

## Purpose and mapping

This design owns the dedicated task shell's phone keybar input interaction, independently of task-shell lifecycle and of the agent and Quick Terminal socket owners. `REQ-UI-MOBILE-SHELL-ARROWS-001` maps to input ownership, compatibility, and verification below.

## Input ownership and mode

`MobileTerminalPane`'s active `TerminalSlot` owns its xterm and open dedicated WebSocket. `mobile-active-terminal.ts` currently registers the raw sender with a session guard and identity-safe cleanup. Associate a live cursor-mode reader with that same registration, scoped to the selected terminal and session; read `xterm.modes.applicationCursorKeysMode` at each arrow tap, not at mount or in React state. A missing xterm or registration means normal mode. Replace and release sender and mode as one ownership unit so a stale cleanup cannot clear a successor. Keep existing registration and fallback availability during xterm/socket initialization and reconnect.

The task shell keybar resolves only its four arrow *actions* to CSI `ESC [ A/B/C/D` in normal mode or SS3 `ESC O A/B/C/D` in application mode. Select once for the same active sender and send one input. If that sender throws and the existing default-shell fallback is used, use the normal sequence because no mode is known for the fallback. If no sender exists or its session does not match, retain the existing normal-sequence fallback. Preserve successful-send, unavailable-client, and modifier consumption semantics.

Do not translate arbitrary byte chunks at `sendShellInput`: xterm `onData` already emits mode-aware native keys and also carries text and paste. Non-arrow bar keys and modifier transforms remain unchanged; Ctrl/Shift do not acquire new arrow chords. The raw sender calls `ws.send` with encoded bytes; it does not call xterm paste. Update stale comments where touched. `MobileAgentTerminal` and `PtyTerminalView` use their own `onSend` callbacks and do not consult the task-shell registry; keep their present CSI behavior and contracts.

## Phone and desktop composition

The entry is task navigation's Terminal panel and terminal picker. The existing `MobileTerminalPane` plus `MobileTerminalKeybar` is the closest shipped terminal exemplar; `task-layout.tsx` provides the curated focused phone layout. Retain the full-height terminal, its vertical output scrolling, the keyboard-aware fixed keybar and safe-area/bottom navigation clearance. The keybar itself scrolls horizontally and keeps 44px touch targets; the document must not horizontally scroll. A persistent row supports frequent navigation without a drawer. No markup, copy, layout, or desktop composition changes are proposed. Desktop continues to use xterm's native keyboard path.

## Verification

Use a real shell PTY and an independently authored curses program with `keypad(True)` as the application-mode user-outcome oracle, after a visible ready marker. Tap all four actual phone keys and assert the program recognizes each corresponding key; establish the same tests fail causally before production changes. Separately demonstrate normal-mode behavior with a real PTY consumer, native keyboard behavior, mode transitions, and terminal ownership. Unit tests cover exact mappings, registration replacement and session isolation, modifiers, fallback, and unchanged native/text/paste paths. Check actual key bounds, tap reachability and document overflow in the phone browser; desktop native keyboard receives a separate desktop-project check. No new persistence, backend API, permissions, or telemetry are required.

## Related delivery

- [Implementation plan](../../../plans/mobile-shell-application-cursors/plan.md).
