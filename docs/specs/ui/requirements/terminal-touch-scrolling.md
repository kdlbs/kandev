---
status: active
system: ui
created: 2026-09-02
owners:
  - kandev
---

# Terminal Touch Scrolling Requirements

## Overview

Kandev terminal surfaces use xterm for task agents and shell sessions. The UI system owns this shared touch interaction contract.

The xterm canvas receives touch input, but xterm does not move its scrollback for a touch drag. Kandev must connect that gesture to scrollback.

## Terminology

- **Coarse pointer:** A primary pointer that the browser reports through `(pointer: coarse)`.
- **Terminal scrollback:** Output above the current xterm viewport.
- **Touch-scroll handler:** The Kandev handler that converts a touch drag to xterm row movement.

## Requirements

### REQ-UI-TERMINAL-TOUCH-SCROLLING-001: Reachable terminal scrollback

**Intent:** Users can inspect earlier terminal output from a touch device at each supported viewport width.

**User story:** As a touch user, I want to move through terminal scrollback, so that I can inspect earlier output.

#### Acceptance criteria

- **AC-UI-TERMINAL-TOUCH-SCROLLING-001.1:** When the primary pointer is coarse, a vertical single-touch drag shall move the connected terminal through its scrollback.
- **AC-UI-TERMINAL-TOUCH-SCROLLING-001.2:** The touch-scroll handler shall remain available at phone, tablet, and desktop viewport widths.
- **AC-UI-TERMINAL-TOUCH-SCROLLING-001.3:** A terminal touch drag shall not scroll or refresh the application document.
- **AC-UI-TERMINAL-TOUCH-SCROLLING-001.4:** A tap, multi-touch gesture, or horizontal-dominant drag shall not move terminal scrollback.
- **AC-UI-TERMINAL-TOUCH-SCROLLING-001.5:** Fine-pointer terminal selection and wheel scrolling shall not use the touch-scroll handler.

## Out of scope

- Native momentum or a custom inertia curve.
- Changes to terminal transport, buffering, persistence, fonts, or themes.
- An xterm fork or an upstream xterm contribution.
- Touch scrolling in terminal products that do not use `PassthroughTerminal`.
