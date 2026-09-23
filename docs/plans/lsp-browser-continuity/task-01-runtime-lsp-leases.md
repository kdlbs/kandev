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
  - AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.8
  - AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.9
system_design:
  - ../../specs/platform/system-design/lsp-file-intelligence-01.md
  - ../../specs/platform/system-design/lsp-file-intelligence-02.md
---

# Task 01: Runtime LSP leases

## Summary

Make the backend own a bounded LSP lease and keep its agentctl stream and language-server process alive when a browser detaches. Supply a safe reattachment handshake, explicit Stop, and separate process-exit and transport-failure signals.

## In scope

- Add a backend lease manager around the current LSP handler. It continuously drains the task-host stream, owns request-generation mapping and bounded resume snapshots, and reattaches only authorized browsers for the same execution and language.
- Keep concurrently attached browser windows on independent leases, including duplicated-tab hints. Count detached leases against the existing limit; release capacity once on editor-idle release, Stop, oldest-detached capacity eviction, process exit, task-runtime stop, or backend shutdown.
- Integrate lease liveness with all `reclaimIdleSession` callers: `idle_session_reaper.go`, `event_handlers_agent.go`, and `event_handlers_streaming.go`. Fence admission against concurrent reclaim and preserve explicit task-stop behavior.
- Handle dynamic registrations and `workspace/configuration` while detached, subscribe to saved per-language settings changes, map `$/cancelRequest`, cancel pending requests on detach/Stop, reconcile reopened document versions, invalidate old diagnostics, and normalize reserved WebSocket close codes. Stop sends `shutdown`/`exit`; agentctl marks confirmed process exit distinctly from upstream read failure and owns process-tree teardown.
- Add the all-off, restart-required `features.lspBrowserContinuity` release toggle through profiles, typed backend config, runtime flag registry/binding, and backend construction/WebSocket gates. Its disabled path preserves the browser-owned proxy and idle-reclaim behavior. Update the `limits.lspMaxConnections` catalog description without changing its identity or default; coordinate with startup-configuration-parity Task 04.

## Out of scope

- Browser status rendering, locale copy, and Playwright flows.
- Durable process persistence across backend or task-host restart.

## Acceptance

1. Closing a browser socket leaves the same upstream/process alive and draining; an authorized later attachment resumes its capabilities and progress without another task-host process start.
2. Explicit Stop, editor-idle release, capacity eviction, and every runtime terminal path stop the intended lease and release capacity once; a separate live window is unchanged, a detached lease remains admissible at capacity, and all idle-reclaim callers skip an execution with an active lease even after the agent turn completes.
3. The broker isolates old request generations, translates cancellations, retains effective dynamic registrations and latest settings, uses increasing server-facing document versions, clears diagnostics on detach, accepts only qualifying fresh publications, and distinguishes confirmed server exit from transport failure.
4. The release toggle is off in all shipped profiles; direct backend access uses the legacy behavior when disabled, and enabled-only work is unreachable without the flag.

## Verification

```bash
(cd apps/backend && go test ./internal/gateway/websocket ./internal/agentctl/server/api ./internal/orchestrator ./internal/backendapp ./internal/runtimeflags ./internal/common/config ./internal/profiles)
(cd apps/backend && go vet ./internal/gateway/websocket ./internal/agentctl/server/api ./internal/orchestrator ./internal/backendapp)
```

## Files likely touched

- `apps/backend/internal/gateway/websocket/lsp_handler.go`
- `apps/backend/internal/gateway/websocket/lsp_capacity.go`
- `apps/backend/internal/gateway/websocket/lsp_lease.go` and focused tests
- `apps/backend/internal/agentctl/server/api/lsp.go` and focused tests for the upstream process-exit signal
- `apps/backend/internal/backendapp/gateway.go` for lifecycle shutdown wiring
- `apps/backend/internal/backendapp/services.go` or the nearest initialization seam for the lease-liveness callback
- `apps/backend/internal/orchestrator/reconcile_liveness.go`, `idle_session_reaper.go`, `event_handlers_agent.go`, `event_handlers_streaming.go`, and focused reclaim tests
- `apps/backend/internal/user/service/service.go` or the existing settings-save event seam for detached configuration updates
- `profiles.yaml`, `apps/backend/internal/common/config/config.go`, `apps/backend/internal/runtimeflags/{registry,config}.go` and their contract tests
- `apps/backend/internal/common/config/catalog.go` and its catalog tests
- `AGENTS.md` Observability section and `apps/backend/AGENTS.md` LSP ownership notes

## Dependencies

None. Task 02 consumes the lease handshake and close-code contract.

## Risks

- A detached server can emit output indefinitely; reading, caching, and client request handling must stay bounded.
- Capacity or process teardown races can leak a lease or interrupt the wrong execution after replacement.
- Idle reclaim and lease admission can race unless both use the same session lifecycle fence or an equivalent generation check.
- A new HTTP or control surface must enforce session authorization before lease lookup.
- Holding the lease pins the whole task host; idle release and capacity eviction must allow normal reclaim again.

## Parallelism

`sequential`

## Inputs

- [Plan and preview](plan.md)
- `REQ-PLATFORM-LSP-FILE-INTELLIGENCE-002` and both linked system designs.
- [ADR-2026-09-23](../../decisions/2026-09-23-task-owned-lsp-leases.md)

## Results

Pending.
