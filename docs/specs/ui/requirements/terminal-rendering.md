---
status: draft
system: ui
created: 2026-08-25
owners:
  - kandev
---

# Terminal Rendering Requirements

## Overview

Kandev terminal surfaces must keep text readable when the application uses a light or dark theme. The UI system owns this presentation contract because the same terminal renderer serves several task and utility flows.

## Terminology

- **Resolved theme:** The active `light` or `dark` theme after Kandev resolves the saved setting and system preference.
- **Adaptive terminal:** An xterm surface that uses the current application background and foreground colors.
- **Fixed-dark terminal:** An xterm surface that keeps its own dark background in both application themes.

## Requirements

### REQ-UI-TERMINAL-RENDERING-001: Readable terminal themes

**Intent:** Terminal prompts, commands, and output remain readable in each supported application theme. A theme change does not interrupt the terminal session.

**User story:** As a user, I want terminal text to remain readable in light and dark modes, so that I can use my selected theme.

#### Acceptance criteria

- **AC-UI-TERMINAL-RENDERING-001.1:** When an adaptive terminal opens, it shall apply the palette for the resolved theme before it shows terminal output.
- **AC-UI-TERMINAL-RENDERING-001.2:** Standard and bright ANSI text shall have a contrast ratio of at least 4.5:1 against the terminal background.
- **AC-UI-TERMINAL-RENDERING-001.3:** When the resolved theme changes, an open adaptive terminal shall update its background, foreground, cursor, selection, and ANSI colors.
- **AC-UI-TERMINAL-RENDERING-001.4:** A theme update shall preserve the terminal instance, buffer, connection, focus, running command, and scroll position.
- **AC-UI-TERMINAL-RENDERING-001.5:** Desktop, tablet, and phone task terminals shall provide the same readable output for the same resolved theme.
- **AC-UI-TERMINAL-RENDERING-001.6:** A fixed-dark terminal shall use a matching dark palette and remain readable in either application theme.

## Out of scope

- User-defined terminal palettes.
- Changes to PTY connections, terminal persistence, shell prompts, or ANSI stream interpretation.
- Changes to terminal fonts, layout, navigation, touch controls, scrolling, or keyboard behavior.
- A redesign of fixed-dark Quick Terminal or agent-login surfaces.
