---
status: draft
system: platform
created: 2026-08-26
updated: 2026-09-27
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

- **AC-PLATFORM-AGENTCTL-INSTANCE-STOP-001.5:** A port release shall name the immutable lease owner and allocation generation. A stale cleanup request shall leave a successor lease for the same numeric port intact.
- **AC-PLATFORM-AGENTCTL-INSTANCE-STOP-001.6:** Repeated allocation for an owner with a reservation shall return that reservation without consuming additional capacity. A later reservation shall have a different generation, including for the same owner.
- **AC-PLATFORM-AGENTCTL-INSTANCE-STOP-001.7:** Marking a reservation unavailable shall require its matching owner and generation. Absent or stale reservations shall not change current capacity or successor ownership.
- **AC-PLATFORM-AGENTCTL-INSTANCE-STOP-001.8:** If creation ends before registration, Kandev shall close the provisional listener and stop its process and tracker resources. Cleanup shall release only the matching reservation after successful teardown. Failed cleanup shall retain the resources and reservation for retry. Shutdown shall wait for asynchronous cleanup and report unresolved teardown errors.
- **AC-PLATFORM-AGENTCTL-INSTANCE-STOP-001.9:** Instance information shall expose reservation generation and listener activity through the control API and client. Missing diagnostics from older servers shall remain compatible and shall not authorize reclamation.
- **AC-PLATFORM-AGENTCTL-INSTANCE-STOP-001.10:** Duplicate stored ports, incomplete probes, and failed inventory reads shall not authorize reservation release. This protection applies to every live non-terminal instance and every instance serving a `WAITING_FOR_INPUT` session. Mixed inventories shall preserve these instances even when another record is terminal or stale.

## Related delivery

The [owned port lease package](../../../plans/agentctl-owned-port-leases/plan.md)
extends the existing stop contract through provisional creation and diagnostic transport.
The earlier [stop package](../../../plans/startup-observability-cleanup/task-03-instance-stop.md)
remains complete for criteria .1 through .4.

## Out of scope

- Changing the existing `ControlClient` treatment of HTTP 404 after a lost
  delete response.
- Changing process termination grace periods or agent protocol behavior.
- Adding persistence for stopped instances.

- Increasing the default 41001–41100 instance range or changing parked-session retention.
- Reclaiming instances from database port values or introducing a recovery scanner.
- Changing Docker, SSH, Kubernetes, Sprites, or UI lifecycles.
- Worktree recovery, commit recovery, and runtime-exit signaling tracked by issue #3962.
