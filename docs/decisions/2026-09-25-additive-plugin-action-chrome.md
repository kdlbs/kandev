# ADR-2026-09-25-additive-plugin-action-chrome: Additive plugin action controls

**Status:** accepted
**Date:** 2026-09-25
**Area:** frontend, protocol

## Context

Native UI plugins currently register arbitrary React components. Some use host
buttons, while others own raw buttons, CSS, animated glyphs, or pointer handlers.
A mandatory renderer cannot preserve all these behaviors without plugin changes.
The user selected backward-compatible introduction and gradual adoption.

## Decision

Add an optional, location-aware host action component to existing component slots.
The host owns the outer control styling. Plugins retain state and interaction
handlers. Native controls and the new component share location-specific primitives.

Keep existing slot registrations, context, ordering identity, and host Button
behavior. Do not restyle legacy descendants through new broad CSS selectors.
Plugins can detect the new export and select one rendering path.
No plugin update is required to run on the upgraded host.

## Consequences

Standard styling applies after adoption. Mixed old and new controls remain
supported. The host must test both paths and keep its SDK runtime-free.
Specialized interactions need typed refs and event handlers, not just an
activation callback. Existing status-bar geometry remains a named exception.
Official plugin releases can adopt the component independently.

## Alternatives Considered

- Force a semantic action registry immediately: too restrictive for existing
  press-and-hold controls, animated content, and controlled disclosures.
- Override every legacy button through CSS: changes plugin layouts and misses
  raw controls, inline styles, and portal content.
- Publish styling instructions only: authors must copy classes, which drift
  when native controls change.
- Change the shared host Button globally: also changes plugin forms and dialogs
  outside the intended action locations.

## Related design

- [Plugin action UX](../specs/plugins/system-design/plugin-action-ux.md)
