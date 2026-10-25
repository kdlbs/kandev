---
created: 2026-09-14
status: done
requirements:
  - REQ-PLUGINS-MARKETPLACE-002
system_design:
  - ../../specs/plugins/system-design/marketplace.md
legacy_specs: []
---

# Implementation plan: Plugin marketplace previews

Extend the existing native-plugin marketplace with optional registry-owned
preview galleries while preserving legacy entries and the current installation
path. The canvas marketplace plan consumes the same registry and gallery
primitives, but the plugin requirement remains owned by this package.

## Scope

- Ordered optional preview metadata for official and custom plugin listings.
- Cover and details gallery behavior with keyboard, touch, and image failures.
- Existing no-preview rows and native install actions.
- Plugin marketplace, manifest, authoring, and registry README guidance.

## Work orders

- [ ] [Task 01: Extend plugin marketplace previews](task-01-plugin-previews.md)

Dependencies: 01. The single work order keeps the plugin requirement and its
system design in one lookup scope for documentation validation.
