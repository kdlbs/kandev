---
status: done
created: 2026-10-23
requirements:
  - REQ-TASKS-COORDINATOR-CHILD-TASK-ORDERING-001
system_design:
  - ../../specs/tasks/system-design/coordinator-child-task-ordering.md
---

# feat(mcp): let direct parents reorder child tasks

Allow the trusted direct parent to reorder selected child tasks using existing task order and all-or-nothing membership/version checks.

## Delivery order and dependencies

The implementation is one existing PR slice. Preserve its native dependencies and existing feature defaults, then verify the narrow affected paths. No dependent PR is imported into this package.

- [Implementation and verification](task-01-implementation.md)

## Scope and risks

Task hierarchy mutation, title-derived authorization, and reordering unrelated tasks. These remain outside this delivery slice. Preserve existing files, configuration and history; report unavailable integration coverage without inventing a successful result.

## Verification strategy

Reuse the completed focused implementation checks recorded below and the existing documentation-coverage evaluator. Only remaining review fixes require their affected checks to run again.
