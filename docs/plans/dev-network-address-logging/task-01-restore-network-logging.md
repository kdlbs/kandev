---
id: "01-restore-network-logging"
title: "Restore network startup logging"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/go-dev-launcher.md"
---

# Task 01: Restore network startup logging

Implement the startup-banner contract added to the Go dev-launcher spec.

## Acceptance

- `dev`, `start`, and `run` print a `network:` URL for each unique non-loopback,
  non-link-local host address on the backend port.
- IPv4 addresses appear before IPv6 addresses, and IPv6 URLs use brackets.
- Interface enumeration errors do not prevent startup or remove the localhost URL.
- Explicit loopback or specific bind hosts do not advertise unrelated interfaces.
- Internal Vite and agentctl ports remain absent from the network-address lines.

## Verification

Run from the repository root:

```bash
cd apps/backend && go test -run 'Test(ListHostNetworkAddresses|NetworkURLsForPort|LogStartup)' ./internal/launcher
```

## Files likely touched

- `apps/backend/internal/launcher/network.go`
- `apps/backend/internal/launcher/network_test.go`
- `apps/backend/internal/launcher/start.go`
- `docs/public/cli.md`

## Dependencies

None.

## Parallelism

Sequential. The helper, shared logger, and regression tests define one output
contract.

## Inputs

- `docs/specs/platform/requirements/go-dev-launcher.md`, startup output section and scenarios.
- The previous Node behavior in `apps/cli/src/shared.ts` at the pre-migration
  revision, especially `listHostNetworkAddresses`, `networkUrlsForPort`, and
  `logStartupInfo`.
- The current shared Go logger in `apps/backend/internal/launcher/start.go`.

## Output contract

Summarize the changed files, exact test command and result, any remaining
platform-specific risk, and synchronize this task and `plan.md` status.

## Results

- `cd apps/backend && go test -run TestLogStartupPrintsNetworkAddress ./internal/launcher` —
  failed before the fix with the expected missing `network:` assertion.
- `cd apps/backend && go test -run 'Test(ListHostNetworkAddresses|NetworkURLsForPort|LogStartup)' ./internal/launcher` —
  passed, 4 tests.
- `cd apps/backend && go test ./internal/launcher` — passed, 196 tests.
- `cd apps/backend && go test -run TestLogStartupSuppressesNetworkAddressesForLoopbackBind ./internal/launcher` —
  failed before the bind-host fix, then passed after the fix.
- `cd apps/backend && go test -run 'Test(NetworkAddressesForBindHost|LogStartupSuppressesNetworkAddressesForLoopbackBind)' ./internal/launcher` — passed.
- `rtk git diff --check` — passed.
- `gofmt -w apps/backend/internal/launcher/network.go apps/backend/internal/launcher/network_test.go apps/backend/internal/launcher/start.go` — completed successfully.
- `node --test scripts/validate-public-docs.test.mjs` — passed, 61 tests.
- `node scripts/validate-public-docs.mjs` — passed, 41 published docs pages.
- Platform risk: native interface enumeration is covered by injected fixtures on this
  Linux runner; non-Linux native enumeration remains unverified.
- Cleanup: no temporary files, processes, or external runtime state created.
