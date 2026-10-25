---
created: 2026-09-09
status: done
requirements:
  - REQ-EXECUTORS-SSH-TRANSPORT-LIVENESS-001
  - REQ-EXECUTORS-SSH-TRANSPORT-LIVENESS-002
system_design:
  - ../../specs/executors/system-design/ssh-transport-liveness.md
legacy_specs: []
---

# Implementation plan: SSH session transport liveness

## Overview

A remote runner's SSH tunnel can die silently over a lossy link and stay dead for
hours: the local port forward keeps accepting connections, so agentctl's WebSocket
hangs waiting on a response that will never arrive instead of erroring. Nothing in
`apps/backend/internal/agent/runtime/lifecycle` probed the transport for liveness;
the keepalive plumbing had been deliberately removed pre-merge (PR #927) after
orphaned goroutines surfaced under e2e fault injection.

This plan restores a per-session SSH keepalive watchdog, owned and bounded by the
session lifecycle so it cannot leak the way the removed version did, and wires its
failure into transport teardown so a dead tunnel becomes a fast, non-retryable
error instead of an infinite hang.

## Scope

In scope: a `keepalive@openssh.com` probe loop per SSH session, liveness
classification (probe reply, probe error, or transport end), transport teardown on
loss (mark, close forward, close client, warn once), disposal-path ownership so a
watchdog cannot outlive its session, and the `GetRemoteStatus` reclassification of
a transport lost mid-probe.

Out of scope (see the requirements doc for the full list): reconnect/re-dial, task
or agent state transitions, an operator-facing config toggle, the two downstream
diagnosability follow-ups (empty-payload-reads-as-success; unbounded agent
backoff), restructuring the serial remote-status poll loop, hardening
`SSHPortForwarder.Close` itself, shortening remote-command timeouts, other
executors, application-level health/ping above the transport, and multi-hop
ProxyJump chains.

## Technical approach

Add `executor_ssh_keepalive.go`: a watchdog goroutine pair (probe + deadline timer)
started under the executor mutex alongside the session record (insert-if-absent),
using fixed package-level tuning values (probe interval, unresponsive threshold,
disposal timeouts) with no runtime feature flag. A `keepalive@openssh.com` reply of
any kind counts as liveness and resets the deadline; a probe error or transport EOF
declares the transport lost immediately. Idempotent teardown runs through
per-handle once-guards so a stop racing a declaration cannot double-dispose.

Extend `executor_ssh.go`'s disposal sequence to close the forward and client and
mark the session transport-lost exactly once, and extend `GetRemoteStatus` to read
the transport marker and client in one critical section so a status read racing a
declaration cannot return a stale `agentctl-down` instead of `disconnected`.

Existing patterns: `executor_ssh_connection.go` for the `golang.org/x/crypto/ssh`
client/session lifecycle this watchdog attaches to, and
`executor_ssh_stop_resume_test.go` / `ssh_fake_server_test.go` for the fake SSH
server harness used to simulate a silent transport.

## Tests

All tests live in `apps/backend/internal/agent/runtime/lifecycle`, TDD'd against
the fake SSH server harness (no new Playwright E2E: no `apps/web/` change, no
schema/migration, no event-bus subscriber).

| Criteria | Evidence |
| --- | --- |
| AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.1 through .4, .8 through .11 | `executor_ssh_keepalive_test.go`: watchdog start-under-mutex, probe/reply/error classification, deadline arithmetic, tuning-value production defaults |
| AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.5 through .7 | `executor_ssh_transport_teardown_test.go`: mark-then-close-forward-then-close-client sequence, single warning, post-teardown remote-status reclassification |
| AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.1 through .15 | `executor_ssh_keepalive_test.go` + `executor_ssh_stop_resume_test.go`: ownership, stop-vs-declare race precedence, once-guard composition, resume-metadata completeness, backstop timeout |
| Watchdog leak safety | `goleak_test.go`: no watchdog goroutine survives session disposal |

## End-to-end evidence

No new Playwright E2E. Confirmed across five spec re-checks (most recently Spec
Review round 3): the changed-file list is backend + specs only, with no frontend,
schema, or event-bus surface.

## Work orders

- [x] [Task 01: Detect and tear down dead SSH session transports](task-01-ssh-transport-watchdog.md)

Single work order; implemented sequentially, no subagents.

## Verification results

Implementation is complete across 5 Build rounds, 5 Testing rounds, and 4 Review
rounds (hard cap). Final receipts (branch head prior to this documentation
addition): `go build ./...` clean; full SSH/keepalive suite green under `-race`;
`golangci-lint` 0 new issues; `gofmt` / spec-lint / architecture-lint clean;
package coverage 79.0%. See PR #3581 for the full round-by-round history and CI
receipts.

## Risks

- The frozen spec accepts three residual internal-consistency gaps (F35/F36/F37)
  with recorded readings rather than spec amendments; see the requirements doc's
  cross-references for the exact boundary each reading draws.
- `resetTrackedManagedBrokerResume`'s two-critical-section shape is the frozen
  design's own mandated shape (read+check before preflight, removal after), not a
  branch-introduced bug; a concurrent stop can still interleave with an in-flight
  preflight and surface a generic error instead of `ErrSSHTransportLost`. Recorded
  as carried-forward, non-blocking test-rigor residual, not fixed by this plan.

## References

- [Requirements](../../specs/executors/requirements/ssh-transport-liveness.md)
- [System design](../../specs/executors/system-design/ssh-transport-liveness.md)
