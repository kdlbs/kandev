---
status: current
system: ui
requirements:
  - REQ-UI-TERMINAL-TOUCH-SCROLLING-001
---

# Terminal Touch Scrolling System Design

## Purpose and boundaries

This design defines touch scrolling for `PassthroughTerminal`. The same renderer serves task-agent and task-shell terminal surfaces.

The design keeps the current xterm instance, WebSocket, buffer, and responsive layouts. It changes only touch-handler activation for coarse pointers.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-TERMINAL-TOUCH-SCROLLING-001` | [Pointer activation](#pointer-activation), [Gesture flow](#gesture-flow), [Responsive behavior](#responsive-behavior), [Test contract](#test-contract) |

## Confirmed fault

PR 1046 added the touch-scroll handler and enabled it for the phone layout. Current phone Chrome evidence shows that the handler moves TUI scrollback.

`PassthroughToolbar` enables the handler from `isMobile`. This value becomes false at 768 CSS pixels, including coarse-pointer tablet and landscape layouts.

A trusted browser touch at 820 CSS pixels leaves the TUI viewport unchanged. The handler is not active in that layout.

## Components and responsibilities

- `useResponsiveBreakpoint` reports pointer precision and the responsive layout.
- `PassthroughToolbar` opts agent passthrough terminals into touch scrolling for coarse pointers.
- `PassthroughTerminal` resolves the shared activation rule and passes the result to `useTouchScroll`.
- `useTouchScroll` attaches and removes one handler for the current xterm instance.
- `attachTouchScroll` converts vertical touch distance to xterm row movement.
- The application shell prevents document overscroll through its existing global style.

## Pointer activation

`PassthroughTerminal` resolves touch scrolling from pointer precision and terminal mode. Coarse-pointer shell terminals enable the handler by default. Agent terminals opt in through their caller, and fine pointers always disable the handler.

This rule is independent of viewport width. A pointer-mode change causes the responsive hook to render again and updates the effect for every shell caller.

Fine-pointer layouts do not install the custom handler. Their xterm selection and wheel behavior stay on the existing path.

## Gesture flow

1. A single touch starts on the xterm canvas and bubbles to the terminal host.
2. The handler waits until vertical movement crosses the current threshold.
3. The handler prevents the document action and converts whole-row distance to `scrollLines` calls.
4. The handler keeps fractional row distance for the next move in the same direction.
5. A touch end or cancellation clears the gesture state.

The handler yields before activation for a tap, a multi-touch gesture, or a horizontal-dominant drag.

## Responsive behavior

- **Desktop outcome:** Fine-pointer selection and wheel scrolling do not change.
- **Touch entry point:** The existing task Chat panel contains the passthrough terminal.
- **Nearest implementation:** `MobileTerminalPane` uses the same renderer and touch-scroll handler.
- **Presentation:** Phone, tablet, and desktop layouts keep their current composition.
- **Scroll owner:** Xterm owns terminal scrollback. The application document stays fixed to the viewport.
- **Shared behavior:** All layouts use the same handler, buffer, and connection. `PassthroughTerminal` enables shell scrolling for coarse pointers, while the agent toolbar opts in through its caller.

## Failure and recovery

If pointer detection is unavailable, the responsive hook keeps its current fine-pointer default. This behavior avoids a custom handler on an unknown desktop.

If xterm is not ready, `useTouchScroll` does not attach the handler. The readiness change attaches it to the initialized instance.

The cleanup path removes each listener. A remount or pointer-mode change cannot leave a duplicate listener.

## Test contract

Component tests prove that `PassthroughToolbar` enables touch scrolling for a coarse pointer. They also prove that a fine pointer disables it.

The current helper tests cover threshold, direction, accumulation, multi-touch, horizontal movement, and listener cleanup.

The mobile Playwright test uses a real `cli_passthrough` profile. It changes to an 820-pixel coarse-pointer viewport and creates enough output for scrollback.

Trusted browser touch input starts on the xterm canvas. The test proves that the xterm viewport moves while the document remains fixed.

## Related decisions

No architecture decision applies. This correction keeps the existing touch-scroll design from PR 1046.
