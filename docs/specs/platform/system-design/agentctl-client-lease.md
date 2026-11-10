---
status: current
system: platform
requirements:
  - REQ-PLATFORM-AGENTCTL-CLIENT-LEASE-001
created: 2026-10-20
owners:
  - kandev
---

# Agentctl Client Lease Design

## Boundary and mapping

| Criteria | Implementation boundary |
| -------- | ----------------------- |
| .1, .3   | `lifecycle.AgentExecution.AgentctlURL` in `agent/runtime/lifecycle/types.go` |
| .2       | `lifecycle.Manager.restoreRecoveredFailedExecution` in `manager_interaction.go` |
| .4       | `lifecycle.StreamManager.replayRecoveredDelivery` and `fetchRecoveredReplayPage` in `durable_delivery_stream.go` |
| .5       | `lifecycle.Manager.cancelAgentExecution`, `escalateStuckCancel` and `settleCancelledSubmission` in `manager_interaction.go` |

## Lock order

`agentctlLifecycleMu` is the client lease: bounded client operations hold its
read side, and terminal stop holds its write side while closing and detaching
the client. Go read locks are not reentrant once a writer is waiting, so a
nested read lease blocks until the outer one is released.

Execution persistence (`buildRunningFromExecution`) runs under the execution
store lock and reads `AgentctlURL`. That read therefore never takes the lease:
it loads the atomic override first and otherwise reads the construction-time
client under `agentctlSourceMu.TryRLock`. The base URL is immutable per client,
so no lease is needed to keep it valid.

A replacement or detach holds the source write side, which would make
`TryRLock` fail for a persistence read running on another goroutine. Both
writers therefore publish the URL they are moving to or from with an atomic
before they take the lock: a replacement publishes the incoming client's URL and
a detach publishes the installed client URL before waiting, then publishes the
empty string after clearing both client lookup tiers under the source lock.
A read that cannot take the read side returns that published value instead of blanking
`executors_running.agentctl_url` and `last_seen_at` for a live execution; an
empty URL therefore only appears when no client is set. Persistence still does
not fall back to the prior row's URL for an empty read, because an empty URL is
the correct value when no client is set.

`restoreRecoveredFailedExecution` holds the lease only for `GetStatus` and
releases it before `markBootReadyFromFailed`, so the lease is never held across
the store lock.

Releasing the lease early opens one accepted race: a terminal stop can
take the write side and detach the client between `GetStatus` and
`markBootReadyFromFailed`. The execution is then marked ready without a
client, and the next prompt fails because the agentctl client is unavailable.
That prompt failure is accepted in place of the deadlock that holding the
lease across the store lock caused.

Two long-lived callers held the lease across lifecycle callbacks and waits, so
any nested acquire under them re-entered the lease. `retainPromptFailureRuntime`
(`retainedPromptFailureExecutionEligible`) acquires it while a replayed
completion event runs the lifecycle callback chain, and `escalateStuckCancel`
acquired it again under `cancelAgentExecution`'s lease. With a terminal stop
queued, the nested read blocks while the goroutine can hold the execution
store lock.

- Replay pins the client only inside `fetchRecoveredReplayPage` for the
  `ReplayDelivery` request and requires it to still be the replay's client.
  Ownership checks, projection, acknowledgement scheduling and the lifecycle
  callbacks run unpinned.
- `cancelAgentExecution` releases the lease right after the cancel request.
  `escalateStuckCancel` no longer accepts a client; it pins the current client
  inside `settleCancelledSubmission` for the settle request only, after the
  wait and the readiness persistence.

Accepted races: a terminal stop can close the client while replay processes a
page, so the page's client calls fail and the replay returns that error as it
would for any stop; the next recovery restarts from the persisted cursor. A
stop or replacement between the cancel request and the escalation settle makes
the settle use the current client, or skip it when none is set, instead of the
client that sent the cancel.

## Verification

Lifecycle unit tests hold a read lease, queue a writer, and require
`AgentctlURL` to return the client URL without waiting, plus the override and
empty-client cases. One test holds the source write side while a replacement
publishes its URL and requires the read to keep that URL, so the executors
upsert cannot blank a live endpoint. A workspace callback/detach regression
uses the real SQLite repository to verify that an installed endpoint survives a
pending detach and clears after detach finishes. A recovery-consumer test
checks lease availability before readiness persistence, with separate status
error, stopped-process, cancellation, and missing-client cases. No agent process
is started.

Further lifecycle tests assert, with the real replay and escalation code, that
the lease is free while the lifecycle callback runs, while the escalation
waits for the prompt to finish (a queued writer proceeds), and during the
readiness persistence reached from `cancelAgentExecution` and
`escalateStuckCancel`.
