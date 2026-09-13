---
status: done
requirements:
  - REQ-TASKS-CROSS-WORKSPACE-HANDOFF-001
  - REQ-TASKS-CROSS-WORKSPACE-HANDOFF-002
system_design:
  - ../../specs/tasks/system-design/cross-workspace-task-handoff.md
legacy_specs: []
---

# Implementation Plan: Cross-workspace task handoff

## Overview

Deliver the Office-only `handoff_task_kandev` flow. Keep authorization at the
trusted MCP boundary, persist two-way provenance, preserve idempotent outcomes,
and retain an explicit agent profile when a handoff starts later.

## Scope

- Add the permission, MCP tool, handler, provenance records, reverse-link CAS,
  and activity events described by the requirements.
- Preserve the selected agent and executor profiles through deferred session
  preparation and launch.
- Keep ordinary task creation and same-workspace flows unchanged.
- Cover authorization, resource resolution, idempotency, metadata boundaries,
  concurrency, and deferred-start behavior with focused tests.

## Work orders

- [Task 01: Cross-workspace task handoff](task-01-cross-workspace-task-handoff.md)

## Delivery notes

The implementation follows the detailed source specification in
`docs/specs/cross-workspace-task-handoff/spec.md`. No public UI or public
documentation surface is part of this delivery.
