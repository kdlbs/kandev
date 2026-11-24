---
id: "01-agentctl-url-lease"
title: "Keep agentctl URL reads off the client lease"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-AGENTCTL-CLIENT-LEASE-001
acceptance_criteria:
  - AC-PLATFORM-AGENTCTL-CLIENT-LEASE-001.1
  - AC-PLATFORM-AGENTCTL-CLIENT-LEASE-001.2
  - AC-PLATFORM-AGENTCTL-CLIENT-LEASE-001.3
system_design:
  - ../../specs/platform/system-design/agentctl-client-lease.md
---

# Task 01: Keep agentctl URL reads off the client lease

## Summary

Make `AgentExecution.AgentctlURL` a non-blocking snapshot that never takes the
agentctl client lease, and release the lease in
`restoreRecoveredFailedExecution` before the execution is marked ready.

## In scope

- `AgentctlURL`: nil-safe; atomic override first, then the source client under
  `TryRLock`. A read that cannot take the read side returns the URL that the
  replacement or detach published before taking the write side, so an empty URL
  means no client is set. Detach publishes the installed URL before waiting
  for callbacks and publishes empty only after clearing the client.
- `restoreRecoveredFailedExecution`: release the lease after `GetStatus` and
  on the nil-client path, not by `defer`.
- Regression coverage in `agentctl_url_persistence_test.go` for the persisted
  URL during and after a callback-delayed detach, and
  `agentctl_recovery_lease_test.go` for release before readiness persistence
  and release on unsuccessful status verification.

## Out of scope

Lease handling in other call sites and the terminal stop path. Client
replacement and detach keep their lease semantics; they only publish their URL
before taking the source write side.

## Verification

- `go test -trimpath ./internal/agent/runtime/lifecycle/ -run '^(TestAgentctlURL|TestRecoverAgentPromptStreamReleasesLeaseBeforePersistence|TestRestoreRecoveredFailedExecutionReleasesLeaseOnErrors)' -count=1`
  (unit and isolated SQLite/HTTP consumer checks; no agent process is started)
