---
id: "01-scope-task-mcp-tools-by-provider"
title: "Scope task MCP tools by provider"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/requirements/provider-aware-review-automation.md"
---

# Task 01: Scope task MCP tools by provider

## Acceptance

- Task-mode MCP registers the GitHub PR pair for `github`, the GitLab MR pair
  for `gitlab`, both for a mixed set, and neither for empty or unsupported sets.
- Agentctl instance creation accepts the explicit provider set and its live
  provider endpoint replaces the set without changing MCP mode.
- A changed effective registry produces tool-list-changed behavior while backend
  handlers and their defense-in-depth validation remain registered.

## Verification

- `cd apps/backend && go test -race ./internal/mcp/server ./internal/agentctl/server/api ./internal/agentctl/server/config ./internal/agentctl/server/instance ./internal/agent/runtime/agentctl`

Add table-driven membership tests and live replacement tests first; they must
fail against the unconditional registration behavior before production changes.

## Files likely touched

- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/server_test.go`
- `apps/backend/cmd/agentctl/main.go`
- `apps/backend/internal/agentctl/server/api/server.go`
- `apps/backend/internal/agentctl/server/api/server_test.go`
- `apps/backend/internal/agentctl/server/config/config.go`
- `apps/backend/internal/agentctl/server/instance/instance.go`
- `apps/backend/internal/agent/runtime/agentctl/control.go`
- `apps/backend/internal/agent/runtime/agentctl/client.go`

## Dependencies

None.

## Parallelism

Parallel-safe with Tasks 04 and 06 only. It owns MCP server and agentctl provider
contract files; coordinate before touching shared lifecycle/executor files.

## Inputs

- Spec sections `Provider-scoped MCP discovery`, `Runtime propagation and
  refresh`, `API surface`, and `Failure modes`
- ADR decision items 1-4 and 6
- Existing `SetMode`, `registerKanbanTools`, mcp-go `SetTools`, and agentctl MCP
  mode endpoint/client patterns

## Risks

- Provider replacement and mode replacement can race. Keep one synchronized
  registry-rebuild path and verify both update orders.
- Treating an omitted set as all providers would preserve the bug; the new
  contract must fail closed.

## Output contract

Report the RED test evidence, normalization boundary, exact tool membership and
notification behavior, files changed, verification result, and remaining risks.
Mark this task `done` and update its plan checkbox in the same conversation.

## Result

- RED: `TestServerModeTask_AbsentProvidersFailClosedForReviewAutomation` failed
  against unconditional task-mode registration.
- Provider values are normalized at the shared MCP boundary and again when
  agentctl builds per-instance configuration; unsupported values are discarded.
- Task mode exposes the PR pair only for GitHub, the MR pair only for GitLab,
  both for a mixed set, and neither for empty/unsupported sets. `SetProviders`
  assembles a complete registry off to the side, replaces it with one mcp-go
  `SetTools` call, and preserves mode; normalized no-op mode/provider updates
  emit no notification.
- Verification: `go test -race ./internal/mcp/server ./internal/agentctl/server/api ./internal/agentctl/server/config ./internal/agentctl/server/instance ./internal/agent/runtime/agentctl` (pass).
- Review remediation: an initialized-session regression proves a live provider
  rebuild emits exactly one list-changed notification with the complete final
  tool list, and equivalent normalized updates are skipped. Verification:
  `cd apps/backend && go test -race ./internal/mcp/server` (189 tests passed).
- Remaining risk: launch/lifecycle propagation and live refresh are covered by
  Tasks 02 and 03.
