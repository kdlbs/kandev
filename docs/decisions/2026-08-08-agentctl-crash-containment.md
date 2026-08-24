# ADR-2026-08-08-agentctl-crash-containment: Publish Immutable Agent Events and Explicit Runtime Failure State

**Status:** proposed
**Date:** 2026-08-08
**Area:** backend, frontend, protocol, operations

## Context

The standalone `agentctl` process crashed with Go's fatal `concurrent map
iteration and map write` while `NormalizedPayload.MarshalJSON` serialized a tool
update. ACP adapter state is mutated under `Adapter.mu`, but emitted events can
retain pointers to that state after the lock is released. The existing
`cloneSubagentPayload` helper snapshots subagent payloads only; generic
`Input`, `Output`, and miscellaneous `Details` maps can still be updated while
the HTTP stream writer marshals an earlier event.

The backend survives an `agentctl` child exit. Its HTTP server and browser
WebSocket therefore remain reachable even though the local agent runtime has
stopped. Browser connectivity alone cannot identify the outage, and the current
UI gives no durable feedback when task and workspace activity becomes stale.

Restarting only the child is not a local launcher concern. The per-launch auth
token, PID, control clients, lifecycle manager, host utility manager, and active
execution metadata are captured by multiple owners during startup. Replacing
the process without atomically rebinding those owners would create a nominally
healthy but internally split runtime.

## Decision

Kandev contains this failure at two boundaries:

1. `NormalizedPayload` gains a complete immutable snapshot operation. Every ACP
   event snapshots its payload while `Adapter.mu` is held and before the event
   crosses the updates-channel boundary. The snapshot copies all typed pointer
   and slice fields and recursively copies JSON-compatible values held in
   `GenericPayload.Input`, `GenericPayload.Output`, and `MiscPayload.Details`.
   The one-off subagent clone is replaced by this common ownership rule.
2. The launcher reports unexpected child exit through a callback after it has
   distinguished the event from intentional shutdown. A concurrency-safe,
   install-wide availability owner records an `unavailable` snapshot and
   publishes a stable `agentctl_exited` reason. Raw process details remain in
   logs.
3. The current availability snapshot is distributed through the boot/app-state
   payload, a global system WebSocket notification, and replay after user
   subscription. The event is install-wide and intentionally uses the gateway's
   global broadcast path.
4. The application shell renders a persistent, non-dismissible runtime alert
   independent of the optional App status bar. It preserves last-known data and
   reuses the existing supervisor capability and restart flow when available;
   unsupported launch modes receive manual guidance.

Availability is in memory and monotonic within one backend boot. Recovery is a
full supervised Kandev restart, which creates a fresh backend and authenticated
child together. The general backend health endpoint remains unchanged.

The observable behavior is specified in
[`docs/specs/platform/requirements/agent-runtime-availability.md`](../specs/platform/requirements/agent-runtime-availability.md).

## Consequences

- Stream consumers receive values they own, so later ACP updates cannot race
  their serialization or retroactively change an earlier event.
- An unexpected child exit becomes visible even when browser-to-backend
  connectivity remains healthy, and late clients converge on the same state.
- The first repair avoids a partial child restart that would leave stale
  credentials and process identity in downstream services.
- The unavailable alert can coexist with the WebSocket connectivity warning
  because they report separate backend-child and browser-backend boundaries.
- Snapshotting adds bounded copy work for tool events. Payload size is already
  bounded for stream presentation, and correctness takes precedence at this
  asynchronous ownership boundary.

## Alternatives Considered

### Recover around JSON marshaling

Rejected. Go's concurrent-map runtime failure is fatal and cannot be reliably
recovered. The mutable alias must not reach the serializer.

### Copy only generic maps in `MarshalJSON`

Rejected. Marshaling occurs after adapter ownership has been released, so a
copy attempted there can race the same mutation. It also leaves earlier queued
events mutable and misses typed slices and nested values.

### Keep the subagent-only clone and patch the observed field

Rejected. The bug is an ownership violation shared by every mutable normalized
payload, not a one-field exception. Another payload kind could recreate the
same crash or event-history corruption.

### Restart `agentctl` in process

Deferred. It requires one coordinated owner for child generation, auth token,
PID, clients, lifecycle state, host utility state, and active execution
reconciliation. A future design may supersede this decision once that rebinding
contract exists.

### Exit the backend when the child exits

Rejected for this repair. An arbitrary backend exit is not guaranteed to invoke
the explicit supervisor restart protocol and can terminate a normal CLI launch.
Keeping the backend available preserves saved data, diagnostics, and the
supported user-initiated restart endpoint.

### Rely on the existing connectivity warning, toast, or App status bar

Rejected. The browser WebSocket can remain connected, toasts are transient, and
the status bar is optional. A core-runtime outage needs persistent shell-level
feedback regardless of route state or feature flags.
