---
created: 2026-09-28
status: completed
requirements:
  - REQ-AGENTS-MCP-BRIDGE-RELIABILITY-001
  - REQ-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001
  - REQ-TASKS-RUNTIME-CLEANUP-001
system_design:
  - ../../specs/agents/system-design/mcp-bridge-reliability.md
  - ../../specs/platform/system-design/runtime-failure-attribution.md
  - ../../specs/tasks/system-design/runtime-cleanup.md
legacy_specs: []
---

# Implementation Plan: Startup Log Corrections

## Overview

Correct Git inspection failure handling, wire MCP before activating consumers,
and explain startup recovery omissions. Implement the three work orders
sequentially. They have independent validation boundaries and no schema changes.
Production and permanent test changes are implemented and verified.
The user confirmed code/diagnostic repair only and preservation of blocked
worktrees; repair of the four live cleanup jobs is excluded.

## Scope

### In scope

- Preserve Git execution errors and attribute cleanup refusals without weakening
  worktree ownership or immutable-commit checks.
- Complete MCP dispatcher and execution-scope wiring before recovery or startup
  launches can issue discovery requests.
- Carry evidence from successful or failed runtime enumeration into the recovery
  summary, preserving current progress and recovery policy.

### Out of scope

- Listener/authentication settings and the Office CEO permission/stall issue.
- Plugin manifest upgrades, JWT configuration, duplicate skills, Git monitor
  noise, invalid workflow input, and browser/GitHub request failures.
- Mutating the user's database, task directories, branches, or cleanup snapshots.
- Auto-adopting competing worktrees, refreshing audited branch commits, relaxing
  deletion checks, suppressing the recovery progress warning, or new UI.

## Evidence and root causes

Evidence: retained `~/.kandev/logs/backend-logs.log`, September 28 startup
10:31:22 through 10:43 local time. Startup became ready at 10:32:26.

1. Four cleanup jobs failed repeatedly due to competing Git metadata, another
   task's ownership, a changed audited commit, and a Git subprocess ENOENT.
   Existing `resource_cleanup_jobs.go` already implements retry backoff and
   distinguishes bounded from cascade-critical retries. These refusals are
   safety behavior, not proof that automatic deletion is appropriate.
   `manager_git.go:branchExists` nevertheless turns every non-context Git error
   into `(false, nil)`, including failure to start Git or an invalid repository.
   The subsequent registration inspection can then obscure the first failure.
   `/usr/bin/git` exists on the inspected host; the log alone does not prove
   why that subprocess returned ENOENT, which can also arise from its cwd.
2. `provideAgentRuntime` calls `lifecycleMgr.Start` before gateway/router MCP
   registration, and `startGatewayAndServe` starts the orchestrator and scheduler
   before `buildHTTPServer` installs handlers. Two `mcp.list_plugin_tools`
   requests failed at 10:31:57 and 10:32:11; registration occurred at 10:32:26.
   The dynamic proxy repairs later calls but cannot fix the first discovery.
3. Recovery totals count persisted candidates; `RecoverInstances` returns only
   revived instances. A successful empty standalone enumeration at 10:31:30
   leaves 17 omitted records classified unknown by Manager. Existing tests
   explicitly preserve 0/N progress and unknown fallback. Add the missing
   evidence channel; do not infer a failed restoration or fabricate N/N progress.

## Ownership and contract reconciliation

Agents owns the MCP bridge; its existing requirement gains AC-001.8 for initial
startup dependency ordering. Platform owns bounded diagnostics; attribution gains
AC-001.5 and AC-001.6 for enumeration and cleanup inspection evidence. Tasks
retains cleanup authority under AC-TASKS-RUNTIME-CLEANUP-001.9 and .32.
No new cleanup state, retry policy, public API, or persistence boundary is needed.
The active attribution design expressly preserves the below-total warning, so
this package fixes the explanation rather than changing that contract.

The earlier [runtime log remediation](../recent-runtime-log-remediation/plan.md)
remains an implemented historical package. This follow-up extends its diagnostic
result; it does not reopen its task statuses or repeat its other changes.

## Technical approach

### Cleanup

Correct `Manager.branchExists` using quiet ref verification and explicit error
classification. Add typed audit stage/reason at branch and registration checks,
then include those fields in existing worker warnings. Preserve wrapped errors,
unchanged snapshots, retry backoff, and every deletion guard. Use temporary real
Git fixtures for missing ref, invalid/missing cwd, changed branch, and competing
registration, plus controlled subprocess failures.

### Startup MCP wiring

