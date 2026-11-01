---
status: active
system: ui
created: 2026-10-28
owners:
  - kandev
---

# Terminal Output Navigation Requirements

## Overview

Users reading earlier task terminal output need a direct way back to the newest output. UI owns this reusable viewport interaction across task shell and dev-server output; terminal transport and lifecycle remain with their existing owners.

## Terminology

- **Latest output:** The bottom of the active normal terminal scrollback.
- **Usable terminal:** An initialized, visible terminal whose content is not covered by a connection, unavailable, or search overlay.

## Requirements

### REQ-UI-TERMINAL-OUTPUT-NAVIGATION-001: Return to latest output

**Intent:** A user can return from scrollback to current output with one visible action while retaining control of their reading position.

#### Acceptance criteria

- **AC-UI-TERMINAL-OUTPUT-NAVIGATION-001.1:** On a usable task shell or dev-server output terminal, a visible, keyboard-accessible and localized action shall appear when the normal-buffer viewport is above the latest output. It shall be absent at the bottom, while an overlay obstructs the terminal, and when the alternate buffer is active.
- **AC-UI-TERMINAL-OUTPUT-NAVIGATION-001.2:** Activating the action shall move that terminal to its latest output and remove the action. It shall not send terminal input, clear output, reconnect, change the running process, or unconditionally move focus to the terminal or steal it from another control.
- **AC-UI-TERMINAL-OUTPUT-NAVIGATION-001.3:** New output while the user reads above the bottom shall preserve the viewport's reading position and keep the action available, subject to normal scrollback eviction. Output at the bottom shall retain normal terminal follow behavior.
- **AC-UI-TERMINAL-OUTPUT-NAVIGATION-001.4:** Desktop task shells and dev-server output and the existing phone task-shell surface shall offer the same navigation outcome. The phone action shall have a minimum 44 by 44 CSS pixel touch target, remain within the usable terminal area and clear safe areas, existing controls and the software keyboard.
- **AC-UI-TERMINAL-OUTPUT-NAVIGATION-001.5:** Scrolling, resizing, output, buffer switches, terminal replacement and unmount shall keep action visibility consistent with the current terminal without stale controls or duplicate subscriptions. Existing wheel, touch, search, terminal input and streaming behavior shall continue.

## Out of scope

- Agent fullscreen TUI, Quick Terminal, login terminals, and adding a phone dev-server surface.
- Forced follow, output clearing, shell bytes, reconnect, transport, buffering or persistence changes.
