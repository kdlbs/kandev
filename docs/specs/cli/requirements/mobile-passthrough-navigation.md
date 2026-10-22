---
status: active
system: cli
created: 2026-10-09
owners:
  - web
---

# Mobile Passthrough Navigation Requirements

## Overview

Phone users need terminal navigation and control keys to interact with native
agent menus and permission prompts. This capability addresses
[issue #4370](https://github.com/kdlbs/kandev/issues/4370). CLI owns the contract
because these actions deliver raw input to an agent PTY, independently of the
Kandev composer and task shell.

## Terminology

- **Navigation bar:** On-screen terminal keys and transient Ctrl/Shift modifiers.
- **Raw input:** Direct PTY input that creates no Kandev user message.
- **Phone:** A viewport below the application's 768 CSS-pixel phone boundary,
  regardless of pointer precision.

## Requirements

### REQ-CLI-MOBILE-PASSTHROUGH-NAV-001: Phone agent-terminal navigation

**Intent:** Complete interactive agent-CLI actions without a physical keyboard.

#### Acceptance criteria

- **AC-CLI-MOBILE-PASSTHROUGH-NAV-001.1:** When the phone task Chat surface shows
  a connected passthrough terminal with its composer and comments panel closed,
  the system shall show the existing terminal navigation bar, including all
  four arrows, Esc, Tab, Ctrl, Shift, and dedicated Ctrl+C/Ctrl+D actions.
- **AC-CLI-MOBILE-PASSTHROUGH-NAV-001.2:** Each navigation action shall deliver
  its terminal sequence once to that visible session's agent PTY. Up, Down,
  Right, and Left shall send ESC `[A`, `[B`, `[C`, and `[D`; Esc shall send
  byte 27, Tab byte 9, Ctrl+C byte 3, and Ctrl+D byte 4. Raw input shall neither
  reach a task shell nor create a Kandev user message.
- **AC-CLI-MOBILE-PASSTHROUGH-NAV-001.3:** One Ctrl tap shall modify the next
  single letter typed into the terminal and then release. A second tap shall
  make Ctrl sticky until toggled off. Shift+Tab shall send reverse Tab. Paste
  and multi-character input shall retain the existing terminal modifier behavior.
- **AC-CLI-MOBILE-PASSTHROUGH-NAV-001.4:** The phone bar shall reserve its own
  space, retain terminal keyboard focus on tap, and remain reachable when the
  software keyboard or browser chrome changes the visible viewport. Terminal
  output and input controls shall not overlap the bar or software keyboard;
  document-level horizontal overflow shall remain zero. Each key shall have
  an accessible name and a touch target of at least 44 by 44 CSS pixels.
- **AC-CLI-MOBILE-PASSTHROUGH-NAV-001.5:** When the composer or comments panel
  opens, the raw navigation bar shall be hidden and modifiers cleared. Composer
  text, drafts, attachments, and explicit send shall retain their existing
  behavior; closing the panel shall restore navigation for a connected terminal.
- **AC-CLI-MOBILE-PASSTHROUGH-NAV-001.6:** While the agent terminal is connecting
  or disconnected, navigation shall be unavailable and input shall not be queued,
  replayed, or rerouted. Reconnection shall target the replacement connection;
  a close event from an older connection shall not disable the newer one.
- **AC-CLI-MOBILE-PASSTHROUGH-NAV-001.7:** Leaving the passthrough surface,
  changing sessions, or leaving phone layout shall clear its transient modifiers
  and input ownership. Later terminal input shall reach only its selected target.
- **AC-CLI-MOBILE-PASSTHROUGH-NAV-001.8:** At widths of 768 CSS pixels and above,
  passthrough shall retain its current input and presentation. Managed Chat,
  task-shell terminals, and Quick Terminal shall retain their existing controls.

## Out of scope

- Tablet-specific navigation additions and desktop floating-bar changes.
- Quick Terminal floating-bar changes, already implemented separately.
- New PTY routes, persistence, provider protocols, or structured prompt behavior.
- New Enter buttons; raw Enter remains available through the terminal keyboard.

## Related specifications

- [Mobile passthrough composer](mobile-passthrough-composer.md).
- [Implementation plan](../../../plans/mobile-passthrough-navigation/plan.md).
