---
created: 2026-11-07
status: completed
requirements:
  - REQ-TASKS-WIP-LIMIT-PULL-SYSTEM-001
system_design:
  - ../../specs/tasks/system-design/wip-limit-pull-system.md
legacy_specs: []
---

# Cross-step promotion entry identity

A pulled feeder task needs its committed entry identity in the existing
`task.moved` event so the destination's entry handler can distinguish it from
the preceding step's route.

- [x] [Task 01: Carry the committed entry identity](task-01-carry-entry-identity.md)
