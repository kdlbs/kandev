---
status: draft
system: platform
created: 2026-08-26
owners:
  - kandev
---
# Agentctl instance stop idempotency Requirements

## Overview

The agentctl control API can receive a second stop request while the first
request is finishing teardown. The instance manager removes the instance after
successful cleanup, so the second request currently reports a 500 even though
the requested stopped state is already true. This requirement makes repeated
stops converge on the same safe postcondition while preserving real cleanup
failures.

## Terminology

- **Already stopped:** The requested instance was successfully torn down and
  removed by another stop operation before this operation completed.
- **Real cleanup failure:** HTTP-server or process-manager cleanup failed, so
  the instance or its port must remain available for retry.

## Requirements

### REQ-PLATFORM-AGENTCTL-INSTANCE-STOP-001: Idempotent agentctl instance stop

**Intent:** Prevent a teardown race from being reported as an internal failure
after the instance has already been stopped, without hiding an incomplete
cleanup operation.

#### Acceptance criteria

- **AC-PLATFORM-AGENTCTL-INSTANCE-STOP-001.1:** When a `DELETE /api/v1/instances/:id` request races with another stop for the same tracked instance and the other stop completes first, the request shall not return HTTP 500 solely because that instance is already stopped; the completed-stop outcome may be HTTP 200 or HTTP 404.
- **AC-PLATFORM-AGENTCTL-INSTANCE-STOP-001.2:** When a stop request observes an unknown instance that was not part of a completed stop, the control API shall retain its HTTP 404 response and shall not release an unrelated port or instance.
- **AC-PLATFORM-AGENTCTL-INSTANCE-STOP-001.3:** When HTTP-server or process-manager cleanup fails, the control API shall retain its HTTP 500 response and error-level diagnostic, and the instance and its allocated port shall remain retryable.
- **AC-PLATFORM-AGENTCTL-INSTANCE-STOP-001.4:** When cleanup succeeds, the instance shall be removed from tracking and its port shall be released at most once, including when duplicate stop calls overlap.

## Out of scope

- Changing the existing `ControlClient` treatment of HTTP 404 after a lost
  delete response.
- Changing process termination grace periods or agent protocol behavior.
- Adding persistence for stopped instances.
