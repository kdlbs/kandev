---
id: "02-propagate-providers-through-agent-launch"
title: "Propagate providers through agent launch"
status: done
wave: 2
depends_on: ["01-scope-task-mcp-tools-by-provider"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/provider-aware-review-automation.md"
---

# Task 02: Propagate providers through agent launch

## Acceptance

- Launch and resume derive a normalized union from every task repository entity,
  independent of primary repository and ordering.
- The provider union crosses executor, backendapp, lifecycle, and every agentctl
  executor mapping without inference from filesystem remotes.
- Mapping tests cover GitHub-only, GitLab-only, mixed, empty/unsupported, and all
  supported executor backends.

## Verification

- `cd apps/backend && go test -race ./internal/orchestrator/executor ./internal/backendapp ./internal/agent/runtime/lifecycle`

Write derivation and mapping regression tests first. At least one seam test must
fail because current launch/config types do not carry provider capabilities.

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/backendapp/adapters.go`
- `apps/backend/internal/agent/runtime/lifecycle/types.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_backend.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_standalone.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_docker.go`
- `apps/backend/internal/agent/runtime/lifecycle/container.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_sprites_operations.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_operations.go`
- Adjacent `*_test.go` mapping tests

## Dependencies

Task 01 defines the agentctl and MCP provider contract this task transports.

## Parallelism

Sequential after Task 01. Do not overlap with Task 03 because both change
lifecycle provider plumbing.

## Inputs

- Existing `resolveAllRepoInfo` and `buildLaunchAgentRequest` paths
- Spec scenario mapping for single, mixed, and unsupported providers
- ADR decision items 1-3

## Risks

- Missing one executor mapping can make behavior environment-dependent. Test the
  common request field at each construction seam.
- Preserve repository order for existing workspace behavior while treating the
  provider value itself as a set.

## Output contract

Report the RED evidence, derivation helper and normalization ownership, mappings
covered, files changed, exact verification result, and risks. Mark this task
`done` and update its plan checkbox in the same conversation.

## Result

- RED evidence: the derivation regression initially failed to compile because
  `deriveMCPProviders` did not exist; the mapping regression then failed because
  standalone, container, and SSH request-builder seams did not exist.
- Added `deriveMCPProviders`, which reads only persisted repository entities and
  delegates canonical filtering/order to `internal/mcp/providers.Normalize`.
  Launch and resume now derive the union from every resolved task repository.
- Propagated `McpProviders` through executor, backendapp, lifecycle launch and
  executor-create requests, then through standalone, Docker/container, Sprites,
  and SSH agentctl mappings. No filesystem remote inference was added.
- Added table-driven derivation, initial/resume mixed-repository, backendapp,
  lifecycle manager, and all-agentctl-mapping tests covering GitHub-only,
  GitLab-only, mixed, empty, and unsupported inputs.
- Verification: `cd apps/backend && go test -race ./internal/orchestrator/executor ./internal/backendapp ./internal/agent/runtime/lifecycle`
  passed (1621 tests across 3 packages).
- Review remediation: promotion of an existing workspace-only execution now
  applies non-empty provider capabilities through its live agentctl endpoint
  before command promotion; a rejected update fails launch and leaves the
  execution unpromoted. Verification: `cd apps/backend && go test -race
  ./internal/agent/runtime/lifecycle` (1045 tests passed).
- Risk: future executor mappings must copy the shared typed field; the lifecycle
  mapping test covers every current supported agentctl backend.
