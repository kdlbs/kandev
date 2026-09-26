---
spec: "../../specs/platform/requirements/go-dev-launcher.md"
created: 2026-08-13
status: implemented
---

# Implementation Plan: Restore dev network address logging

## Overview

Restore the startup-banner behavior that the Node launcher provided before the
`make dev` migration. Add a small Go network-address helper, feed its filtered
addresses into the existing shared `logStartup` path, and cover formatting,
filtering, and enumeration failures with focused launcher tests. This keeps the
behavior shared by `dev`, `start`, and `run` without exposing the internal Vite or
agentctl ports.

---

## Backend

### Host address discovery and startup banner

- Add `apps/backend/internal/launcher/network.go` with best-effort enumeration of
  active, non-loopback host addresses, deduplication, link-local filtering,
  IPv4-first ordering, IPv6 URL formatting, and filtering against an explicit
  bind host.
- Update `apps/backend/internal/launcher/start.go` so `logStartup` prints the
  filtered network URLs for `portConfig.BackendPort` between the localhost URL and
  the existing MCP/DB/log-level lines.
- Keep interface-enumeration errors non-fatal and leave the existing localhost
  startup path unchanged.

---

## Tests

- **What:** LAN and Tailscale-style IPv4 addresses are listed, duplicates and
  loopback/link-local addresses are omitted, IPv6 URLs are bracketed, and IPv4 is
  listed before IPv6.
  **File:** `apps/backend/internal/launcher/network_test.go`.
  **How:** Deterministic table-driven unit tests over representative interface
  address fixtures.
- **What:** The shared startup banner emits the network URLs and still emits the
  localhost URL when interface enumeration fails.
  **File:** `apps/backend/internal/launcher/network_test.go`.
  **How:** Assert the generated startup output with injected address-discovery
  results and an enumeration-error case.
- **What:** Down interfaces are not advertised, and explicit bind hosts only
  advertise reachable interface addresses.
  **File:** `apps/backend/internal/launcher/network_test.go`.
  **How:** Use active/down interface fixtures and loopback, wildcard, and
  specific-bind cases.

## Documentation

- Update `docs/public/cli.md` so the public CLI reference describes the optional
  `network:` lines and keeps the network-exposure warning clear. This page is a
  reference document.

## Verification Results

- `cd apps/backend && go test -run TestLogStartupPrintsNetworkAddress ./internal/launcher` —
  failed before the fix with the expected missing `network:` assertion.
- `cd apps/backend && go test -run 'Test(ListHostNetworkAddresses|NetworkURLsForPort|LogStartup)' ./internal/launcher` —
  passed, 4 tests.
- `cd apps/backend && go test ./internal/launcher` — passed, 196 tests.
- `cd apps/backend && go test -run TestLogStartupSuppressesNetworkAddressesForLoopbackBind ./internal/launcher` —
  failed before the fix, then passed after bind-host filtering was added.
- `cd apps/backend && go test -run 'Test(NetworkAddressesForBindHost|LogStartupSuppressesNetworkAddressesForLoopbackBind)' ./internal/launcher` — passed.
- `cd apps/backend && go test -run TestListHostNetworkAddressesSkipsDownInterfaces ./internal/launcher` — passed.
- `rtk git diff --check` — passed.
- `node --test scripts/validate-public-docs.test.mjs` — passed, 61 tests.
- `node scripts/validate-public-docs.mjs` — passed, 41 published docs pages.
- Platform risk: native interface enumeration is covered by injected fixtures on this
  Linux runner; non-Linux native enumeration remains unverified.

## Implementation Waves And Parallel Candidates

Wave 1, sequential:

- [x] [task-01-restore-network-logging](task-01-restore-network-logging.md)

The implementation and its tests share the launcher output contract, so the task
is intentionally sequential and is not a delegation authorization.

## Open Questions

None.
