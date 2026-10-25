---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-AGENTCTL-INSTANCE-STOP-001
created: 2026-08-26
updated: 2026-09-27
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
  state, including its immutable allocator lease.
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
   process manager, releases the matching lease once, and removes the pointer from the
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

## Owned port reservations

Criteria .5 through .10 extend this platform lifecycle contract. Executor port
probing and control-server ownership remain separate contracts.

`PortLease` contains `Port`, `Owner`, and `Generation`. `PortAllocator` keeps
port-to-lease and owner-to-lease indexes under its existing mutex. Each new
reservation receives a nonzero generation that increases within that allocator.
Generation exhaustion must fail allocation rather than wrap to a reused identity.
Generations have meaning only within the owning allocator process.

`Allocate(owner)` returns the existing lease when that owner already has one.
`Release(lease)` and `MarkUnavailable(lease)` compare the full lease under the
same mutex. Matching mutations remove both indexes. An absent lease is a no-op.
An owner or generation mismatch preserves both indexes and the unavailable set.
Only a matching mark-unavailable mutation blocks the numeric port.

`Instance` stores the lease privately. Numeric `Port` fields remain compatible
with configuration and response payloads. They cannot authorize release.
Registered teardown keeps its pointer comparison, `stopMu`, and one-time release
guard. Database rows never supply allocator authority.

This extends the existing instance-pointer fence to allocator state. A port-only
key cannot distinguish reuse. An owner-only key cannot distinguish successive
reservations for the same owner. The generation closes both gaps without storage
changes or a cross-process lease protocol.

## Provisional creation and failed cleanup

The manager owns one provisional bundle containing the lease, listener, and
process manager. The bundle exists from successful bind until registration.
It is private to the instance manager and never appears as a runnable instance.

1. Under `m.mu`, reject a create ID already registered or held by a provisional
   bundle. Allocator idempotency does not authorize a second bind for that ID.
2. After bind, record the bundle and register its cleanup wait-group ownership.
   Arm one deferred cleanup path for every return before registration.
3. Add the process manager to the bundle before starting comparison preparation
   or workspace trackers. Preserve both caller-context checks.
4. On registration, transfer ownership to `Instance`, remove the provisional
   entry, and disarm cleanup. Balance the wait-group count exactly once.
5. On abandonment, close process admission and close the listener. Drain process
   and tracker resources with a fresh cleanup context, outside `m.mu`.
6. Release the matching lease only after listener closure and teardown succeed.
   Treat an already-closed listener as success. Retain any other closure error.
7. On failure, retain the bundle, error, and lease in a private pending-cleanup
   map. Reject the same owner while this entry remains. Do not return capacity.

Each bundle serializes its own cleanup attempts. Successful listener closure is
remembered so a retry does not depend on repeated `Close` behavior.
`Shutdown` first closes creation admission and drains `abandonWG`. It then retries
retained bundles once with the shutdown context, outside `m.mu`. Failed retries
remain available to a later `Shutdown` call and contribute to its returned error.
This change adds no automatic reclamation loop or task-layer event.

Bind failures before a listener exists still release or mark unavailable only
the matching lease. Existing address-in-use classification and range limits stay
in effect. Unknown state never counts as successful cleanup.

## Diagnostic transport

Add `lease_generation` and `listener_active` to server and control-client
`InstanceInfo`. The instance ID supplies the owner. Instance list and detail
responses share these fields through `Instance.Info()`.

Store listener activity atomically. Mark it active before serving and clear it
when `Serve` returns. Listener inactivity does not prove process teardown or
permit lease release. Process-manager cleanup can still be pending.

Older servers omit the fields. Generation zero means unavailable lease metadata.
A false activity value with missing metadata cannot prove an inactive listener.
Existing recovery keeps using its authoritative runtime identity checks. These
fields add evidence but do not introduce a reclamation decision.

The fields use existing authenticated control routes. This package adds no pool
metrics endpoint, public setting, schema migration, or browser surface.

## Implementation plans

- [Owned port leases](../../../plans/agentctl-owned-port-leases/plan.md): criteria
  .5 through .10 and regression protection for .1 through .4.
- [Existing stop idempotency](../../../plans/startup-observability-cleanup/task-03-instance-stop.md):
  completed delivery of criteria .1 through .4.
