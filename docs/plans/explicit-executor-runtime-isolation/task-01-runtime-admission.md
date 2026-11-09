---
id: "01-runtime-admission"
title: "Reject unavailable required runtimes before effects"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-RUNTIME-SELECTION-001
  - REQ-EXECUTORS-OWNERSHIP-004
acceptance_criteria:
  - AC-EXECUTORS-RUNTIME-SELECTION-001.1
  - AC-EXECUTORS-RUNTIME-SELECTION-001.2
  - AC-EXECUTORS-RUNTIME-SELECTION-001.3
  - AC-EXECUTORS-RUNTIME-SELECTION-001.4
  - AC-EXECUTORS-RUNTIME-SELECTION-001.5
  - AC-EXECUTORS-OWNERSHIP-004.1
  - AC-EXECUTORS-OWNERSHIP-004.3
system_design:
  - ../../specs/executors/system-design/runtime-selection.md
  - ../../specs/executors/system-design/per-user-ownership.md
---

# Task 01: Reject unavailable required runtimes before effects

## Summary

Make concrete runtime selection fail closed and integrate early admission with fresh agent and workspace preparation.
Keep default selection and actor authorization intact.

## In scope

- Refresh main, #5175, #5162, #5176, and #5184 heads against the plan snapshots.
- Add typed runtime refusal errors with safe bounded reasons and no credential or endpoint contents.
- Make `getExecutorBackend` require the mapped backend for nonempty selection under allow, warn, deny, and unset internal policy.
- Keep empty/default and Local/Worktree/mock controls. Keep constructor shape and document the restricted fallback-policy semantics.
- Preserve concrete executor/profile intent on lookup errors. An unresolved ID with an empty type cannot reach default standalone.
- Add `ValidateExecutorRuntime` registration admission through the existing narrow capability seam and required production wiring.
- Cover `orchestrator/service.go` capability installation and the real agent-manager adapter. Missing validation must fail closed for concrete selection.
- Place registration checks after actor authorization, before preparation credentials, scripts, workspace mutation, and runtime allocation.
- If the refreshed base includes #5175, refuse unavailable runtime before `claimInheritedEnvironmentTransition` changes bindings.
- Cover `prepareSessionAttempt`, `buildLaunchAgentRequest`, lifecycle `Launch`, and workspace-only creation.
  Task 02 owns retained identity and resume cleanup integration.
- Check returned backend/instance agreement and Docker container identity before readiness and agent start.
- Route typed failures through existing generic launch-failure cause/details framing. Retry keeps the same executor.

## Out of scope

Retained execution correction, fallback configuration APIs, provider mapping changes, Docker deployment repair, new UI, or secret-sharing changes.

## Acceptance

1. Explicit missing Docker refuses under every policy with zero credential resolutions, scripts, standalone creates, authenticated workers, and prompts.
   Both repository-free and repository-backed preparation use this guard.
2. Registered Docker client/daemon/create failures never call standalone. Healthy Docker needs consistent actual runtime and a container identity.
3. Unspecified/local/worktree and unknown/plugin controls retain their expected behavior. Unauthorized callers and invalid durable actors fail before protected details or effects.
   Missing concrete IDs and profile/storage lookup errors cannot become unspecified selection.

## TDD and verification

