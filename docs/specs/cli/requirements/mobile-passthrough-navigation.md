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

Menu navigation and confirmation must be available when the phone key bar
first opens. [Issue #5423](https://github.com/kdlbs/kandev/issues/5423).

## Terminology

- **Key bar:** On-screen terminal keys and transient Ctrl/Shift modifiers.
- **Raw input:** Direct PTY input that creates no Kandev user message.
- **Phone:** A viewport below the application's 768 CSS-pixel phone boundary,
  regardless of pointer precision.

## Requirements

### REQ-CLI-MOBILE-PASSTHROUGH-NAV-001: Phone agent-terminal navigation

**Intent:** Complete interactive agent-CLI actions without a physical keyboard.

#### Acceptance criteria

- **AC-CLI-MOBILE-PASSTHROUGH-NAV-001.1:** When the phone task Chat surface shows
  a connected passthrough terminal with its composer and comments panel closed,
  the system shall show the terminal key bar. The bar shall include all four
  arrows, Enter, Esc, Tab, Ctrl, Shift, and dedicated Ctrl+C/Ctrl+D actions.
- **AC-CLI-MOBILE-PASSTHROUGH-NAV-001.2:** Each navigation action shall deliver
  its terminal sequence once to that visible session's agent PTY. Up, Down,
  Right, and Left shall send ESC `[A`, `[B`, `[C`, and `[D`, respectively.
  Enter shall send byte 13 (carriage return). Esc shall send byte 27, Tab byte 9,
  Ctrl+C byte 3, and Ctrl+D byte 4. Raw input shall neither reach a task shell
  nor create a Kandev user message.
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
  passthrough shall use the native terminal input and presentation. Managed
  Chat shall use its structured input controls.
- **AC-CLI-MOBILE-PASSTHROUGH-NAV-001.9:** When the key bar first opens in a
  375 CSS-pixel phone viewport, Up, Down, and Enter shall be fully visible and
  tappable without scrolling.
- **AC-CLI-MOBILE-PASSTHROUGH-NAV-001.10:** When the key strip overflows, the
  key bar shall show a visible horizontal swipe cue. Users shall be able to
  swipe the strip to reach every key.
- **AC-CLI-MOBILE-PASSTHROUGH-NAV-001.11:** The phone task-shell and Quick
  Terminal bars shall share the key order and swipe cue of the passthrough
  bar. When the selected terminal accepts input, Enter shall deliver one
  carriage return to that terminal.

## Out of scope

- Key bars in tablet and desktop layouts.
- New PTY routes, persistence, provider protocols, or structured prompt behavior.

## Related specifications

- [Mobile passthrough composer](mobile-passthrough-composer.md).
- [Implementation plan](../../../plans/mobile-passthrough-navigation/plan.md).
- [Key bar fix plan](../../../plans/mobile-terminal-keybar-discoverability/plan.md).
