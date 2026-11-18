---
id: "06-generation-binding"
title: "Bind MCP authority to provider generations"
status: done
wave: 3
depends_on:
  - 05-replacement-admission
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-APPLY-001
  - REQ-AGENTS-MCP-APPLY-002
  - REQ-AGENTS-MCP-APPLY-003
acceptance_criteria:
  - AC-AGENTS-MCP-APPLY-001.3
  - AC-AGENTS-MCP-APPLY-001.6
  - AC-AGENTS-MCP-APPLY-002.1
  - AC-AGENTS-MCP-APPLY-002.3
  - AC-AGENTS-MCP-APPLY-002.4
  - AC-AGENTS-MCP-APPLY-002.5
  - AC-AGENTS-MCP-APPLY-003.2
system_design:
  - ../../specs/agents/system-design/task-mcp-application.md
---

# Task 06: Bind MCP authority to provider generations

## Summary

Implement the retained-helper replacement protocol and immutable provider MCP scopes.
Keep provider credentials and delivery generations separate from workspace control authority.

## In scope

Negotiate `provider-mcp-replacement-v1`. Add idempotent prepare, retire, activate, accept, release, and status operations.
Persist helper receipts before effects and SQL binding snapshots/current pointers with conditional transitions.
Add source/target process and binding identity to application persistence and status projections.
Fence scope routes, legacy routes, MCP bridge envelopes, backend effects, catalog evidence, and late provider events.
Keep the settled source conversation stream, incarnation, and SQL harness generation. Persist only the new provider process/binding mapping. Do not rotate delivery ownership.
Freeze workspace configuration while applying a separate provider configuration and credential.

## Out of scope

Automatic coordinator scheduling, live-binding mutation, new package grants, whole-helper replacement, or provider capability enablement without conformance.

## Acceptance

1. Old transports and credentials cannot invoke successor tools. An unscoped route cannot inherit successor authority. Pre-acceptance scopes allow catalog reads but no effects.
2. Duplicate/lost requests and backend restart reconcile one persisted operation. Unknown termination, storage failure, or inconsistent generation evidence never creates another process.
3. Binding rotation preserves workspace control connections and environments. Journal history and queue identities survive, and late old events cannot settle new work.

## Verification

Use TDD and deterministic barriers for every transition. Run from repository root.

```bash
(cd apps/backend && go test -race ./internal/mcp/server ./internal/mcp/profile ./internal/agentctl/server/api ./internal/agentctl/server/process ./internal/agentctl/journal ./internal/agent/runtime/lifecycle ./internal/task/repository/sqlite -run 'Test(MCPProviderReplacement|BuiltinGeneration|BuiltinApplication|BuiltinBinding|NativeResumeRetirement|SilentRestoreDelivery)' -count=1)
bash scripts/lint-go-changed $(git ls-files --modified --others --exclude-standard -- '*.go')
```

New tests: `internal/mcp/server/builtin_generation_test.go`: `TestBuiltinGenerationStaleCalls` and `TestBuiltinGenerationLegacyRoutes`.
`internal/agentctl/server/api/provider_replacement_test.go`: `TestMCPProviderReplacementReplay` and `TestMCPProviderReplacementAuthorization`.
`internal/agentctl/journal/provider_replacement_test.go`: `TestMCPProviderReplacementJournalTransition`.
`internal/task/repository/sqlite/builtin_generation_test.go`: `TestBuiltinGenerationAcceptanceCAS`.
Include reconnects, old SSE/message sessions, forged scope fields, same tool name across generations, and backend work already queued before retirement.
Inject crashes before/after retire, spawn, journal-owner registration, SQL acceptance, and helper acceptance acknowledgement.
Count starts and effects. Assert the same execution ID with distinct process generations and unchanged native delivery identity.

## Files likely touched

- `apps/backend/internal/agentctl/types/` and `internal/agent/runtime/agentctl/` (protocol/client)
- `apps/backend/internal/agentctl/server/api/builtin_mcp.go` and new provider operation handlers
- `apps/backend/internal/agentctl/server/process/builtin_mcp_preparation.go` and provider replacement coordinator
- `apps/backend/internal/mcp/server/` (generation scopes and bridge envelope)
- `apps/backend/internal/agentctl/types/streams/mcp_execution.go` and backend MCP dispatch authorization
- `apps/backend/internal/agentctl/journal/` and durable delivery transition code
- `apps/backend/internal/task/repository/sqlite/`, `internal/task/models/`, and binding evidence projection
- `apps/backend/internal/agent/runtime/lifecycle/builtin_mcp_promotion.go` (explicit current-scope recovery, no weakened legacy check)

Paths without an `apps/backend/` prefix in grouped bullets are relative to that directory.

## Dependencies

05-replacement-admission. Work order 02 integrates the protocol with human-save coordination.

## Risks

The old execution ID remains valid for workspace services, so it cannot alone authorize provider tools.
Old backends must not adopt transitioned scope records as legacy initial bindings.
A helper storage error must leave workspace services alive while provider admission remains closed.

## Parallelism

Sequential. No delegation is authorized.

## Inputs

- [Generation authority](../../specs/agents/system-design/task-mcp-application.md#generation-scoped-mcp-authority).
- [Delivery identity](../../specs/agents/system-design/task-mcp-application.md#delivery-and-event-identity).
- [Lifetime decision](../../decisions/2026-11-15-provider-refresh-workspace-lifetime.md).
- Existing initial-binding, project-delivery, native-resume retirement, and silent-restore generation tests.

## Results

Immutable provider scopes bind execution, helper, process generation, journal provenance, profile digest, and a new private bearer credential. Legacy endpoints and retired generation routes cannot acquire successor authority. Both helper and backend dispatch gates reject stale calls before effects. Fresh launch metadata cannot inherit provider authority. Retained peers restore authority from the matching SQL owner.

Durable helper receipts cover prepare, release, retirement, activation, catalog verification, acceptance, failure, recovery, and explicit Retry. The operation is recorded before each irreversible provider action. Lost responses reconcile from receipts. Failed spawn consumes its prospective process generation. Retirement requires confirmed reaping. New protocol headers fence older control clients. Advanced journal format requires compatible readers.

SQL persists the saved selection, attempt, target binding, complete catalog, and final acknowledgement independently. A newer save cannot rewrite an attempt or relabel its result. Explicit Retry resolves current saved choices within current workspace policy and checks request/version/revisions atomically. Uncertain acceptance parks without another automatic mutation. Workspace services remain available while provider effects stay closed.

Refresh preserves the journal stream, incarnation, cursor, native conversation, and SQL harness generation. The former successor-stream prototype was removed. Scope journal generation is acceptance provenance. A later authorized context reset advances delivery ownership independently and cannot change frozen MCP permissions.

Focused scope, receipt, recovery, API, lifecycle, SQL, native-provider, concurrency, and input tests cover these contracts. Final validation is recorded in the plan. No live user session was restarted.

Selection hashes are canonical across manifest order. SQL checks the staged package set against the saved hash before activation and acceptance. Acceptance also requires every selected package's catalog, unique nonempty tool names, and no excluded reserved server. Regression cases reject a wider staged target, partial or empty catalogs, duplicate tools, and excluded-package observations.
