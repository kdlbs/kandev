---
id: "01-ssh-transport-watchdog"
title: "Detect and tear down dead SSH session transports"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-TRANSPORT-LIVENESS-001
  - REQ-EXECUTORS-SSH-TRANSPORT-LIVENESS-002
acceptance_criteria:
  - AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.1
  - AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.2
  - AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.3
  - AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.4
  - AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.5
  - AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.6
  - AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.7
  - AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.8
  - AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.9
  - AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.10
  - AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.11
  - AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.1
  - AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.2
  - AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.3
  - AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.4
  - AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.5
  - AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.6
  - AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.7
  - AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.8
  - AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.9
  - AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.10
  - AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.11
  - AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.12
  - AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.13
  - AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.14
  - AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.15
system_design:
  - ../../specs/executors/system-design/ssh-transport-liveness.md
---

# Task 01: Detect and tear down dead SSH session transports

## Summary

Restore a `keepalive@openssh.com` liveness watchdog on the per-session SSH client,
owned and torn down by the session lifecycle so it cannot leak the orphaned
goroutines that got the original plumbing removed in PR #927. Wire watchdog
failure into transport teardown so a dead tunnel closes the client, which makes
`ChannelBackendClient.RequestPayload`'s blocked wait fail fast with a clean,
non-retryable error instead of hanging indefinitely.

## In scope

- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_keepalive.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_connection.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_scripts.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_startup.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_docker.go` and
  `executor_sprites.go` (shared disposal helper adjustments only)
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_keepalive_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_transport_teardown_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_stop_resume_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_lifecycle_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/goleak_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/ssh_fake_server_test.go`

## Out of scope

Reconnect/re-dial, task or agent state transitions, an operator config/runtime
toggle, the two downstream diagnosability follow-ups filed separately
(empty-payload-reads-as-success; unbounded agent backoff), restructuring the
serial remote-status poll loop, hardening `SSHPortForwarder.Close` itself,
shortening remote-command timeouts, other executors, application-level
health/ping above the transport, and multi-hop ProxyJump chains.

## Acceptance

- A session's watchdog starts in the same critical section as its record
  (insert-if-absent) and probes every 15s; any reply resets the deadline.
- 45s of silence, a probe error, or transport end declares the transport lost and
  runs the six-step disposal sequence: mark, skip/abandon remote commands per the
  unresponsive condition, close the forward, close the client, warn once, join.
- A stop request wins a three-way race against a declaration and a reset; teardown
  is idempotent under two per-handle once-guards; the record stays insert-if-absent
  and is not removed by teardown itself.
- `GetRemoteStatus` reads the transport marker and client in one critical section
  so a probe-time loss is reported as `disconnected`, not `agentctl-down`.
- A lost create-vs-resume race adopts the winner's full resume metadata, including
  runtime API tunnel fields.
- No watchdog goroutine survives session disposal (`goleak`).

## Verification

Run from `apps/backend`:

```bash
go build ./...
go test ./internal/agent/runtime/lifecycle/... -run 'TestSSH' -race -count=1
golangci-lint run ./internal/agent/runtime/lifecycle/...
gofmt -l internal/agent/runtime/lifecycle
```

## Files likely touched

See "In scope" above.

## Dependencies

None.

## Risks

The link this watchdog defends against is lossy (12.5% packet loss observed in
production); tuning values are fixed, not configurable, per the frozen design.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/ssh-transport-liveness.md)
- [System design](../../specs/executors/system-design/ssh-transport-liveness.md)
- [Plan](plan.md)
- PR #927 (prior keepalive removal, cited for its e2e fault-injection rationale)

## Results

Implemented the watchdog, teardown sequence, and ownership guarantees across 5
Build rounds, 5 Testing rounds, and 4 Review rounds (hard cap). Two incidental
bugs were fixed during TDD: the prober's replies/error channel split, and an
upstream `x/crypto/ssh` v0.52.0 deadlock (bumped to v0.57.0). Production bugs
found and fixed during Build/Review: `buildInstanceForLostRace` dropped, then
still didn't fully adopt, the resume winner's tunnel metadata (fixed across
rounds 3-4, mirroring `buildResumedInstance`); `runLoop`'s stop-beats-both
precedence wasn't Go-select-guaranteed (fixed with `stopRequested()`
reverification before every declare/handle site); a flaky Testing-round-4 test
raced a classification threshold against a fixed sleep (fixed with a
lock-guarded poll).

Verification (branch head prior to this documentation addition):

- `go build ./...`: clean.
- `go test ./internal/agent/runtime/lifecycle/... -race`: full SSH/keepalive
  suite green; package coverage 79.0%.
- `golangci-lint run`: 0 new issues.
- `gofmt -l`, spec-lint, architecture-lint: clean.

See PR #3581 for full round-by-round receipts, the rebase reconciliation with the
concurrent SSH Runtime API Tunnel feature, and CI history.
