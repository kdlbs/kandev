---
status: active
system: ui
created: 2026-09-09
owners:
  - kandev
---

# Toast Theme Requirements

## Overview

In-app toast notifications must follow the application's resolved color theme.
This includes plugin installation and update feedback on phones and desktops.
UI owns this reusable presentation contract; plugin lifecycle and platform
notification delivery retain their existing owners.

## Requirements

### REQ-UI-TOAST-THEME-001: Application theme consistency

**Intent:** Keep notification surfaces and text consistent with the surrounding
application from the first notification after page load.

#### Acceptance criteria

- **AC-UI-TOAST-THEME-001.1:** When a page opens with an explicitly selected dark
  theme, or with the system theme resolving to dark, in-app toasts shall use
  their dark background, foreground, and border colors without requiring a
  theme toggle first. This applies on mobile and desktop, including plugin
  installation and update success feedback.
- **AC-UI-TOAST-THEME-001.2:** When the resolved application theme is light,
  toasts shall use their light colors. An explicit application choice shall
  take precedence over the operating system's color preference in both modes.
- **AC-UI-TOAST-THEME-001.3:** When the resolved application theme changes,
  visible and subsequent toasts shall follow that theme without being
  dismissed, duplicated, or losing their content or actions. This includes
  system-theme changes and theme preview or discard.

## Out of scope

- Operating-system notification styling, sound, and delivery preferences.
- Changes to plugin installation, update, or error-reporting behavior.
- New toast palettes, notification placement, navigation, or touch controls.
- Changes to how theme preferences are saved.
