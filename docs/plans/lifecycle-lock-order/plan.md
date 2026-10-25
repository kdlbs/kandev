---
status: in_progress
requirements:
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-002
system_design:
  - ../../specs/platform/system-design/session-recovery-convergence.md
---

# Lifecycle lock ordering

Restore prompt lifecycle transitions without blocking the global execution store.
This implements the existing recovery and publication contract; it adds no
public API, recovery policy, ownership rule, or configuration.

- [x] [Task 01: Serialize prompt lifecycle snapshots](task-01-prompt-lifecycle-snapshots.md)

## Delivery

The production change orders the prompt mutex before the store mutex when
capturing cancellation readiness, passthrough running, and recovered availability.
Current-main native recovery changes remain intact. Focused tests must prove
transition outcomes and stale-registration rejection, not merely goroutine exit.
Exact-head hosted backend, frontend and E2E verification and review dispositions
remain required before the normal squash merge.
