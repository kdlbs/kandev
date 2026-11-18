---
id: "02-retained-runtime-admission"
title: "Enforce the boundary on retained execution"
status: done
wave: 2
depends_on:
  - "01-runtime-admission"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-RUNTIME-SELECTION-002
  - REQ-EXECUTORS-OWNERSHIP-004
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003
acceptance_criteria:
  - AC-EXECUTORS-RUNTIME-SELECTION-002.1
  - AC-EXECUTORS-RUNTIME-SELECTION-002.2
  - AC-EXECUTORS-RUNTIME-SELECTION-002.3
  - AC-EXECUTORS-RUNTIME-SELECTION-002.4
  - AC-EXECUTORS-OWNERSHIP-004.1
  - AC-EXECUTORS-OWNERSHIP-004.2
  - AC-EXECUTORS-OWNERSHIP-004.3
  - AC-EXECUTORS-OWNERSHIP-004.4
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.3
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.4
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003.1
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003.2
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003.3
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003.4
system_design:
  - ../../specs/executors/system-design/runtime-selection.md
  - ../../specs/executors/system-design/per-user-ownership.md
  - ../../specs/tasks/system-design/additional-session-workspace-reuse.md
---

# Task 02: Enforce the boundary on retained execution

## Summary

Refuse new effects on an execution whose actual runtime cannot satisfy its admitted selection.
Preserve observation, history, workspace resources, and exact authorized Stop.

## In scope

- Add `ValidateExecutionRuntime` reuse validation for cached actual runtime, matching environment inventory, and Docker identity.
- Guard all session/environment cache returns and coalesced results using the requesting session's resolved selection.
- Guard promotion before MCP or profile mutation. Preserve #5162's concrete-profile persistence when present in the refreshed base.
- Check resume before stale-runtime cleanup and before credential lease issuance.
  A mismatch refusal must not enter replacement, generic cleanup, or environment-retirement branches.
- Guard direct `StartAgentProcess` and `PromptAgent` before new authenticated effects.
- Guard `EnsurePassthroughExecution`, `ResumePassthroughSession`, and shared passthrough startup, including the ready-process fast return.
- Keep authenticated recovery truthfully observable while suppressing new-work availability for a mismatch.
  Preserve journal, SQL generation, stream, active-submission, and workspace-source evidence from the retained owner.
- Keep Stop routed by actual runtime/resource identity without requiring selected-runtime availability.
- Preserve #5175 environment selection, durable child claims, exact predecessor retirement, and Kubernetes ownership rules.
  Registration admission precedes its claim. Runtime comparison applies to reuse candidates in the selected binding.
  A policy-proven predecessor is not a candidate for reuse on the new runtime.
  Same-binding Docker/standalone mismatch and unknown environment identity must not become transition authority.

## Out of scope

Automatic migrations, worker termination, environment retirement, new actor attribution, journal recovery redesign, or unrelated source/worktree repair.
PRs #5176 and #5184 own their actor changes.

## Acceptance

1. Cache, ensure, promotion, resume, startup, and prompt routes refuse selected Docker with actual standalone or unproven Docker identity.
   Refusal occurs before credentials, cleanup, MCP mutation, or process/prompt admission, including concurrent ensure/launch callers.
2. Refusal leaves runtime identity, files, repository inventory, history, and unrelated retained workers unchanged.
   A healthy/mismatched mixed inventory never uses one sibling to authorize or invalidate another execution.
3. An authorized actor can observe and Stop the exact mismatched execution. Unauthorized, deleted-actor, and canceled requests gain no access or fallback authority.
   Valid policy-authorized executor transitions retain their exact predecessor retirement without adopting that predecessor as the new runtime.

## TDD and verification

