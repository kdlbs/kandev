---
id: "03-refresh-tools-after-source-attachment"
title: "Refresh tools after source attachment"
status: done
wave: 3
depends_on: ["01-scope-task-mcp-tools-by-provider", "02-propagate-providers-through-agent-launch"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/provider-aware-review-automation.md"
---

# Task 03: Refresh tools after source attachment

## Acceptance

- A successful workspace-source batch and legacy add-branch operation each
  recompute the task provider union once and refresh every active task session.
- No-active-session is a successful no-op; mixed-provider addition exposes the
  new pair and uses the live agentctl provider operation.
- Refresh failure leaves the attached source committed, preserves the previous
  fail-closed runtime subset, logs actionable context, and is corrected by the
  already-tested next launch/resume path.

## Verification

- `cd apps/backend && go test -race ./internal/backendapp ./internal/agent/runtime/lifecycle ./internal/task/service`

Add live addition and failure-path tests before implementation. Prove the shared
batch invokes refresh once rather than once per repository.

## Files likely touched

- `apps/backend/internal/backendapp/workspace_source_materializer.go`
- `apps/backend/internal/backendapp/branch_materializer.go`
- `apps/backend/internal/backendapp/provider_refresher.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/task/service/service_workspace_sources.go`
- Adjacent `*_test.go` files

## Dependencies

- Task 01 provides the live agentctl endpoint/client.
- Task 02 provides authoritative derivation and lifecycle transport types.

## Parallelism

Sequential after Tasks 01 and 02. It owns the live-refresh lifecycle seam and
both source materializers.

## Inputs

- Existing `commitWorkspaceSourceBatch`, `workspaceSourceMaterializer`, legacy
  branch finalization, and `agentctlRescanner` patterns
- Spec `Live source addition` and `Failed live refresh` scenarios
- ADR decision item 5

## Risks

- Refreshing before persistence/materialization completes can expose capabilities
  for a source that is later compensated. Trigger only after successful final
  commit/materialization.
- Do not turn a transient agentctl failure into a source-attachment rollback.

## Output contract

RED evidence: the new attachment regressions first failed because the task
service had no provider-refresh hook and backendapp had no provider-union
refresher. The implementation now invokes the refresher once from the shared
post-materialization commit boundary, which covers both workspace-source
batches and legacy `add_branch`; a two-repository batch test proves it is not
called once per repository.

The backendapp refresher reads the provider identities with one joined task-
repository/repository query, normalizes their union, and attempts every
non-terminal task session. The lifecycle method resolves each session to its
live execution and replaces the agentctl MCP provider set; missing executions
are no-ops. Refresh errors are logged/aggregated but ignored by task
attachment, so the source remains committed and the previous live subset is
preserved on a failed agentctl request. Launch/resume remains the
reconciliation fallback.

Review remediation bounds the post-commit live refresh with a detached
five-second context, so a blocked database or agentctl dependency cannot delay
the attachment event indefinitely. A blocking-refresher regression covers the
timeout, and the joined provider query has direct SQLite coverage.

Changed files:

- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/provider_refresher.go`
- `apps/backend/internal/backendapp/provider_refresher_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_mcp_providers_test.go`
- `apps/backend/internal/task/service/service.go`
- `apps/backend/internal/task/service/service_workspace_sources.go`
- `apps/backend/internal/task/service/service_workspace_sources_test.go`

Verification:

```text
cd apps/backend && go test -race ./internal/backendapp ./internal/agent/runtime/lifecycle ./internal/task/service
ok  github.com/kandev/kandev/internal/backendapp          10.984s
ok  github.com/kandev/kandev/internal/agent/runtime/lifecycle 13.918s
ok  github.com/kandev/kandev/internal/task/service        51.030s
```

Review verification: `cd apps/backend && go test -race ./internal/task/service
./internal/backendapp ./internal/task/repository/sqlite` passed (1238 tests
across 3 packages).

Risk: live refresh is best-effort after persistence/materialization. A
temporary agentctl failure does not roll back an attachment; the next launch
or resume recomputes the authoritative union.