Create `TestExecutorRuntimeSelectionPolicies`, `TestExplicitRuntimeAdmissionBeforeEffects`, `TestExplicitDockerCreationFailureDoesNotUseStandalone`, and `TestDockerRuntimeIdentityRequired`.
The missing-Docker expected-refusal cases must fail on the current base for the observed fallback reason.
Use counting fakes through real producer/wiring seams, not only direct selector assertions.
Provider-failure controls can already pass before the correction. Record that distinction.
Read `.agents/skills/tdd/references/backend-tests.md` before implementation.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -run '^(TestExecutorRuntimeSelectionPolicies|TestExplicitDockerCreationFailureDoesNotUseStandalone|TestDockerRuntimeIdentityRequired)$' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator/executor -run '^TestExplicitRuntimeAdmissionBeforeEffects$' -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle ./internal/orchestrator/executor ./internal/backendapp -run 'ExecutorRuntimeSelection|ExplicitRuntimeAdmission|ExplicitDockerCreation|DockerRuntimeIdentity|RuntimeAdmissionWiring' -count=1)
```

Add `TestRuntimeAdmissionWiring` in backendapp to prove production uses the required validator and denial policy.
Run `bash scripts/lint-go-changed` from the repository root with the actual changed Go paths before completing this work order.
Record the expanded command and result in Results.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_executor.go`, `manager.go`, `manager_capabilities.go`.
- New `manager_runtime_selection_test.go` and a small runtime-admission/error file in that lifecycle package.
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`, `manager_execution.go`.
- `apps/backend/internal/orchestrator/executor/executor.go`, `executor_execute.go`, `launch_failure.go`.
- `apps/backend/internal/orchestrator/executor/executor_state.go`, only if existing admission does not preserve unresolved selection intent.
- New `apps/backend/internal/orchestrator/executor/executor_runtime_selection_test.go`.
- `apps/backend/internal/backendapp/agents.go`, `adapters.go`, and a focused wiring test.
- `apps/backend/internal/orchestrator/service.go` and the current agent-manager adapter's capability forwarding.
- Existing capability test doubles that must implement the required method.

## Dependencies

None. Adjacent PRs are overlap constraints, not prerequisites for this correction.

## Risks

The lifecycle lookup is currently too late to prevent orchestrator credential resolution.
Changing only production policy or the selector does not meet the zero-effect condition.
Keep unauthorized/not-found behavior ahead of runtime diagnostics.
Do not grow already oversized test files. Add focused files.

## Parallelism

`sequential`

## Inputs

- Runtime-selection design: Selection and admission; Failure channels.
- Ownership design: Admission and execution.
- `manager_admission_test.go`, `executor_docker_test.go`, and existing orchestrator ownership tests.
- Plan evidence and compatibility matrix.

## Results

Implemented. Required-runtime admission now fails closed for every internal fallback policy.

- Added typed refusal sentinels and a bounded `ExecutorRuntimeError` in
  `apps/backend/internal/agent/runtime/lifecycle/manager_runtime_selection.go`.
- `Manager.getExecutorBackend` requires the mapped backend for every nonempty selection.
  The `ExecutorFallback*` constants remain for source compatibility but no longer authorize
  replacement. Production wiring uses `ExecutorFallbackDeny`.
- Added `ValidateExecutorRuntime` and exposed it through the orchestrator capability seam.
  Registration admission runs after actor authorization and before credentials, scripts,
  workspace mutation, and runtime allocation in `prepareSessionAttempt`,
  `buildLaunchAgentRequest`, lifecycle `Launch`, and `launchInternal`.
- Added created-instance validation for backend/runtime agreement and Docker container identity
  before registration or agent startup. Failed creation rolls back only the created resource.
- Preserved unresolved executor intent: a failed executor lookup with no resolvable type is
  refused instead of falling back to standalone.
- Added safe launch-failure classification for unavailable, mismatched, unverified, and
  unresolved selections. Retry keeps the same executor.
- Prepared-workspace launches now perform request-scoped retained admission before profile
  replacement cleanup, environment recovery, credential resolution, existing-workspace success,
  or MCP/env updates. A failure to resolve an explicitly selected profile's parent executor is
  never allowed to retain the workspace-default host runtime.

Verification:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -run '^(TestExecutorRuntimeSelectionPolicies|TestExplicitDockerCreationFailureDoesNotUseStandalone|TestDockerRuntimeIdentityRequired)$' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator/executor -run '^TestExplicitRuntimeAdmissionBeforeEffects$' -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle ./internal/orchestrator/executor ./internal/backendapp -run 'ExecutorRuntimeSelection|ExplicitRuntimeAdmission|ExplicitDockerCreation|DockerRuntimeIdentity|RuntimeAdmissionWiring' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/backendapp -run '^TestRuntimeAdmissionWiring$' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator/executor -run '^(TestLaunchPreparedSessionRejectsRetainedRuntimeBeforeEffects|TestExplicitExecutorProfileParentFailureIsUnresolved)$' -count=1)
bash scripts/lint-go-changed <changed Go paths>
```

All commands passed. `golangci-lint` reported 0 issues for the changed packages.
