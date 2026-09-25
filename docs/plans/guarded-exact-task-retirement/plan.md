---
created: 2026-09-25
status: in_progress
requirements:
  - REQ-TASKS-EXACT-RETIREMENT-001
  - REQ-TASKS-EXACT-RETIREMENT-002
  - REQ-TASKS-EXACT-RETIREMENT-003
system_design:
  - ../../specs/tasks/system-design/guarded-exact-task-retirement.md
legacy_specs: []
---

# Implementation plan: Guarded exact-task retirement

## Overview

Provide a new, fenced retirement path for one old task and one same-workspace
replacement. The first wave defines the durable contract and a read-only,
fail-closed preview; later waves consume verified handoff/preservation evidence
and add the operation ledger and physical executor.

## Work orders

- [x] [Task 01: Record exact-retirement contract](task-01-record-contract.md)
- [x] [Task 02: Add read-only exact-pair preview](task-02-read-only-preview.md)
- [ ] Task 03: Integrate preservation, FIFO handoff, and pending-move evidence.
- [ ] Task 04: Add the fenced operation ledger and admission claim.
- [ ] Task 05: Add exact physical cleanup under worktree guards.
- [ ] Task 06: Finalize old-only deletion, tombstone, and recovery.
- [ ] Task 07: Document and test the operator contract.

## Dependency order

Tasks 01 and 02 run before preservation work. Task 03 consumes source-manifest
and pending-move contracts. Task 04 waits for environment admission. Tasks 05
and 06 wait for the ledger and terminal-retention contract. No work order may
touch the ten reproduction tasks or archives.
