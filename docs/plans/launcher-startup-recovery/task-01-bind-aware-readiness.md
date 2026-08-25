---
id: "01-bind-aware-readiness"
title: "Resolve and probe effective bind addresses"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-LAUNCHER-STARTUP-001
acceptance_criteria:
  - AC-LAUNCHER-STARTUP-001.1
  - AC-LAUNCHER-STARTUP-001.2
  - AC-LAUNCHER-STARTUP-001.3
  - AC-LAUNCHER-STARTUP-001.4
  - AC-LAUNCHER-STARTUP-001.5
  - AC-LAUNCHER-STARTUP-001.6
system_design:
  - ../../specs/launcher/system-design/startup-recovery.md
---

# Task 01: Resolve and Probe Effective Bind Addresses

## Summary

Derive health targets and the access URL from the effective backend binds.
Probe all targets with the existing launch token so a specific LAN bind works
without a loopback listener.

## In scope

- Add the immutable backend endpoint set and resolver.
- Map IPv4 and IPv6 wildcard binds to their loopback families while preserving
  the default `localhost` browser origin for IPv4.
- Probe multiple targets concurrently while preserving loopback-first target
  priority.
- Use the resolved access URL in `dev`, `start`, and `run` output and browser
  opening.

## Out of scope

- Human-readable failure summaries beyond the existing error text.
- Changes to port selection or backend listener behavior.
- Proxy-trust configuration.

## Acceptance

- A single specific LAN bind becomes ready without a loopback listener.
- Any owned healthy target makes a multi-bind launch ready.
- The printed or opened URL is reachable through the effective bind set.

## Verification

```bash
(cd apps/backend && go test ./internal/launcher -run 'Test(BackendEndpoint|WaitForHealth|RunManagedApp|RunDev)' -count=1 && go test ./internal/launcher -count=1)
```

## Files likely touched

- `apps/backend/internal/launcher/network.go`
- `apps/backend/internal/launcher/network_test.go`
- `apps/backend/internal/launcher/health.go`
- `apps/backend/internal/launcher/health_test.go`
- `apps/backend/internal/launcher/start.go`
- `apps/backend/internal/launcher/start_test.go`
- `apps/backend/internal/launcher/dev.go`
- `apps/backend/internal/launcher/dev_test.go`

## Dependencies

None.

## Risks

- Concurrent probes must end when the shared context ends.
- Desktop token handoff must remain unchanged.

## Parallelism

`sequential`

## Inputs

- `REQ-LAUNCHER-STARTUP-001` and the endpoint-resolution design.
- Existing `ResolvedBinds`, `waitForHealth`, and launcher-mode tests.

## Results

Implemented the endpoint set used by `dev`, `start`, and `run`. It derives
separate health targets and browser URLs from `ResolvedBinds()`, probes IPv4
wildcards through `127.0.0.1` while preserving the default `localhost` browser
origin, maps IPv6 wildcards to `[::1]`, prefers loopback access, deduplicates
targets, and probes concurrently while retaining target priority. A lower-
priority success waits for higher-priority probes to complete without the
launch token. The launcher preserves the existing health-token ownership
contract.

Verification passed:

- `go test ./internal/launcher -run 'Test(BackendEndpoint|WaitForHealth|RunManagedApp|RunDev)' -count=1`
- `go test ./internal/launcher -count=1`
- `go test -race ./internal/launcher -run 'TestWaitForHealth' -count=1`
