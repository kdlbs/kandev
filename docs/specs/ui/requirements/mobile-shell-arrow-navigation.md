---
status: active
system: ui
created: 2026-10-27
owners:
  - web
---

# Mobile Shell Arrow Navigation Requirements

## Overview

Phone users need the task terminal's arrow controls to navigate interactive shell programs. UI owns this independent terminal-input interaction; task terminal lifecycle, agent passthrough, and Quick Terminal retain their separate contracts. No existing requirement owns the dedicated task-shell keybar's cursor-mode behavior: terminal rendering explicitly excludes keyboard behavior, while CLI navigation owns the agent PTY.

## Requirements

### REQ-UI-MOBILE-SHELL-ARROWS-001: Task shell arrow navigation

**Intent:** A phone user can navigate a terminal program with the visible arrow controls.

#### Acceptance criteria

- **AC-UI-MOBILE-SHELL-ARROWS-001.1:** When the active dedicated task shell requests normal cursor keys, each Up, Down, Right, and Left tap shall deliver its corresponding normal cursor key once to that shell.
- **AC-UI-MOBILE-SHELL-ARROWS-001.2:** When that shell requests application cursor keys, each of the four arrow taps shall deliver its corresponding application cursor key once to that shell, including when the mode changes during the same connection.
- **AC-UI-MOBILE-SHELL-ARROWS-001.3:** Switching task terminals or sessions shall select the current target and its current cursor mode; a previously selected terminal shall not receive the tap. The existing unavailable-sender fallback and modifier consumption behavior shall remain intact.
- **AC-UI-MOBILE-SHELL-ARROWS-001.4:** Native keyboard input, ordinary text and paste, and existing supported Ctrl-letter and Shift-Tab behavior shall retain their existing semantics. Home and End remain shell line-editing shortcuts.
- **AC-UI-MOBILE-SHELL-ARROWS-001.5:** The existing phone keybar remains reachable by touch, retains focus and its scrollable row and viewport clearance, and does not cause document-level horizontal overflow. Desktop native terminal keyboard navigation remains usable.

## Out of scope

- Agent passthrough and Quick Terminal cursor-mode changes; their existing contracts remain in force.
- New modifier-arrow chords, full-screen Home/End support, terminal layout changes, or backend protocol changes.

## Related specifications

- [CLI mobile passthrough navigation](../../cli/requirements/mobile-passthrough-navigation.md).
- [Quick Terminal](quick-terminal.md).
- [Implementation plan](../../../plans/mobile-shell-application-cursors/plan.md).
