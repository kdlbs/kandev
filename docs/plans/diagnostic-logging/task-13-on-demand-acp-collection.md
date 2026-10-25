---
id: "13-on-demand-acp-collection"
title: "On-demand ACP collection"
status: done
wave: 10
depends_on:
  - "12-custom-bundle-contracts"
plan: "plan.md"
spec: "../../specs/platform/requirements/diagnostic-logging.md"
---

# Task 13: On-demand ACP collection

## Acceptance

- Debug-enabled agentctl exports only recognized retained raw and normalized
  files for the exact requested session under a server-clamped byte limit;
  debug-disabled, unknown, expired, and malicious requests fail closed.
- The backend collects explicitly selected host or reachable executor ACP
  evidence only after owner/admin authorization, with at most ten sessions,
  two concurrent executor reads, and a 30-second total deadline.
- ACP-inclusive archives use server-numbered paths, enforce the 96/48/96/2 MiB
  source budgets and existing global/temp caps, and become partial with exact
  manifest warnings for unavailable, invalid, or truncated sessions.

## Verification

```bash
cd apps/backend
go test ./internal/agentctl/server/api \
  ./internal/agent/runtime/agentctl \
  ./internal/agent/runtime/lifecycle \
  ./internal/system/logbundle
go test -race ./internal/agentctl/server/api \
  ./internal/agent/runtime/agentctl \
  ./internal/system/logbundle
```

## Files likely touched

- `apps/backend/internal/agentctl/server/api/server.go`
- `apps/backend/internal/agentctl/server/api/acp_debug_export.go`
- `apps/backend/internal/agentctl/server/api/acp_debug_export_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/shared/acplog.go`
- `apps/backend/internal/agent/runtime/agentctl/client.go`
- `apps/backend/internal/agent/runtime/agentctl/client_files.go`
- `apps/backend/internal/agent/runtime/lifecycle/diagnostic_materializer.go`
- `apps/backend/internal/system/logbundle/acp_collector.go`
- `apps/backend/internal/system/logbundle/archive.go`
- `apps/backend/internal/system/logbundle/service.go`
- `apps/backend/internal/system/logbundle/*_test.go`
- `apps/backend/internal/backendapp/helpers.go`

## Dependencies

- Task 12 defines accepted sources, selected-session job identity, capability
  gating, authorization, and manifest DTOs.

## Parallelism

Sequential. It crosses agentctl, lifecycle/runtime, and the shared logbundle
archive/job state established by Task 12.

## Inputs

- Spec: performance/resource contract, ACP internal export API, Permissions,
  Failure modes, Persistence guarantees, and ACP scenarios.
- Plan: On-demand ACP export.
- Agentctl guidance: recognized debug filenames, writer rotation/retention,
  and debug-only privacy contract.

## Risks

- Raw frames may contain secrets and full work content; never broaden a
  selected session to a directory scan or reuse client-provided ZIP paths.
- Docker/remote executors may disappear while collecting. Cancellation must
  close bodies/temp files without stalling the agent or product request path.
- Do not add continuous ACP forwarding, a permanent backend ACP store, or an
  unbounded archive/decompression path.

## Output contract

Report exporter/collector contracts, executor coverage, concurrency/deadline
bounds, archive validation, partial semantics, exact tests/results,
blockers/risks, and synchronize this task plus `plan.md` status.

## Results

Implemented on-demand ACP export and collection across agentctl, lifecycle,
backendapp, and the diagnostic archive service.

- Added a debug/dev-gated agentctl export route that returns only exact-session
  raw and normalized files, with server-owned ZIP paths, regular-file checks,
  byte/file limits, and rotated-file matching.
- Added authenticated runtime-client and lifecycle seams plus the backendapp
  exporter/provider adapters. Session authorization happens before executor or
  host-file access; explicit selections are limited to ten and can include
  host-retained sessions while unavailable sessions are rejected.
- Added bounded, 30-second ACP collection with 96 MiB ACP and 96 MiB backend
  budgets. Remote ZIPs are fully validated before any archive entry is written;
  invalid, unavailable, and truncated evidence produces partial warnings.
- Verification: `go test ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/system/logbundle ./internal/backendapp -count=1` (pass; 1,468 tests).
- Verification: `go test -race ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/system/logbundle -count=1` (pass; 1,242 tests).
