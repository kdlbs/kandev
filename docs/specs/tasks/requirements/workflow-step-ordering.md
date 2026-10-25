---
status: active
system: tasks
created: 2026-10-01
owners:
  - kandev
---

# Workflow Step Ordering Requirements

## Overview

Workflow authors reorder steps without losing their saved configuration. The
task system owns this contract because it owns workflow definitions and their
persisted order. These criteria specify the approved persistence repair;
settings draft and Save interactions retain their existing contract.

## Requirements

### REQ-TASKS-WORKFLOW-STEP-ORDERING-001: Complete and atomic step ordering

**Intent:** Save a complete step order while preserving independently saved settings.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-STEP-ORDERING-001.1:** A successful reorder shall place
  every current step of the named workflow exactly once in the requested order,
  using consecutive positions starting at zero.
- **AC-TASKS-WORKFLOW-STEP-ORDERING-001.2:** A reorder shall preserve saved step
  configuration, including a prompt, profile, or completion setting successfully
  saved by another caller while the reorder is pending. Only ordering and its
  modification timestamp shall change.
- **AC-TASKS-WORKFLOW-STEP-ORDERING-001.3:** When any reorder write fails, all
  steps shall retain their prior positions and modification timestamps.
- **AC-TASKS-WORKFLOW-STEP-ORDERING-001.4:** Duplicate, missing, foreign, or
  omitted step IDs shall reject the entire reorder without changing any step.
  An empty order shall be valid only for a workflow with no steps.
- **AC-TASKS-WORKFLOW-STEP-ORDERING-001.5:** Reorder shall retain existing
  workflow authorization, sync-managed immutability, and session-target order
  restrictions. Rejection shall not disclose a foreign workflow's step data.

## Related contracts

- [Task completion](task-completion.md), especially
  `AC-TASKS-COMPLETION-001.4`, owns completion-setting preservation and final-step eligibility.
- [Settings manual save](../../ui/requirements/settings-manual-save.md) owns
  draft persistence and contributor-level failures.

## Out of scope

- Atomicity across a complete settings Save, step creation/deletion, or separate content edits.
- New order-version APIs, conflict-resolution UI, schema migrations, or workflow transitions.

## Implementation plans

- [Workflow step reorder atomicity](../../../plans/workflow-step-reorder-atomicity/plan.md)
