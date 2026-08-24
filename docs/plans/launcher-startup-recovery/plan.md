---
created: 2026-08-24
status: done
requirements:
  - REQ-LAUNCHER-STARTUP-001
  - REQ-LAUNCHER-STARTUP-002
  - REQ-LAUNCHER-STARTUP-003
system_design:
  - ../../specs/launcher/system-design/startup-recovery.md
legacy_specs:
  - ../../specs/platform/startup-configuration-parity.md
  - ../../specs/auth/trusted-proxies.md
---

# Implementation Plan: Launcher Startup Recovery

## Overview

Make backend readiness use the effective bind addresses. Then add typed probe
evidence and concise failure output. Update the public CLI and configuration
guides after the runtime behavior is stable.

## Scope

### In scope

- Bind-aware health targets and a reachable access URL.
- Multi-target readiness with the existing health token.
- Typed readiness failures with configuration and probe evidence.
- Clear output that distinguishes a child exit from launcher-initiated
  shutdown.
- Reverse-proxy CIDR and startup troubleshooting documentation.

### Out of scope

- Automatic private-network trust.
- A new configuration key, CLI flag, or Settings page.
- Changes to `/health` or ACP probing.
- General parsing or ranking of backend log messages.

## Technical approach

### Endpoint resolution and readiness

Add a launcher-owned endpoint resolver beside `network.go`. It consumes
`ServerConfig.ResolvedBinds()` and the selected backend port. It returns an
ordered probe-target set, a corresponding browser-URL set, and the preferred
access URL. An IPv4 wildcard is probed through `127.0.0.1` while its default
browser URL remains `localhost`. Change `waitForHealth` to probe all targets
concurrently while selecting the highest-priority healthy target. Thread the
endpoint set through `runManagedApp` and `runDev` without changing port
selection.

### Failure evidence and presentation

Replace string-only readiness errors with a typed error that retains bounded
per-target observations and child state. Format one summary at the launcher
boundary. Include configuration provenance from `config.Config.Source`, the
expected backend log path, and a class-specific next action. Keep the existing
bounded backend-output dump.

### Public guidance

Update the CLI health-timeout section with the new output and the distinction
between readiness, ACP closure messages, and trusted-proxy warnings. Update the
configuration guide with exact-IP and CIDR proxy examples. State that the
trusted range must contain controlled proxies, not browser clients.

## Tests

- `internal/launcher/network_test.go` covers IPv4, IPv6, wildcard, loopback,
  specific-address, multi-bind, and access-URL resolution.
- `internal/launcher/health_test.go` covers concurrent multi-target success,
  target-priority selection, target observations, token mismatch, child exit,
  cancellation, and timeout.
- `internal/launcher/start_test.go` and `dev_test.go` cover shared endpoint
  wiring and failure presentation.
- Public documentation validation covers links, frontmatter, and navigation.

## Work orders

- [x] [Task 01: Resolve and probe effective bind addresses](task-01-bind-aware-readiness.md)
- [x] [Task 02: Present actionable startup failures](task-02-actionable-startup-failures.md)
- [x] [Task 03: Document startup and proxy recovery](task-03-startup-proxy-guidance.md)

## Verification results

- Task 01 targeted launcher tests passed: `go test ./internal/launcher -run 'Test(BackendEndpoint|WaitForHealth|RunManagedApp|RunDev)' -count=1`.
- Task 02 targeted launcher tests passed: `go test ./internal/launcher -run 'Test(WaitForHealth|StartupFailure|RunManagedApp|RunDev)' -count=1`; concurrent readiness coverage also passed under `go test -race ./internal/launcher -run 'TestWaitForHealth' -count=1`.
- The full launcher package passed: `go test ./internal/launcher -count=1`.
- Public documentation validation passed: `node --test scripts/validate-public-docs.test.mjs`, `node scripts/validate-public-docs.mjs`, and `python3 scripts/lint-spec-files.py --all`.

The launcher test commands were run with the task runner's inherited
`KANDEV_INTERNAL_CONFIG_FILE` and `KANDEV_INTERNAL_CONFIG_HOME_FILE` unset so
service configuration tests could select their temporary fixtures.

## Risks

- Hostnames and IPv6 addresses must use correct URL and socket formatting.
- Multi-target probes must not leak goroutines after cancellation.
- Desktop-owned health tokens and supervisor restarts must retain their current
  ownership contracts.
- A broad trusted-proxy example can weaken header trust if the operator exposes
  the backend directly.
