---
status: done
created: 2026-10-23
requirements:
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004
system_design:
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
---

# fix(task): retain failed environments before reserving replacements

Retain failed physical environments and historical bindings while atomically reserving a fresh materializer/root for the requesting terminal session; environment-scoped worktree reuse.

## Delivery order and dependencies

The implementation is one existing PR slice. Preserve its native dependencies and existing feature defaults, then verify the narrow affected paths. No dependent PR is imported into this package.

- [Implementation and verification](task-01-implementation.md)

## Scope and risks

Deletion or resetting of old files, broad discovery, live-consumer eviction, and operating data changes. These remain outside this delivery slice. Preserve existing files, configuration and history; report unavailable integration coverage without inventing a successful result.

## Verification strategy

Reuse the completed focused implementation checks recorded below and the existing documentation-coverage evaluator. Only remaining review fixes require their affected checks to run again.
