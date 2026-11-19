---
created: 2026-11-19
status: done
requirements:
  - REQ-TASKS-TRANSIENT-LAUNCH-RETRY-001
system_design:
  - ../../specs/tasks/system-design/transient-launch-retry.md
legacy_specs: []
---

# Implementation plan: Retry a transient launch failure once

## Overview

A launch that fails because the agent runtime or the workspace was briefly
not ready (for example "agentctl not ready: context deadline exceeded") leaves
the session and the task `FAILED` until the user retries by hand. This package
adds one delayed automatic retry through the existing runtime retry recovery,
as defined by `REQ-TASKS-TRANSIENT-LAUNCH-RETRY-001` and its
[system design](../../specs/tasks/system-design/transient-launch-retry.md).

## Scope

In scope: transient classification, the once-per-session claim, the
service-owned retry worker and its shutdown, and the failure-path hook in
`handleSessionLaunchFailure`.

Out of scope: retries after a turn started, configurable counts or delays,
and any frontend change.

## Work orders

- [x] [Task 01: Retry a transient launch failure once](task-01-retry-transient-launch.md)

One work order: one failure-path hook and its worker, with one verification
boundary.

## Verification strategy

Orchestrator tests cover a real executor launch failure, resume-token
preservation, automatic capacity deferral, replacement failures, workspace-only
preparation, worker restart, shutdown cancellation, once-per-hour claims, and
non-transient failures. No end-to-end test: the mock agent cannot fail its
startup transiently on demand.

## Risks

- A retry could repeat side effects of a failed start. It is limited to a
  closed list of startup failures, runs once per session per hour, re-checks
  failure identity at resume admission, keeps automatic capacity provenance,
  and uses the same recovery path as the manual retry action.

## References

- [Requirement](../../specs/tasks/requirements/transient-launch-retry.md)
- [System design](../../specs/tasks/system-design/transient-launch-retry.md)
