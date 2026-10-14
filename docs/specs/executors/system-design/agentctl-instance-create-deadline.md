---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-AGENTCTL-CREATE-DEADLINE-001
  - REQ-EXECUTORS-AGENTCTL-CREATE-DEADLINE-002
---

# Agentctl instance creation deadline and concurrency

## Purpose and boundaries

`internal/agent/runtime/agentctl.ControlClient` is the backend client for the
agentctl control API. Docker, standalone, Kubernetes, plugin-provided, and host
utility runtimes all create instances through `ControlClient.CreateInstance`.
On the controller, `POST /api/v1/instances` calls
`internal/agentctl/server/instance.Manager.CreateInstance`.

This design selects the client deadline for creation and moves the
controller's creation startup out of the manager lock. It does not change the
control API, the request and response shapes, any caller's launch context, or
the work a single creation performs.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-EXECUTORS-AGENTCTL-CREATE-DEADLINE-001 | Deadline selection, Cancellation and errors, Verification |
| REQ-EXECUTORS-AGENTCTL-CREATE-DEADLINE-002 | Creation phases, Shutdown, Lock scope for other operations, Verification |

## Current behavior

`NewControlClient` builds one `http.Client` with a 30-second `Timeout`. Every
control method, including `CreateInstance`, sends its request through that
client.

`Manager.CreateInstance` holds `Manager.mu` for the whole call: admission,
process manager construction, comparison-target preparation, workspace tracker
startup, HTTP server start, and publication. Concurrent creations therefore
queue behind one another's startup. `GetInstance`, `ListInstances`, and
`StopInstance` also wait on the same lock.

Callers bound creation with longer contexts. The lifecycle launch phase uses
`constants.AgentLaunchTimeout`, which includes the setup-script allowance. The
30-second client timeout was therefore the effective bound for creation.

## Deadline selection

`CreateInstance` sends its request through a per-call copy of the client's
`http.Client` whose `Timeout` is `createInstanceHTTPTimeout`, 120 seconds.

The copy shares the configured `Transport`. The bearer credential
(`authTransport`) and the runtime binding guard (`runtimeBindingTransport`)
therefore apply exactly as they do for other control calls. The copy is taken
under the client's `mu`, the lock `SetAuthToken` holds when it installs a new
transport, so a copy made per call observes the transport installed at that
time without racing credential rotation.

The client's shared `http.Client` keeps its 30-second `Timeout`. All other
control methods continue to use it. A fixed constant is sufficient. No
configuration key, option, or environment variable is introduced.

## Cancellation and errors

The request keeps the caller's context through `http.NewRequestWithContext`.
The effective bound is the earlier of the caller's deadline or cancellation and
the 120-second client timeout.

Transport and timeout failures keep the existing `failed to create instance`
error wrapping. Non-success responses, response decoding, and logging are
unchanged.

## Creation phases

`Manager.CreateInstance` validates the requested protocol, then runs in three
phases.

1. **Admission, under `mu`.** Refuse during shutdown or for a caller whose
   context is already done, collect and validate the agent environment
   (`config.CollectAgentEnvWithError`), reject an ID that is published or
   provisional, reserve a port and listener, and register the
   `provisionalInstance`. Admission increments `activeCreations` while it still
   holds `mu`. An invalid environment therefore fails before any port is
   reserved.
2. **Startup, without `mu`.** Build the instance configuration and
   `process.Manager`, prepare comparison targets, start the workspace trackers,
   re-check the caller's context, and build the HTTP handler. The provisional
   bundle keeps the ID and port exclusive during this phase.
3. **Publication, under `mu`.** Refuse if shutdown has begun or if the caller's
   context is done; the caller can give up while publication waits for `mu`.
   Otherwise start the HTTP server on the reserved listener and move the
   instance from `provisional` to `instances`. `CreateReadyMillis` is recorded
   after publication.

The HTTP server starts inside the publication critical section. A published
instance therefore always has a server, and a refused or abandoned creation
never starts one. Starting the server only launches its serve goroutine, so `mu` is held
briefly.

Any failure or refusal after admission keeps the existing abandonment path. A
deferred step schedules `abandonPartialInstance` on `abandonWG`, then marks the
creation finished on `activeCreations`. Cleanup closes the listener, stops the
process manager, and releases the lease, or retains the bundle for retry. The
process manager is stored on the bundle under `cleanupMu`, the lock cleanup
already uses.

`createStart` covers all three phases, so the existing
`agentctl_create_ready_ms` diagnostic still reports the full creation time.

## Shutdown

`Shutdown` sets `shuttingDown` under `mu`, then waits on `activeCreations`
before it drains `abandonWG` and snapshots `instances` and `provisional`.

Admission adds to `activeCreations` under `mu` after checking `shuttingDown`.
Every admitted creation is therefore visible to that wait, and no creation is
admitted afterward. An admitted creation that reaches publication after
shutdown began is refused and abandoned. It registers its `abandonWG` entry
before it leaves `activeCreations`, so the following `abandonWG.Wait` covers
it.

The previous contract still holds: after `Shutdown` returns, no instance from
an in-progress creation is published or holds its port, unless cleanup failed
and the bundle was retried and reported.

## Lock scope for other operations

`GetInstance`, `ListInstances`, `StopInstance`, and the idle reaper take `mu`
only for map access. They no longer wait for another creation's startup.

An instance in startup is not published, so lookup and stop report it as not
found, as they do for an unknown ID. Its caller owns it until creation returns.

## Known limits

A longer client deadline and overlapping startup let slow but progressing
creations complete. They do not make a single startup faster, and a creation
that exceeds 120 seconds still fails. These changes do not address other causes
of a stalled session start.

## Verification

Client tests stay alongside `ControlClient` in
`internal/agent/runtime/agentctl`:

- A creation request observes a deadline close to 120 seconds through a
  stubbed transport (`AC-EXECUTORS-AGENTCTL-CREATE-DEADLINE-001.1`).
- The constructor test and the creation-deadline test keep the shared
  30-second timeout (`AC-EXECUTORS-AGENTCTL-CREATE-DEADLINE-001.2`).
- A shorter caller deadline fails creation with a wrapped
  `context.DeadlineExceeded` (`AC-EXECUTORS-AGENTCTL-CREATE-DEADLINE-001.3`).
- A token-authenticated creation request carries the bearer credential, and a
  race-enabled run with concurrent credential rotation reports no race on the
  client copy (`AC-EXECUTORS-AGENTCTL-CREATE-DEADLINE-001.4`).

Manager tests stay in `internal/agentctl/server/instance`. They use the
`afterTrackerStart` seam to hold creations at the end of startup:

- Several creations reach the end of startup at the same time
  (`AC-EXECUTORS-AGENTCTL-CREATE-DEADLINE-002.1`).
- A same-ID creation is rejected while the first is held in startup
  (`AC-EXECUTORS-AGENTCTL-CREATE-DEADLINE-002.2`).
- Existing abandonment tests cover a caller that gives up before or during
  startup, failed cleanup, and retry. A caller cancelled while publication
  waits for `mu` leaves no published instance or reserved port
  (`AC-EXECUTORS-AGENTCTL-CREATE-DEADLINE-002.3`).
- Shutdown started while a creation is held does not return until the creation
  ends, and leaves no published instance or reserved port
  (`AC-EXECUTORS-AGENTCTL-CREATE-DEADLINE-002.4`).
- Lookup, listing, and stop of another instance complete while a creation is
  held in startup (`AC-EXECUTORS-AGENTCTL-CREATE-DEADLINE-002.5`).
