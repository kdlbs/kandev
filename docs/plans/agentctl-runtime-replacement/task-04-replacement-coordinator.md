---
id: "04-replacement-coordinator"
title: "Coordinate bounded replacement within the healthy backend"
status: done
wave: 4
depends_on: ["03-owned-exit"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001
  - REQ-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002
acceptance_criteria:
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001.2
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001.3
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001.4
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001.9
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002.1
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002.2
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002.3
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002.4
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002.6
system_design:
  - ../../specs/platform/system-design/agent-runtime-availability.md
---

# Task 04: Coordinate bounded replacement within the healthy backend

## Summary

Coordinate bounded replacement within the healthy backend. Preserve original session authority and all prior durable-delivery fixes.

## In scope

Wire one coordinator into backendapp startup and cleanup. Use the existing launcher and authenticated adoption helpers.
Fence local admission before publishing recovering; cancel retired leases and renewal loops.
Apply the three-start budget, bounded delays, sixty-second outage window, and stable-health reset from the design.
Prepare new credentials, required consumer bindings, and adoption records before atomic availability publication.
Treat failed owner-record persistence as candidate failure, not a warning followed by availability.
Handle a backend crash during candidate creation through existing startup ownership discovery.
Make shutdown cancel timers and candidate startup; dispose only owned candidates and never restart after intentional Stop.
Publish bounded metrics and structured sanitized reasons. Do not let a legacy callback publish unavailable for a successor.
Do not enable independent session dispatch merely because the global runtime is healthy.

## Out of scope

Other work orders, unproven Git repairs, automatic external mutation retries, and unrelated executor changes.

## Acceptance

- The scoped outcome passes every named regression, including stale-owner and failure cases.
- No prompt, tool, or Git mutation is replayed by runtime replacement.
- Partial failure leaves accurate availability and admission fences; cleanup affects only owned resources.

## Tests

TestRuntimeReplacementKeepsBackendBoot; TestRuntimeReplacementBudget; TestRuntimeReplacementSingleFlight; TestRuntimeReplacementCrashBeforePublish; TestRuntimeReplacementShutdownRace.
Use fake clocks for retries and a real spawned test control process for death/replacement integration.
Assert changed child identity/credential, unchanged backend PID/boot_id, one renewal owner, and bounded cleanup.
Exercise survivor adoption, survival off, failed auth, foreign owner, persistence failure, and repeated short-lived starts.
Proposed tests belong beside their production owners. Verify the command selects them before recording success.

## Verification

Run each command from the repository root after implementation.

```bash
(cd apps/backend && go test -race ./internal/backendapp ./internal/agent/runtime/agentctl ./internal/agent/runtime/agentctl/launcher ./internal/agent/runtime -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -race ./internal/persistence/storeconformance -count=1)
make -C apps/backend lint
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

For persisted identity changes, include fresh boot, reopen, prior-schema upgrade, interrupted migration, and PostgreSQL conformance. Record unavailable infrastructure as an incomplete gate.

## Files likely touched

- `apps/backend/internal/backendapp/agentctl.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/agent/runtime/agentctl/availability.go`
- `apps/backend/internal/agent/runtime/agentctl/launcher/launcher.go`

New runtime-owner/coordinator and regression files are expected beside these owners.

## Dependencies

Task 03: Detect runtime loss and contain only owned children.

## Risks

Delayed callbacks and partial binding can affect a successor. Prove generation ownership before every state-changing result.

## Parallelism

`sequential`

## Inputs

- [Manifest](plan.md).
- [Requirements](../../specs/platform/requirements/agent-runtime-availability.md).
- [Design](../../specs/platform/system-design/agent-runtime-availability.md).
- [Decision](../../decisions/2026-09-27-agentctl-runtime-replacement.md).

## Results

Completed on 2026-09-27. Added the bounded in-process coordinator with single-flight startup/recovery, one- and two-second backoffs, a three-start/sixty-second episode budget, five-minute stable reset, fenced idempotent manual retry, sanitized metrics/logging, and shutdown cancellation. Runtime replacement keeps the backend boot identity and rejects a candidate whose owned runtime exits before publication. Owner cleanup now waits for retired binding cleanup without blocking a callback that initiated retirement.

Validation passed:

- `(cd apps/backend && TMPDIR=/root/.cache/kandev-agentctl-runtime-tmp go test -p 1 -race ./internal/backendapp ./internal/agent/runtime/agentctl ./internal/agent/runtime/agentctl/launcher ./internal/agent/runtime -count=1)`
- `(cd apps/backend && TMPDIR=/root/.cache/kandev-agentctl-runtime-tmp go test -p 1 -race ./internal/agent/runtime/agentctl -run TestRuntimeReplacementStableWindowResetsBudget -count=20)`
- `(cd apps/backend && TMPDIR=/root/.cache/kandev-agentctl-runtime-tmp go run -p 1 ./cmd/sqlguard ./internal)`
- `(cd apps/backend && TMPDIR=/root/.cache/kandev-agentctl-runtime-tmp go test -p 1 -race ./internal/persistence/storeconformance -count=1)`
- `TMPDIR=/root/.cache/kandev-agentctl-runtime-tmp make -C apps/backend lint`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`

The SQLite identity migration checks and cross-platform compile checks recorded under Task 03 remain applicable. PostgreSQL conformance and native Windows/macOS containment execution remain open release gates; cross-compilation is not native evidence.
