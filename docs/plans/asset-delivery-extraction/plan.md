---
created: 2026-10-28
status: done
requirements:
  - REQ-PLATFORM-WEB-ASSET-DELIVERY-001
  - REQ-UI-SESSION-REFRESH-EFFICIENCY-004
system_design:
  - ../../specs/platform/system-design/web-asset-delivery.md
  - ../../specs/ui/system-design/session-refresh-efficiency.md
legacy_specs: []
---

# Asset delivery extraction

Extract the retained asset compression and deferred Monaco boundaries from
reviewed dense-workbench work order 04 and source commit `4fbc42edcfad`.
The user authorized implementation and one focused PR on current main.
The source investigation checkout remains read-only. Historical timings do not
establish current load-time, warm CPU, Firefox, or task-switch improvements.

## Work orders

- [Task 01: asset delivery](task-01-asset-delivery.md): done.

## Scope and validation

Preserve decoded JS/CSS bytes, HTTP representation semantics, editor content,
focus, and native phone viewing. Use deterministic smaller-only gzip siblings
and the existing Monaco initialization owner. No new setting or public copy.
Run the focused checks in Task 01. Serialize builds and browser suites with
`/tmp/kandev-contribution-validation-5936a872.lock` across sibling contributions.

Desktop keeps the Dockview file/diff panel with its existing loading state.
Phone keeps Files navigation and the native full-height CodeMirror viewer.
The phone viewer owns scrolling and safe-area behavior. Its file tap must not
initialize Monaco. No composition, geometry, or animation redesign is needed.