Separate backend construction/wiring from activation in `main.go` and
`helpers.go`. Construct the gateway, Office dependencies, system/storage services,
and application router before any recovery stream or task-launch producer runs.
Install handlers and scope/principal callbacks once. Subscribe the event watcher
before lifecycle recovery, then start the orchestrator and launch producers.
Keep the early bootstrap listener, required-store accounting, retained-outcome
publication, final persistence gate, schema-version recording, and readiness
handoff. Move launch-capable side effects out of constructors as needed; do not
wait on a dispatcher from a recovery call that prevents registration completing.

### Recovery diagnostics

Add a per-pass detailed recovery result behind an optional backend interface.
Implement it in StandaloneExecutor; retain the existing interface fallback for
other backends. Registry merges only matching backend/input identities. Manager
combines producer evidence with reconstruction outcomes exactly once per record.
Report `no_matching_instance` only after successful enumeration, and report
`enumeration_failed` separately. Preserve unknown for evidence-free omissions.
Do not log record keys, advance progress, or mutate recovery policy.

| Boundary | Evidence and behavior | Unsupported/failure fallback |
| --- | --- | --- |
| Standalone ACP recovery | Current agentctl inventory correlated by session; report no match or enumeration failure | Ambiguous or unclassified records stay unknown |
| Plugin remote recovery | Existing provider recovery and Manager reconstruction | No standalone inference; omitted record stays unknown |
| Other ExecutorBackend implementations | Existing return contract remains valid | No new recovery support implied |
| New and recovered MCP streams | Same complete dispatcher, execution principal, and scope | Genuine missing dependency retains correlated error |
| External MCP | Same handler registry after readiness | Existing auth and tool policy unchanged |

## Tests

| Work order | Proposed regression | Criteria |
| --- | --- | --- |
| 01 | `manager_cleanup_inspection_test.go`: `TestBranchExistsDistinguishesMissingRefFromInspectionFailure`, `TestCleanupInspectionFailurePreservesSnapshot` | AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.6; AC-TASKS-RUNTIME-CLEANUP-001.9/.32 |
| 02 | `startup_mcp_order_test.go`: `TestStartupMCPReadyBeforeRecoveryAndLaunch`, `TestStartupMCPWiringFailurePreventsConsumers` | AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.1/.3/.8 |
| 03 | `manager_recovery_outcomes_test.go`: `TestRecoverySummaryDistinguishesEmptyEnumerationFromFailure`, `TestRecoverySummaryMixedBackendsAccountsEachRecordOnce` | AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.4/.5 |

Regression names are planned tests, not already passing evidence. Use production
composition seams and real temporary Git repos; avoid tests that only mirror a
new helper. MCP end-to-end evidence runs a real dispatcher request through the
execution-bound proxy from recovery/launch callbacks in the startup fixture.
No rendered interaction changes require a browser E2E test.

## Work orders

- [x] [Task 01: Cleanup inspection failures](task-01-cleanup-inspection.md)
- [x] [Task 02: MCP startup ordering](task-02-mcp-startup-order.md)
- [x] [Task 03: Recovery outcome attribution](task-03-recovery-outcomes.md)

## Verification results

Implementation verification completed on 2026-09-28:

- `go test -race ./internal/worktree ./internal/task/service -run 'BranchExists|CleanupInspection|RetryTaskResourceCleanupJob|RetryCascadeCleanupJob|MissingWorktree' -count=1`: passed.
- `go test -race ./internal/backendapp ./internal/agent/runtime/lifecycle -run 'StartupMCP|StartOrchestrator|Bootstrap|MCPHandler|Recovery' -count=1`: passed.
- `go test -race ./internal/agent/runtime/lifecycle -run 'RecoverySummary|RecoverAll|StandaloneExecutorRecoverInstances|ManagerStart|RecoveryGuard' -count=1`: passed.
- Full package test suites for `internal/backendapp`, `internal/agent/runtime/lifecycle`, `internal/agent/runtime/agentctl`, and `internal/startup`: passed.
- `python3 scripts/list-docs.py validate`: passed, 321 decisions and 1220 specs.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.

## Risks

- Startup composition includes workers and cleanup callbacks; reordering must
  preserve cancellation, restore quiescing, event subscribers, and single starts.
- `branchExists` has non-cleanup callers. Missing-ref semantics must remain
  compatible while operational errors become visible rather than false absence.
- Successful enumeration with no matching instance is not proof of OS process
  death and must never authorize cleanup.
- Existing blocked cleanup jobs remain blocked until their ownership evidence
  can be reconciled. This package neither force-completes nor deletes them.
