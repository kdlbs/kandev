---
id: "02-replay-cancel-lease"
title: "Bound the replay and cancel-escalation leases"
status: done
wave: 2
depends_on:
  - "01-agentctl-url-lease"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-AGENTCTL-CLIENT-LEASE-001
acceptance_criteria:
  - AC-PLATFORM-AGENTCTL-CLIENT-LEASE-001.4
  - AC-PLATFORM-AGENTCTL-CLIENT-LEASE-001.5
system_design:
  - ../../specs/platform/system-design/agentctl-client-lease.md
---

# Task 02: Bound the replay and cancel-escalation leases

## Summary

Stop two callers from holding the agentctl client read lease across lifecycle
callbacks, waits and persistence, where a nested acquire deadlocks behind a
queued terminal stop.

## In scope

- `replayRecoveredDelivery`: no lease across the page loop; each page fetch
  pins the client through `fetchRecoveredReplayPage` and fails if the client
  changed.
- `cancelAgentExecution`: release the lease after the cancel request.
- `escalateStuckCancel`: drop the client parameter and the lease held for the
  whole call; pin the client in `settleCancelledSubmission` for the settle
  request only.
- Regression coverage in `agentctl_replay_lease_test.go`.

## Out of scope

Other lease users, the terminal stop path, and the client replacement and
detach semantics.

## Verification

- `go test -trimpath ./internal/agent/runtime/lifecycle/ -run '^(TestRecoveredDeliveryReplayReleasesLeaseBeforeLifecycleCallbacks|TestEscalateStuckCancelReleases|TestCancelAgentExecutionReleasesLeaseBeforeEscalation)' -count=1`
  (isolated SQLite/HTTP peers; no agent process is started)
