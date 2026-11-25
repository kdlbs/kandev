---
id: "02-replay-lease"
title: "Bound recovered delivery replay leases"
status: done
wave: 2
depends_on:
  - "01-agentctl-url-lease"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-AGENTCTL-CLIENT-LEASE-001
acceptance_criteria:
  - AC-PLATFORM-AGENTCTL-CLIENT-LEASE-001.4
system_design:
  - ../../specs/platform/system-design/agentctl-client-lease.md
---

# Task 02: Bound recovered delivery replay leases

## Summary

Release the agentctl client read lease before SQL operations and lifecycle
callbacks during recovered delivery replay. A callback can then take its own
lease without a nested read.

## In scope

- `recoveredReplayClient`: take a short lease for page reads, delivery status
  reads, and synchronous acknowledgements. Reject a different or closed client.
- Replay event preparation: pass the scoped client to the duplicate and expired
  receipt paths. Keep the SQL durability barrier before acknowledgement.
- Regression coverage in `agentctl_replay_lease_test.go`.

## Verification

- `go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run '^TestRecoveredDeliveryReplay' -count=1`
  (isolated SQLite and HTTP peers).
