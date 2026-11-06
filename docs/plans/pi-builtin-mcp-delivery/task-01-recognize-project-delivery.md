---
id: "01-recognize-project-delivery"
title: "Recognize prepared Pi project delivery"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-BUILTIN-MCP-002
  - REQ-AGENTS-BUILTIN-MCP-003
  - REQ-AGENTS-BUILTIN-MCP-004
  - REQ-AGENTS-MCP-PREP-005
acceptance_criteria:
  - AC-AGENTS-BUILTIN-MCP-002.4
  - AC-AGENTS-BUILTIN-MCP-003.1
  - AC-AGENTS-BUILTIN-MCP-003.2
  - AC-AGENTS-BUILTIN-MCP-003.4
  - AC-AGENTS-BUILTIN-MCP-003.5
  - AC-AGENTS-BUILTIN-MCP-003.6
  - AC-AGENTS-BUILTIN-MCP-004.1
  - AC-AGENTS-MCP-PREP-005.1
  - AC-AGENTS-MCP-PREP-005.2
  - AC-AGENTS-MCP-PREP-005.4
  - AC-AGENTS-MCP-PREP-005.5
system_design:
  - ../../specs/agents/system-design/builtin-mcp-management.md
  - ../../specs/agents/system-design/builtin-mcp-project-delivery.md
  - ../../specs/agents/system-design/agent-mcp-preparation.md
---

# Task 01: Recognize prepared Pi project delivery

## Summary

Carry successful Pi project preparation through authenticated configure into
ACP admission. Preserve rejection for each selected package without a valid
route. Deliver the entire handoff and regression coverage in one pass.

## In scope

- Produce an immutable record only after successful, nonempty materialization.
- Bind the record to the exact strategy, workspace, task, instance, and manifest.
- Negotiate helper support and round-trip the record through configure.
- Validate and clone the record under configuration/start ordering.
- Apply per-package admission and truthful evidence to every session operation.
- Cover real producer wiring, API configuration, adapter wire behavior, and failures.

## Out of scope

Provider capability overrides, new Cursor project admission, package grants,
live project-file reload, UI changes, credentials, and Kubernetes infrastructure.
Do not modify production or permanent tests until the user requests implementation.

## Acceptance

1. The new/load/resume/reset regression passes with complete prepared Pi
   delivery and no built-in ACP wire entries. Missing or partial delivery fails
   before provider RPC. Include Core present plus selected Diagnostics absent.
2. Producer, client, API, process, and adapter tests prove execution-bound
   evidence and helper negotiation. Failed writes, skipped preparation, invalid
   records, omitted replacement records, and prior-process records cannot admit
   missing packages. Preserve same-process records across valid session resets.
3. Existing ACP filtering, package pruning, Cursor behavior, Pi file ownership,
   and user-file merge tests pass. Delivery evidence never invents connection
   health. Every changed suite and the documentation preflight pass.

## TDD sequence

1. Read the [design](../../specs/agents/system-design/builtin-mcp-project-delivery.md)
   and the plan's test matrix.
2. Add `TestBuiltinSessionRequestsAcceptPreparedProjectDelivery` to
   `acp/builtin_mcp_test.go`. Use `newSessionResumeAdapter` and a generated Pi
   file. Introduce only the typed declarations needed for a compiling fixture.
   Demonstrate the unsupported Core error before the behavioral correction.
3. Add the producer and handoff regressions named in the plan. Introduce the
   typed record in the smallest compiling change. Do not bypass the red test
   through `AssumeMcpHttp`, a provider-name check, or an unvalidated boolean.
4. Add mixed-package, identity, generation, write-failure, helper, and evidence
   cases. Use channel barriers for concurrency. Register cleanup immediately.
5. Run the exact checks. Update this Results section, the plan, and the two
   directly affected companion-plan links with current evidence.

## Verification

