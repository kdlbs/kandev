---
status: active
system: ui
specification_version: 1
migration: in_progress
owners:
  - web
---

# User Interface

## Purpose

The UI system owns shared presentation and interaction contracts used across
Kandev's web surfaces. Its consumers include desktop browsers, phone browsers,
and the desktop shell's webview.

## Ownership

- Responsive layout behavior shared by multiple product surfaces.
- Reusable overlay geometry, focus, keyboard, pointer, and touch behavior.
- Accessibility semantics implemented by shared UI primitives.
- Browser viewport adaptation, including visual-viewport changes caused by a
  mobile software keyboard.

## Exclusions

- Feature-specific result sources, filtering, and insertion semantics remain
  with the feature that invokes a shared UI primitive.
- Backend APIs, persistence, and provider integrations remain with their owning
  backend systems.
- Desktop process and window lifecycle remain with the desktop runtime.

## Specification map

### Requirements

- [Composer suggestion overlays](requirements/composer-suggestion-overlays.md)

### System design

- [Composer suggestion overlays](system-design/composer-suggestion-overlays.md)

### Legacy feature specifications

The migration is in progress. The following legacy specifications remain
authoritative for feature-specific triggers, results, selection, and
serialization. The requirement above is authoritative for their shared popup
geometry and visible-viewport behavior.

- [Entity reference composer](entity-reference-composer.md)
- [Slash command composer](slash-command-composer.md)
- [Agent launch prompt composer](agent-launch-prompt-composer.md)

## Related systems

- [Product specifications](../product/README.md): Product-wide constraints that
  UI behavior must preserve.
