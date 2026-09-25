---
status: active
system: ui
created: 2026-09-16
owners:
  - kandev
---

# Composer provider icon

## Overview

Users need to recognize the CLI agent serving the composer's selected model.
UI owns this presentation-only contract; configured agent identity remains owned
by the agent system. A CLI provider is the agent application, not the model vendor.

## Requirements

### REQ-UI-COMPOSER-PROVIDER-ICON-001: Visible CLI identity

The closed composer model picker shall identify its session's CLI agent with a
small leading icon while retaining the existing model and configuration summary.

#### Acceptance criteria

- **AC-UI-COMPOSER-PROVIDER-ICON-001.1:** When session agent identity is available, the closed composer picker shall display that CLI agent's logo before the model name, and the open popover shall display the same logo beside its Model heading. Changing models within the same agent shall retain the agent logo; switching sessions shall reflect the newly selected session.
- **AC-UI-COMPOSER-PROVIDER-ICON-001.2:** When the logo cannot load, the picker shall show a neutral terminal fallback without blocking model selection. When agent identity is unavailable, it shall retain the existing text-only trigger without guessing from the model name.
- **AC-UI-COMPOSER-PROVIDER-ICON-001.3:** On desktop and phone, the icon shall remain visible without shrinking, the model summary shall truncate when necessary, and the chevron and model selection shall remain usable. Phone/coarse-pointer hit targets shall remain at least 44px, with no document horizontal overflow. Keyboard interaction and the trigger's accessible name shall remain intact.

## Out of scope

Changing provider identity, model selection semantics, model-list rows, settings
pickers, provider switching, backend APIs, or unrelated composer controls.
