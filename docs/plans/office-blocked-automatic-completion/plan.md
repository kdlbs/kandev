---
created: 2026-10-29
status: done
requirements:
  - REQ-OFFICE-TASKS-001
system_design:
  - ../../specs/office/system-design/tasks-02.md
legacy_specs: []
---

# Implementation Plan: Preserve blocked Office outcomes during automatic completion

## Overview

An Office task explicitly marked BLOCKED could still reach the automatic
turn-end or child-completion path and be overwritten as COMPLETED. A
concurrent state or workflow-step change could also arrive between the
automatic completion read and its write. This plan records the delivery of a
narrow repair: keep blocked Office tasks waiting, and condition the automatic
terminal write on the observed state and workflow step in one repository
update. Manual status changes, approval redirects, and non-Office completion
keep their existing paths. A stale automatic write stops before publishing
completion side effects.

The authoritative pair is
[REQ-OFFICE-TASKS-001](../../specs/office/requirements/tasks.md) and the
[Office tasks system design part 2](../../specs/office/system-design/tasks-02.md).

## Scope

In scope:

- Refuse the automatic terminal transition for an Office task whose observed
  state is BLOCKED, leaving it waiting and publishing no completion side effect.
- Persist the automatic terminal write with a compare-and-set on both the
  observed task state and the observed workflow step.
- Keep the existing manual, approval-redirect, and non-Office completion paths.

Out of scope:

- Reopening completed tasks, replaying history, or migrating task rows.
- Changing review threads, review ownership, or the public PR.
- New verification systems or broad product test runs.

## Verification

- The changed delta is documentation only; the gate check is the existing
  `.github/scripts/pr-docs.cjs` coverage validator against the PR head.
- The linked work order names the existing regression coverage for blocked
  turn-end and terminal handling and the state-plus-step compare-and-set.

## Work orders

- [Task 01: Preserve blocked outcomes during automatic completion](task-01-preserve-blocked-completion.md)
