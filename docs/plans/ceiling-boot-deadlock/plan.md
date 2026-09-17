---
created: 2026-09-17
status: done
requirements:
  - REQ-AGENTS-SESSION-CEILING-001
system_design:
  - ../../specs/agents/system-design/session-concurrency-ceiling.md
legacy_specs: []
---

# Implementation Plan: Ceiling replay boot deadlock

## Overview

Restore deferred replay and independent task admission after an agent boots.
Two live goroutine snapshots on September 17 confirmed a circular wait.
Replay held task admission and waited for the session guard.
The boot callback held the session guard and waited for task admission.
It also held the global task-state mutex, which blocked unrelated launches.

Existing replay and manual-admission criteria define the intended behavior.
The user requested this package, implementation, and PR in the same turn.

## Scope

### In scope

- Limit replay admission locking to validation.
- Preserve durable claims and final workflow-binding validation.
- Acquire task admission before the global task-state mutex.
- Cover replay progress and independent scheduling with regression tests.

### Out of scope

- Live-instance restart, database changes, backup performance, and plugin warnings.
- Changes to ceiling limits, manual overrides, or rendered UI.

## Technical approach

`ceiling_replay.go` releases its validation lock before concrete replay dispatch.
The caller context must not retain the released lock's ownership marker.
Durable claims still exclude concurrent replay and Send Now.
Existing launch and prompt boundaries still validate the immutable workflow binding.

`writeTaskReviewState` in `event_handlers_streaming.go` acquires task admission
before `taskRuntimeStateMu`. A task-local wait cannot hold the global mutex.
The agent system design records these lock lifetimes.

## Tests

`ceiling_boot_deadlock_test.go` covers:

- `TestCeilingReplayReleasesAdmissionBeforeProviderDispatch`: AC-AGENTS-SESSION-CEILING-001.5.
- `TestReviewAdmissionWaitDoesNotBlockIndependentScheduling`: AC-AGENTS-SESSION-CEILING-001.2.

Existing ceiling replay tests cover stale workflow entries and durable claims.
Existing boot-ready tests cover session settlement and queue draining.

## E2E tests

Service tests exercise scheduling and replay through the real repository and executor.
Only the provider boundary is simulated. No rendered UI changes require browser coverage.

## Work orders

- [x] [Task 01: Break the replay lock cycle](task-01-break-lock-cycle.md)

## Verification results

Both new regression tests failed on the original code for the expected lock waits.
Both passed after the correction (0.156 seconds).
The work-order race command passed (4.449 seconds).
The documentation catalog validated 288 decisions and 1002 specifications.
The specification linter and git diff whitespace check passed.
The package status check found both new documents before staging.

## Risks

Releasing admission before dispatch must preserve final workflow-binding checks
and must not leak lock ownership into callbacks.
