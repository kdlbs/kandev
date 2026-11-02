---
created: 2026-10-29
status: done
requirements:
  - REQ-OFFICE-SCHEDULER-001
system_design:
  - ../../specs/office/system-design/scheduler-01.md
legacy_specs: []
---

# Implementation Plan: Preserve routine work context and completion settings

## Overview

A routine fire could lose the registered routine's work description, the
Office identity of a task created in the workspace's system Routine workflow,
and the trusted completion settings that belong to the routine. A heavy
routine's task template may declare `project_id`, `blocked_by`, and a trusted
`required_subtask_count` minimum. The template value is persisted under the
server-owned namespaced metadata key `kandev.office_required_subtask_count`;
the legacy generic metadata key `required_subtask_count` remains inert.
Every task-state completion write and Done-step entry uses the task repository
transaction and is blocked until enough same-workspace, non-ephemeral, durable
child tasks have a runner seat on that child. A routine-source wake coalesces only with an in-flight run
from the same routine.

This plan records the delivery of the repair. A taskless routine wake resolves
its registered routine ID, name, and description into the agent prompt. Tasks
in the same workspace's system-owned builtin Routine workflow project as
Office. Trusted creation persists the namespaced assignment minimum before
auto-start and keeps it protected from generic metadata updates. Completion
writes and Done-step entry share the same guarded transaction. The workflow
flag heal is limited to the Office Default template so configured Routine
settings are retained.

The authoritative pair is
[REQ-OFFICE-SCHEDULER-001](../../specs/office/requirements/scheduler.md) and the
[Office scheduler system design part 1](../../specs/office/system-design/scheduler-01.md).

## Scope

In scope:

- Routine task creation options for `project_id`, `blocked_by`, and the trusted
  namespaced assignment minimum.
- Routine prompt context for taskless routine wakes.
- Office identity for the same-workspace system Routine workflow.
- The template's `required_subtask_count` input, the server-owned
  `kandev.office_required_subtask_count` metadata key, and the inert legacy
  generic metadata key.
- Guarded completion in every task-state write and Done-step entry, plus the
  bounded completion reads.
- Same-routine-only coalescing for routine-source wakes.
- Keeping configured Routine workflow step flags during the builtin heal.

Out of scope:

- Replaying existing runs, reopening completed tasks, or migrating stored rows.
- Changing concurrency policy, provider routing, or attachment behavior.
- New verification systems or broad product test runs.
- Review-thread changes or any public PR update.

## Verification

- The PR head adds or updates focused tests for the adapter, routine service and
  prompt contract, namespaced requirement protection, completion guards,
  same-routine coalescing, Office identity, and builtin workflow heal. Those
  tests are the evidence for the product behavior.
- This delivery package adds documentation only. Coverage is proven by the
  repository's `.github/scripts/pr-docs.cjs` validator reading this work order,
  its sibling plan, `REQ-OFFICE-SCHEDULER-001`, and the referenced design at the
  PR head. The documentation delta does not rerun product tests or CI.

## Work orders

- [Task 01: Preserve routine context and required assignments](task-01-preserve-routine-context-and-assignments.md)
