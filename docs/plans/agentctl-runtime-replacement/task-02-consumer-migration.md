---
id: "02-consumer-migration"
title: "Move local runtime consumers to the shared owner"
status: done
wave: 2
depends_on: ["01-runtime-owner"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001
  - REQ-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002
acceptance_criteria:
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001.2
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001.9
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002.4
system_design:
  - ../../specs/platform/system-design/agent-runtime-availability.md
---

# Task 02: Move local runtime consumers to the shared owner

## Summary

Move local runtime consumers to the shared owner. Preserve original session authority and all prior durable-delivery fixes.

## In scope

Migrate the consumer inventory in the design before enabling replacement.
Start with standalone executor, lifecycle per-instance clients, run-owner persistence, and hostutility.Manager.
Follow transitive consumers: plugin utility adapters, profile/model discovery, Git/file/shell/preview requests, MCP, and LSP leases.
Retain the same owner across services; do not mutate cfg.Agent concurrently or patch individual clients after publication.
Prepare replacement bindings behind closed admission. Abort all prepared consumers if any required participant fails.
Invalidate stale caches, sockets, and periodic workers without duplicate subscriptions or plugin processes.
Classify every direct Standalone* configuration read as bootstrap-only, diagnostic-only, or migrated.
Preserve independently healthy remote executions; local-utility dependencies may return explicit temporary unavailability.

## Out of scope

Other work orders, unproven Git repairs, automatic external mutation retries, and unrelated executor changes.

## Acceptance

- The scoped outcome passes every named regression, including stale-owner and failure cases.
- No prompt, tool, or Git mutation is replayed by runtime replacement.
- Partial failure leaves accurate availability and admission fences; cleanup affects only owned resources.

## Tests

TestRuntimeConsumerRebindUsesFreshCredential; TestRuntimeConsumerPrepareFailure; TestHostUtilityRuntimeRebind; TestRuntimeRebindPreservesRemoteExecution.
Exercise real client factories against old/new authenticated test servers and delayed old replies.
Assert no requests use the old credential after publication and no utility/Git mutation is retried.
Inventory every remaining config read in Results, including intentionally unchanged bootstrap/logging reads.
Proposed tests belong beside their production owners. Verify the command selects them before recording success.

## Verification

Run each command from the repository root after implementation.

```bash
(cd apps/backend && go test -race ./internal/backendapp ./internal/agent/runtime/lifecycle ./internal/agent/hostutility ./internal/gateway/websocket ./internal/plugins -count=1)
make -C apps/backend lint
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/backendapp/agents.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/orchestrator.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_standalone.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_backend.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager.go`
- `apps/backend/internal/agent/runtime/lifecycle/run_owner_persistence.go`
- `apps/backend/internal/agent/hostutility/manager.go`
- `apps/backend/internal/gateway/websocket/lsp_lease.go`

New runtime-owner/coordinator and regression files are expected beside these owners.

## Dependencies

Task 01: Introduce a generation-fenced runtime owner.

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

Implemented shared-runtime leases across standalone control and per-instance clients, lifecycle operations and completion handling, host-utility instances and caches, plugin-facing utility calls, websocket streams, workspace streams, port/VS Code proxies, and tunnels. Requests and long-lived streams are canceled when their bound runtime retires; late lifecycle results from retired local epochs are discarded. Remote execution clients remain unbound and continue independently. No prompt, tool, utility, or Git mutation is retried.

The current owner publishes one immutable endpoint/credential binding. Consumers that need clients resolve them from its lease; utility instances and proxy caches are rebuilt lazily by runtime epoch. No consumer-specific network binding is published ahead of the owner. The replacement coordinator work order owns closed-admission candidate preparation and publication ordering.

Remaining configuration reads are limited to startup/adoption/bootstrap in `backendapp/agentctl.go`, the startup readiness probe in the same file, and the boot-time PID fallback wired in `backendapp/agents.go`. `backendapp/orchestrator.go` reads the configured port for a startup diagnostic. Lifecycle persistence reads endpoint and port from durable execution/run-owner records, not mutable configuration. Static control-client fields in `NewStandaloneExecutor` and the host-utility constructor remain compatibility paths for tests and embedded callers; backendapp production wiring injects the shared owner.

Validation passed:

- `(cd apps/backend && go test -race ./internal/backendapp ./internal/agent/runtime/lifecycle ./internal/agent/hostutility ./internal/gateway/websocket ./internal/plugins -count=1)`
- `make -C apps/backend lint`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`

The first race run exposed a tunnel shutdown goroutine leak when a compatibility client supplied a non-cancelable background context. The tunnel now derives its own cancelable context while preserving runtime-retirement cancellation; the websocket race test and final full race command passed. The linter's nested-branch finding in host-utility cleanup was extracted into a helper; the final backend lint passed.