Run from the repository root. New test names are specified in the plan.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run 'TestBuiltinSessionRequestsAcceptPreparedProjectDelivery|TestBuiltinSessionRequestsRejectPartialProjectDelivery|TestBuiltinProjectDeliveryEvidenceIsNotConnectionHealth|TestBuiltinSessionRequestsRejectUnsupportedInjection' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/server/adapter ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/mcpconfig ./internal/mcp/profile -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -run 'ProjectMCP|ProjectMcp|PiProject|PassthroughMCP|PassthroughMcp|Builtin' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/process ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle -run 'ProjectMCPDelivery|PreparedProjectDelivery|PartialProjectDelivery|BuiltinProjectDelivery' -count=1)
git diff --name-only -z -- '*.go' | xargs -0 -r bash scripts/lint-go-changed
git diff --check
```

Also run the plan's complete [documentation preflight](plan.md#documentation-preflight)
from the repository root. Add new Go files to changed-file lint explicitly
while they remain untracked. A successful `go test` with no matching tests is
not acceptance. Confirm each proposed regression exists and runs.

The process integration `TestPiProjectMCPDeliveryACPFlow` supplies internal
end-to-end evidence. A browser test cannot establish provider transport
admission. An optional real-Pi smoke must state its runtime and executor.

## Files likely touched

Paths identify owned code and adjacent tests. New filenames are proposed.

- `apps/backend/internal/agentctl/types/project_mcp_delivery.go` and its test.
- `apps/backend/internal/mcp/profile/builtin_binding.go` and its digest tests.
- `apps/backend/internal/agent/runtime/lifecycle/manager_project_mcp.go`,
  `manager_project_mcp_test.go`, `types.go`, and `manager_launch.go`.
- `apps/backend/internal/agent/runtime/agentctl/client.go` and
  `project_mcp_delivery_test.go`.
- `apps/backend/internal/agentctl/server/api/server.go`, `identity.go`, and
  `project_mcp_delivery_test.go`.
- `apps/backend/internal/agentctl/server/process/manager.go` and
  `project_mcp_delivery_test.go`.
- `apps/backend/internal/agentctl/server/adapter/adapter.go`,
  `transport/shared/config.go`, and their configuration tests.
- `apps/backend/internal/agentctl/server/adapter/transport/acp/builtin_mcp.go`,
  `builtin_mcp_test.go`, and `adapter_session.go`.
- This package and related documentation links for final results.

Inspect actual filenames before editing. Keep large-file additions in focused
helpers and test files. If a new file falls outside these commands, run its
test suite too.

## Dependencies

None. Existing built-in package binding and Pi file materialization already
exist on the source baseline. Recheck their current main revisions first.

## Risks

The main risks are premature evidence, cross-process reuse, partial-package
acceptance, and observations that imply provider health. Preserve the existing
file ownership fence and configuration/start lock ordering.

## Parallelism

`sequential`

## Inputs

- [Issue #5090](https://github.com/kdlbs/kandev/issues/5090).
- Requirement and design references in frontmatter.
- [ADR 0020](../../decisions/0020-pi-project-mcp-config-injection.md).
- [Pi ownership decision](../../decisions/2026-10-08-pi-project-mcp-ownership.md).
- `acp/builtin_mcp_test.go` and `adapter_session_resume_test.go`.
- `process/manager_permission_acp_test.go::newPermissionACPFlow`.
- Lifecycle project materialization and shared-file ownership tests.
- `apps/backend/AGENTS.md`, `agentctl/AGENTS.md`, and the backend TDD reference.

## Results

Implemented on 2026-11-05.

- `agentctl/types`: added `ProjectMCPDelivery`, its version/capability/strategy
  constants, clone, structural validation, and exact package match.
- `mcp/profile`: added `BuiltinBindingDigest` using the existing canonical
  binding serialization; `mcpconfig` exposes only `PiStrategy` as a project
  delivery producer.
- Lifecycle: `projectMCPDeliveryEvidence` binds the pinned helper instance,
  canonical workspace, task, binding digest, and exact injected packages.
  `materializeRuntimeProjectMCPWithPreparation` clears evidence on every
  skipped or failed path and records it only after the writer reports that the
  generated files were actually materialized. Rollback and cleanup discard it.
- Handoff: `configureAndStartAgentWithPreparedArgs` validates the record against
  the pinned helper, negotiates `project-mcp-delivery-v1`, and sends it through
  the new authenticated configure carrier.
- Helper: the process manager validates instance, task, workspace, binding
  digest, and package membership under the existing configure/start lock,
  clears evidence when the record is omitted, consumes it for exactly one
  adapter/process generation, and retains the immutable clone on that adapter
  for same-process load/resume/reset.
- Remote targets: the lifecycle asks the helper for its own workspace and
  loopback MCP URL instead of reading `standalonePort`, materializes generated
  files through the authenticated project-file operation with `os.Root`
  containment, verifies read-back entries/fingerprints, and cleans the claims
  on stop.
  Local/worktree targets keep the existing backend writer.
- Kubernetes restart: the staged replacement client prepares the project file
  and configures the delivery record before starting the agent.
- ACP: admission accepts ACP delivery or validated project delivery per
  package; new, load, resume, and reset share the same decision. Project
  delivery publishes deduplicated `project_config_materialized` evidence and
  never adds ACP wire entries or connection-health observations.

Regressions delivered:

- `acp/builtin_mcp_test.go`: `TestBuiltinSessionRequestsAcceptPreparedProjectDelivery`,
  `TestBuiltinSessionRequestsRejectPartialProjectDelivery`,
  `TestBuiltinProjectDeliveryEvidenceIsNotConnectionHealth`.
- `process/project_mcp_delivery_test.go`:
  `TestProjectMCPDeliveryRejectsMismatchedConfiguration`,
  `TestPiProjectMCPDeliveryACPFlow`.
- `runtime/agentctl/project_mcp_delivery_test.go`:
  `TestProjectMCPDeliveryRequiresHelperCapability`,
  `TestProjectMCPDeliveryCapabilityMissingIdentityEndpoint`,
  `TestConfigureAgentWithProjectDeliveryRoundTrip`,
  `TestProjectMCPDeliveryCloneIsolation`.
- `api/project_mcp_delivery_test.go`:
  `TestHandleAgentConfigureProjectMCPDelivery`.
- `agentctl/types/project_mcp_delivery_test.go`:
  `TestProjectMCPDeliveryValidation`, `TestProjectMCPDeliveryCloneIsolation`,
  `TestCanonicalProjectMCPDeliveryWorkspace`.
- `process/project_mcp_files_test.go`:
  `TestProjectMCPFilesOwnerPrepareMergeAndCleanup`,
  `TestProjectMCPFilesOwnerPreservesUserFile`,
  `TestProjectMCPFilesOwnerRejectsStaleGenerationAndEscape`.
- `agent/runtime/agentctl/client_project_mcp_test.go`:
  `TestProjectMCPClientRoundTrip`, `TestProjectMCPClientReportsUnsupportedAndConflict`.
- `api/project_mcp_test.go`:
  `TestProjectMCPRoutesMaterializeOnHelper`, `TestProjectMCPRoutesRequireAuthentication`.
- `lifecycle/manager_project_mcp_test.go`:
  `TestProjectMCPDeliveryPreparedOnlyAfterSuccessfulWrite`,
  `TestPromoteWorkspaceExecutionPreparesProjectDelivery`,
  `TestProjectMCPDeliveryHandoffRejectsReplacedHelper`.
- `lifecycle/project_mcp_delivery_test.go`:
  `TestProjectMCPDeliveryConfigureHandoff`,
  `TestProjectMCPDeliveryConfigureFailsBeforeOldHelper`.
- `lifecycle/project_mcp_files_test.go`:
  `TestRemoteProjectMCPDeliveryUsesExecutorLocalFilesystem`,
  `TestRemoteProjectMCPDeliveryRequiresHelperSupport`,
  `TestKubernetesRestartConfiguresProjectDelivery`,
  `TestKubernetesRestartRollsBackProjectDeliveryOnFailure`,
  `TestProjectMCPCleanupDoesNotReacquireLifecycleLock`,
  `TestProjectMCPDeliveryHandoffAcceptsLongWorkspace`.
- `process/project_mcp_delivery_test.go`:
  `TestProjectMCPDeliveryConsumedByProcessGeneration`,
  `TestProjectMCPDeliveryAcceptsNestedWorkspacePath`.

Verification:

- `go test -trimpath -tags fts5 ./internal/agentctl/types ./internal/agentctl/server/adapter ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/mcpconfig ./internal/mcp/profile ./internal/agent/runtime/lifecycle`: passed.
- Remote producer/configure fixture with zero `StandalonePort`, a separate
  backend/target filesystem, exact package URLs, and Kubernetes restart
  handoff: passed.
- Process-generation consumption, stop/start without configure, and workspace
  paths beyond the short identity bound: passed.
- Targeted `-race` subset across ACP, process, client, and lifecycle: passed.
- `bash scripts/lint-go-changed <all changed and new Go files>`: 0 issues.
- Full backend `go build -trimpath -tags fts5 ./...`: passed.
- Documentation preflight: see `plan.md#documentation-preflight`.

A real Pi/Kubernetes smoke was not performed and is not claimed.
