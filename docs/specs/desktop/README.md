---
status: draft
system: desktop
specification_version: 1
migration: complete
owners:
  - kandev
---

# Desktop system

## Purpose

The desktop system owns the Tauri shell, native process supervision, desktop
boot and shutdown behavior, and desktop-only integration boundaries.

## Ownership

This system owns desktop launch lifecycle, native window and backend process
coordination, desktop health handoff, packaging-facing runtime contracts, and
desktop-specific behavior that is not part of the web UI contract.

## Exclusions

- Shared backend launch behavior belongs to the [platform system](../platform/README.md).
- Responsive web behavior belongs to the [UI system](../ui/README.md).
- Release publication belongs to the [release system](../release/README.md).

## Specification map

### Requirements



- [Tauri Desktop App](requirements/desktop-tauri-app.md)

### System design



- [Tauri Desktop App](system-design/desktop-tauri-app.md)

## Migration record

All legacy sources assigned to this system are now represented by the canonical requirement and system-design documents above. Source detail is retained in those documents or in their linked design parts.

## Related systems

- [Platform](../platform/README.md): supplies the native runtime contract.
- [UI](../ui/README.md): supplies the web application surface.
