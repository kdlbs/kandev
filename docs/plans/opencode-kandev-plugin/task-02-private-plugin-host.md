---
id: "02-private-plugin-host"
title: "Build the private plugin host and delivery contract"
status: completed
wave: 2
depends_on:
  - "01-prove-plugin-artifact"
plan: "plan.md"
requirements:
  - REQ-AGENTS-OPENCODE-PLUGIN-002
  - REQ-AGENTS-OPENCODE-PLUGIN-003
acceptance_criteria:
  - AC-AGENTS-OPENCODE-PLUGIN-002.1
  - AC-AGENTS-OPENCODE-PLUGIN-002.2
  - AC-AGENTS-OPENCODE-PLUGIN-002.3
  - AC-AGENTS-OPENCODE-PLUGIN-002.4
  - AC-AGENTS-OPENCODE-PLUGIN-002.5
  - AC-AGENTS-OPENCODE-PLUGIN-003.1
  - AC-AGENTS-OPENCODE-PLUGIN-003.2
  - AC-AGENTS-OPENCODE-PLUGIN-003.3
  - AC-AGENTS-OPENCODE-PLUGIN-003.4
  - AC-AGENTS-OPENCODE-PLUGIN-003.6
  - AC-AGENTS-OPENCODE-PLUGIN-003.8
system_design:
  - ../../specs/agents/system-design/opencode-kandev-plugin.md
---

# Task 02: Build the private plugin host and delivery contract

## Summary

Implement the small shared transport for declared OpenCode behavior modules.
Own its lease, authenticated loopback listener, configuration watch, bounded
outbox, receipt contract, and cleanup. Keep runtime activation disconnected.

## Scope and owned files

- Own Go `host.go`, `lease.go`, `protocol.go`, `registry.go`, `limits.go`, and
  sibling tests in `internal/agentctl/server/opencodeplugin`.
- Own TypeScript `src/core/`, protocol validators, registry lifecycle, and
  `test/core.test.mjs` / `test/registry.test.mjs` in the plugin package.
- Own shared sanitized protocol fixtures and their Go/TypeScript contract tests.
- Define a typed commit interface returning durable receipts. Task 03 connects
  it to the process manager and existing journal.

Use a dedicated `127.0.0.1:0` listener with only hello, configuration, and event
operations. Protect it with a launch-specific credential. Add no exception to
agentctl control authentication and expose no process/filesystem/MCP actions.

Implement the design's payload, queue, request, claim, and time limits. A
configuration update carries complete typed module configuration and a revision.
It cannot execute arbitrary module methods. Setup returns cleanup promptly.
Delivery errors cannot block native event draining or crash OpenCode setup.

Keep receiver authentication independent of ordinary application auth mode.
Use private bootstrap-file protection on each supported OS. Never print the
credential or include it in configuration options, environment dumps, or logs.

## Acceptance

1. Unknown modules, versions, invalid credentials, stale leases, foreign native
   sessions, oversized bodies, and malformed identities fail before effects.
2. A fake committer proves that acknowledgement follows durable acceptance.
   Retry after an uncertain response preserves event identity and payload checks.
3. Independent modules share lifecycle and delivery without shell-specific core
   branches. Disablement, gaps, cancellation, and cleanup preserve ACP usability.

## Verification

Add `host_test.go`, `lease_test.go`, `protocol_test.go`, `limits_test.go`, and
`registry_test.go`. Include `TestBehaviorRegistryContract`,
`TestOpenCodePluginLease`, `TestOpenCodePluginRedaction`, and
`TestOpenCodePluginDeliveryGap`.

From `apps/backend/`:

```bash
go test -tags fts5 -count=1 ./internal/agentctl/server/opencodeplugin
go test -race -tags fts5 -count=1 ./internal/agentctl/server/opencodeplugin -run 'TestOpenCodePlugin(Lease|DeliveryGap|ConcurrentClose)'
```

From `apps/`:

```bash
pnpm --filter @kandev/opencode-plugin typecheck
pnpm --filter @kandev/opencode-plugin test
pnpm --filter @kandev/opencode-plugin build
pnpm --filter @kandev/opencode-plugin check:artifact
```

Use deterministic event barriers for concurrency tests. A fake committer proves
transport ordering only. Do not label it journal-backed acceptance evidence.

## Exclusions and dependencies

Task 01 must establish the real plugin ABI. Shell correlation, actual journal
integration, automatic staging, feature flags, and UI scenarios belong to later
work orders. No host listener starts in production from this work alone.

## Risks

Loopback is a routing boundary, not authentication. Always require the lease.
Avoid unbounded maps for event receipts, claims, or callback retry state.
Plugin reload creates a new producer but does not grant another module version.

## Results

Completed on 2026-10-29.

- Added the loopback host with a launch lease, per-launch bearer credential,
  mode-0600 bootstrap file, `hello`, revisioned configuration long poll, and
  bounded event batches.
- Added the static behavior registry with behavior-owned configuration and
  admission, size/count limits, receipt evidence, and a commit-before-ack
  interface. The admission fence orders configuration revocation and close
  against the durable write.
- Tests cover invalid credentials, stale leases, foreign sessions, unknown
  behavior versions, oversized batches and bodies, configuration wakeups,
  durable acknowledgement, lost acknowledgement, redaction, and concurrent
  close. Race tests pass for lease, delivery gap, and concurrent close.
- No production listener starts from this work alone.
