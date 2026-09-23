---
id: "01-runtime-lsp-leases"
title: "Runtime LSP leases"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-LSP-FILE-INTELLIGENCE-002
acceptance_criteria:
  - AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.1
  - AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.2
  - AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.3
  - AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.4
  - AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.5
  - AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.6
system_design:
  - ../../specs/platform/system-design/lsp-file-intelligence-01.md
  - ../../specs/platform/system-design/lsp-file-intelligence-02.md
---

# Task 01: Runtime LSP leases

## Summary

Make the backend own a bounded LSP lease and keep its agentctl stream and language-server process alive when a browser detaches. Supply a safe reattachment handshake, explicit Stop, and separate process-exit and transport-failure signals.

## In scope

- Add a backend lease manager around the current LSP handler. It continuously drains the task-host stream, owns request-generation mapping and bounded resume snapshots, and reattaches only authorized browsers for the same execution and language.
- Keep concurrently attached browser windows on independent leases. Count detached leases against the existing limit; release capacity once on Stop, process exit, task-runtime stop, or backend shutdown.
- Integrate lease liveness with idle-session reclaim so a retained server keeps its execution available. Fence admission against concurrent reclaim and preserve explicit task-stop behavior.
- Handle server requests while detached, reconcile reopened documents, avoid stale diagnostics, and normalize reserved WebSocket close codes. Keep the agentctl process manager responsible for process-tree teardown.

## Out of scope

- Browser status rendering, locale copy, and Playwright flows.
- Durable process persistence across backend or task-host restart.

## Acceptance

1. Closing a browser socket leaves the same upstream/process alive and draining; an authorized later attachment resumes its capabilities and progress without another task-host process start.
2. Explicit Stop and every runtime terminal path stop the intended lease and release capacity once; a separate live window is unchanged, a detached lease remains admissible at capacity, and idle reclaim skips an execution with an active lease.
3. The broker isolates old request generations, never replays diagnostics for mismatched document text, responds to server requests while detached, and distinguishes actual server exit from transport failure.

## Verification

```bash
(cd apps/backend && go test ./internal/gateway/websocket ./internal/agentctl/server/api ./internal/orchestrator)
(cd apps/backend && go vet ./internal/gateway/websocket ./internal/agentctl/server/api ./internal/orchestrator)
```

## Files likely touched

- `apps/backend/internal/gateway/websocket/lsp_handler.go`
- `apps/backend/internal/gateway/websocket/lsp_capacity.go`
- `apps/backend/internal/gateway/websocket/lsp_lease.go` and focused tests
- `apps/backend/internal/agentctl/server/api/lsp.go` and focused tests if the stable upstream contract needs a small adjustment
- `apps/backend/internal/backendapp/gateway.go` for lifecycle shutdown wiring
- `apps/backend/internal/backendapp/services.go` or the nearest initialization seam for the lease-liveness callback
- `apps/backend/internal/orchestrator/reconcile_liveness.go` and focused reclaim tests

## Dependencies

None. Task 02 consumes the lease handshake and close-code contract.

## Risks

- A detached server can emit output indefinitely; reading, caching, and client request handling must stay bounded.
- Capacity or process teardown races can leak a lease or interrupt the wrong execution after replacement.
- Idle reclaim and lease admission can race unless both use the same session lifecycle fence or an equivalent generation check.
- A new HTTP or control surface must enforce session authorization before lease lookup.

## Parallelism

`sequential`

## Inputs

- [Plan and preview](plan.md)
- `REQ-PLATFORM-LSP-FILE-INTELLIGENCE-002` and both linked system designs.
- [ADR-2026-09-23](../../decisions/2026-09-23-task-owned-lsp-leases.md)

## Results

Pending.
