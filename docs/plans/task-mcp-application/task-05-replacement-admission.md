---
id: "05-replacement-admission"
title: "Separate provider refresh from workspace lifetime"
status: done
wave: 2
depends_on:
  - 01-save-intent
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-APPLY-001
  - REQ-AGENTS-MCP-APPLY-003
acceptance_criteria:
  - AC-AGENTS-MCP-APPLY-001.2
  - AC-AGENTS-MCP-APPLY-001.3
  - AC-AGENTS-MCP-APPLY-001.6
  - AC-AGENTS-MCP-APPLY-003.1
  - AC-AGENTS-MCP-APPLY-003.2
system_design:
  - ../../specs/agents/system-design/task-mcp-application.md
---

# Task 05: Separate provider refresh from workspace lifetime

## Summary

Add provider-only stop/start operations inside a retained agentctl instance.
This revised work order replaces the unimplemented empty-helper admission proposal. Its ID remains stable.

## In scope

Separate provider cancellation, workers, channels, configuration, and process groups from instance-owned resources.
Preserve terminals, previews, workspace processes, VS Code, LSP, trackers, control credentials, and the journal during refresh.
Add provider-operation admission and generation checks without closing instance admission.
Keep ordinary full Stop and teardown semantics for existing callers.
Do not enable automatic MCP replacement until work orders 06, 02, and 03 complete.

## Out of scope

Deleting helpers, moving live shells between instances, waiting for independent workspace resources to close, or changing public task deletion behavior.

## Acceptance

1. A provider-only stop/start preserves resource IDs, PIDs, PTYs, output buffers, preview URLs, and established connections. New workspace operations continue during refresh.
2. Only provider-owned active work defers refresh. Tests prove that the old provider group exits and independent groups survive on supported platforms.
3. Concurrent user Stop wins over provider activation. A stale exit, configure, or start cannot affect a successor or reopen stopped instance admission.

## Verification

Write failing contract tests before changing shared lifecycle code. Run from repository root.

```bash
(cd apps/backend && go test -race ./internal/agentctl/server/process ./internal/agentctl/server/instance ./internal/agentctl/server/api -run 'Test(MCPProviderReplacement|StopInstance|ManagedStartAdmission|ManagerWorkspaceRestart)' -count=1)
bash scripts/lint-go-changed $(git ls-files --modified --others --exclude-standard -- '*.go')
```

Add `server/process/provider_replacement_test.go` with `TestMCPProviderReplacementPreservesWorkspace`,
`TestMCPProviderReplacementConcurrentWorkspaceStart`, and `TestMCPProviderReplacementStopWins`.
Use real disposable shell/preview fixtures with deterministic barriers. Do not count reconnecting as preservation.
Read and write through an existing PTY and keep a preview connection usable during and after provider-start failure.
Test dev-server and LSP ownership independently from provider descendants.
Keep Unix process-group and Windows job-object coverage in their supported platform test lanes.
Audit every exit/cleanup branch, including failed start, late old events, and tracker renewal.

## Files likely touched

- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/process/provider_replacement.go` (new)
- `apps/backend/internal/agentctl/server/process/provider_replacement_test.go` (new)
- `apps/backend/internal/agentctl/server/process/` (provider configuration, adapters, exit workers, and process ownership)
- `apps/backend/internal/agentctl/server/api/` (provider operation integration)
- `apps/backend/internal/agentctl/server/instance/manager.go` (full teardown interlock)

## Dependencies

01-save-intent. Work order 06 adds the authority and durable protocol for these lifecycle operations.

## Risks

Existing Start renews workspace trackers, and Configure mutates shared environment state.
Skipping shell cleanup alone does not create a safe provider-only lifecycle.
Provider-owned background processes must not be mistaken for independent workspace processes.

## Parallelism

Sequential. No delegation is authorized.

## Inputs

- [Requirements](../../specs/agents/requirements/task-mcp-application.md).
- [Resource ownership and protocol](../../specs/agents/system-design/task-mcp-application.md#safe-replacement-and-resource-ownership).
- [Lifetime decision](../../decisions/2026-11-15-provider-refresh-workspace-lifetime.md).
- `Manager.Stop`, `StopForTeardown`, `startWithGeneration`, `configure`, and workspace restart fixtures.

## Results

Provider-only stop, configuration, start, admission, workload proof, and process events are separated from workspace lifetime. Provider replacement retains the same manager, execution, journal owner, workspace environment, trackers, shells, preview, and protocol subprocesses. Full Stop and teardown keep their existing complete cleanup.

Focused race tests cover established resources, workspace work during replacement, concurrent starts, failed spawn, stale configuration, user Stop, scoped ordinary-start rejection, complete reaping, and error/complete suppression outside a user turn. The permanent native conformance test also proves terminal output, preview content, and editor connection survival with the actual Codex binary.

The earlier full API/process audit has unrelated baseline failures in Git capture/numstat and LSP bridge tests. They reproduce with original process manager source. Those failures are not claimed as passing and are outside this work. Final focused checks and exact limitations are recorded in the plan.

Baseline audit failures reproduced with original manager source:

- API: `TestGitStatusHTTPReturnsRecoveredEnrichedCapture`, `TestGitStatusHTTPWarnsOnceAfterContinuousCaptureChanges`, and `TestGitStatusMultiRetriesOnlyTheRepositoryWithChangedEvidence`.
- API: `TestHandleLSPStreamBridgesFramesAndStopsOwnedProcess` and `TestHandleLSPStreamAutoInstallHandsFirstClientMessageToBridge`.
- Process: `TestWorkspaceGitMalformedNumstatDetailsUnavailable` (12 cases) and `TestWorkspaceGitEmptyNumstatDetailsReady`.

The isolated full instance suite passes. The final feature regression uses explicit test families and does not hide or fix these unrelated baseline failures.
