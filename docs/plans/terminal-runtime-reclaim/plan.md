---
created: 2026-11-07
status: in_progress
requirements:
  - REQ-EXECUTORS-IDLE-PARKING-002
system_design:
  - ../../specs/executors/system-design/idle-runtime-parking.md
legacy_specs: []
---

# Terminal runtime reclaim

Release settled failed or cancelled session runtimes without losing resumable
state or interrupting pending work. This carries the existing local terminal
reclaim implementation onto current upstream lifecycle and recovery fences.

## Work order

- [ ] [Task 01: Reclaim settled terminal runtimes](task-01-terminal-runtime.md)

The restart recovery portion remains open. Do not omit a terminal record from
the startup guard inventory before its stop outcome and owner are reconciled.
