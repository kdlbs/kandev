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

### Runtime-epoch capability refresh (2026-10-03)

The shard 5 settings failure followed the local runtime-replacement E2E. Its profile-specific capability probe successfully reattached the host-utility instance, while the general capability cache stayed empty after epoch invalidation. `/api/v1/agents/available` therefore reported `not_configured` until the worker restarted.

Runtime-epoch reconciliation now schedules bounded host-utility reprobes under the manager lifetime context. Rebound probes share instance creation, publish only for the current runtime epoch and instance, and stop before owned instances are removed. Initial bootstrap results use the same epoch fence.

- RED: `go test ./internal/agent/hostutility -run '^TestHostUtilityRuntimeRebind$' -count=1 -timeout=30s` failed because the successor capability snapshot was absent.
- GREEN: the focused regression passed, then `go test -race ./internal/agent/hostutility -count=1 -timeout=180s` and the complete five-package race command passed: `go test -race ./internal/backendapp ./internal/agent/runtime/lifecycle ./internal/agent/hostutility ./internal/gateway/websocket ./internal/plugins -count=1` (backendapp 84.211s, lifecycle 105.719s, hostutility 1.558s, websocket 5.383s, plugins 22.486s).
- `make -C apps/backend lint` passed with 0 issues. Targeted ESLint, Prettier, and `git diff --check` passed.
- The six-case Chromium reproduction passed with retries disabled: `bash e2e/scripts/run-raw-e2e.sh --project=chromium e2e/tests/layout/agent-runtime-replacement.spec.ts e2e/tests/settings/agent-profile-acp.spec.ts --reporter=list --retries=0` (6 passed, 33.0s). The full pre-fix shard reproduced the same dynamic-config failure (246 passed, 8 skipped, 1 failed).
- Hosted run `37092164822` on the preceding PR tip failed six E2E shards on individual flaky or timeout-sensitive tests (56 passed, 10 skipped, 1 neutral, 8 failed). Retrying only failed jobs reduced this to shards 4 and 10 plus their two dependent aggregates (60 passed, 10 skipped, 1 neutral, 4 failed). The retried shards failed on different tests than their first attempt; no repeated test failure was established.
- During that long run, `main` advanced. The branch was rebased onto the fetched current `main` without conflicts, and `git range-diff` matched all 95 branch patches. After the rebase, `go test -race ./internal/agent/hostutility -count=1 -timeout=180s`, `make -C apps/backend lint`, targeted ESLint, Prettier, and `git diff --check` passed. Fresh hosted CI for the updated base remains pending.

### Agentctl readiness reconciliation (2026-10-03)

The current-head Chromium shard intermittently timed out waiting for `sessionAgentctl` to become ready after the backend session had reached `WAITING_FOR_INPUT`. Investigation showed that session rows can arrive through route hydration or partial WebSocket upserts after the agentctl `starting` notification. The ready fallback previously ran only for the state-change handler, leaving these independent projections inconsistent. The browser also includes `agent_execution_id` on task-session DTOs; that existing backend field is now present in the frontend type.

- RED: the session-slice test failed when a live partial event followed `starting`; the hydrator test failed when a live session row arrived in a later hydration without an agentctl status payload.
- GREEN: `pnpm --filter @kandev/web test -- --run lib/state/hydration/hydrator.test.ts lib/state/slices/session/session-slice.upsert.test.ts lib/ws/handlers/agent-session.test.ts` passed (147 tests).
- Live session state now promotes a matching `starting` agent execution to `ready` across route hydration, full or partial session snapshots, and WebSocket upserts. Execution IDs fence the inference so an old live session does not mark a different new agent execution ready. Unit cases cover both matching and mismatched identities, parked sessions, and missed readiness events.
- `pnpm run typecheck`, `pnpm run lint`, targeted Prettier, and `git diff --check` passed.
- A first E2E rerun used an outdated ignored `apps/web/dist` bundle. After `pnpm run build:e2e`, the terminal context-reset E2E passed (2.9s), the workflow target-delivery E2E passed (27.8s), and the mobile workspace-repository drawer E2E passed (2.4s), each with retries disabled.
- Fresh hosted CI for the final published head remains required.
