---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-AGENTCTL-INSTANCE-STOP-001
created: 2026-08-26
owners:
  - kandev
---
# Agentctl instance stop idempotency System Design

## Purpose and boundaries

The platform agentctl instance manager owns the lifecycle of each per-agent
HTTP server, process manager, and allocated port. The control server exposes
that lifecycle through `DELETE /api/v1/instances/:id`. This design makes a
completed duplicate stop benign while preserving failure and retry semantics.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-AGENTCTL-INSTANCE-STOP-001` | [Stop flow](#stop-flow) and [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

- `internal/agentctl/server/api.ControlServer` validates the requested ID and
  maps the manager result to the existing HTTP response.
- `internal/agentctl/server/instance.Manager` snapshots the tracked instance,
  serializes teardown with the instance's `stopMu`, and owns map and port
  mutation.
- `instance.Instance` owns the per-instance stop lock and one-time port-release
  state.
- `internal/agent/runtime/agentctl.ControlClient` retains its existing rule that
  a 404 after a lost delete response satisfies the stopped postcondition.

## Data and contracts

The existing DELETE route and response bodies remain in use. An unknown ID is
still reported as HTTP 404. A stop that starts with a tracked instance and then
finds that the same instance has already been removed may complete as success;
an already-completed request that reaches the initial lookup after removal may
continue to receive HTTP 404. Neither completed-stop path is an HTTP 500.

If the ID is reused for a different tracked instance while an old stop finishes,
the manager must not treat that different pointer as the old completed stop.
That safety check prevents one lifecycle from mutating another lifecycle.

## Stop flow

1. The control handler performs its existing lookup and returns 404 for an ID
   that is absent before the stop begins.
2. `Manager.StopInstance` snapshots the instance pointer and serializes against
   other stops for that pointer with `stopMu`.
3. After acquiring the lock, the manager rechecks the ID mapping. If the mapping
   is absent because this same pointer was already torn down and removed, it
   returns the already-stopped success outcome. If a different pointer occupies
   the ID, it retains the safety error.
4. The first successful teardown closes admission, stops the HTTP server and
   process manager, releases the port once, and removes the pointer from the
   instance map.
5. The control handler returns the existing success response for a nil manager
   result. Real manager errors retain the existing failure response.

## Failure and recovery

Unknown instances remain non-mutating 404 responses. Completed duplicate stops
do not log an error or return 500. If HTTP-server or process-manager cleanup
fails, the manager retains the instance and allocated port for retry, and the
control handler keeps the error-level diagnostic and HTTP 500 response.

## Observability

The normal completed-stop path continues to log successful instance removal.
The already-stopped race path may use debug-level diagnostic context, but it
must not emit the current `failed to stop instance` error with a stack trace.
Real teardown failures keep their current error-level log and request status.