Create `TestExecutorRuntimeReuseAdmission`, `TestExecutorRuntimeResumeAdmission`, `TestExecutorRuntimeMismatchPreservesStop`, and `TestExecutorRuntimeRecoveryObservation`.
Add `TestExecutorRuntimePassthroughAdmission` for cached-ready, cached-stopped, and cold terminal routes.
Include memory and real temporary SQLite cases. Use distinct file/identity sentinels for selected and sibling resources.
Include a valid inherited transition, a same-binding fallback worker, and an unproven predecessor as three separate controls.
Use channel barriers for promotion/coalescing, cancellation, and execution rotation. Join all started goroutines.
The base must fail the mismatch-refusal cases. Existing valid standalone and Docker reuse controls must remain green.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -run '^TestExecutorRuntime(ReuseAdmission|MismatchPreservesStop|RecoveryObservation|PassthroughAdmission)$' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator/executor -run '^TestExecutorRuntimeResumeAdmission$' -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle ./internal/orchestrator/executor -run 'ExecutorRuntime(ReuseAdmission|ResumeAdmission|MismatchPreservesStop|RecoveryObservation|PassthroughAdmission)|Inherited|ExecutorMismatch|WorkspacePromotion|StandaloneRetained|AdoptedStandalone|DockerReconnect' -count=1)
```

Run `bash scripts/lint-go-changed` from the repository root with the actual changed Go paths.
Record actual inherited/transition controls selected on the refreshed base, including #5175 cases if merged.
Do not mark a filter with no discovered tests as successful coverage.

## Files likely touched

- Lifecycle `manager_workspace_admission.go`, `manager_execution.go`, `manager_launch.go`, `recovery_launch_coalescing.go`.
- Lifecycle `manager_startup.go`, `manager_interaction.go`, `manager_lifecycle.go`, `manager_recovery_turn_outcome.go`.
- Lifecycle `manager_passthrough.go` and its shared start helper, if separate from those entry points.
- New lifecycle `manager_runtime_reuse_test.go` and focused recovery/Stop tests.
- Orchestrator executor `executor_resume.go`, `executor_environment_reuse.go`, `executor_execute.go`.
- If #5175 is present, `executor_inherited_env_transition.go` only for admission ordering and compatibility controls.
- New orchestrator executor `executor_runtime_resume_test.go`.
- Existing admission/error helper from task 01.
- `apps/backend/internal/backendapp/adapters.go` for reuse-validator forwarding through `lifecycleAdapter`.

## Dependencies

Task 01. Refresh adjacent heads before editing shared launch, resume, or promotion functions.

## Risks

Selection validation on Stop would strand historical fallback workers.
Running validation after `cleanupStaleExecutionBeforeResume` can destroy evidence or unrelated retained state.
Returning a shared environment execution without the requesting selection can reintroduce the bypass.
Passive recovery and prompt admission must remain separate authorities.

## Parallelism

`sequential`

## Inputs

- Runtime-selection design: Retained execution; Recovery and Stop.
- Existing active ownership and additional-session workspace criteria listed in frontmatter.
- Merged standalone journal recovery plan and current generation-fenced environment contracts.
- Current `manager_execution_test.go`, `manager_launch_workspace_test.go`, recovery, and executor environment-reuse test patterns.

## Results

Implemented. Retained executions are validated against the requesting selection before new work.

- Added `Manager.ValidateExecutionRuntime`, which compares the in-memory execution and durable
  `executors_running` runtime with the admitted selection. Docker requires a nonempty container ID.
- Applied the check to session/environment cache returns, coalesced ensure/launch results,
  workspace promotion, resume preflight before stale-runtime cleanup, `StartAgentProcess`,
  prompt and steer admission, and passthrough reuse/start.
- Recovery keeps authenticated observation but suppresses availability publication for a runtime
  mismatch. Stop and read-only inventory continue to use the recorded resource identity and do not
  require selected-runtime admission.
- Refusal never removes the retained execution, rotates its ID, relabels it, or creates a
  replacement. Registration admission precedes resume cleanup and credential leases.
- Cold session/environment ensure and cold launch compare durable inventory before workspace
  reconciliation, preparation secrets, or backend allocation. Inventory read failures and
  canceled reads fail closed; only authoritative not-found is a fresh session. Durable evidence
  must describe the exact candidate execution, and unknown remote runtime evidence is refused.

Verification: the new lifecycle reuse, mismatch/Stop, recovery-observation, passthrough, cold
ensure/launch, inventory-read-failure, and unknown-remote-runtime tests passed, along with
`TestExecutorRuntimeResumeAdmission` in the orchestrator executor package. The
full lifecycle, orchestrator/executor, orchestrator, and backendapp suites passed, including the
existing inherited-environment and workspace-promotion regressions. `golangci-lint` reported 0
issues for the changed packages.
