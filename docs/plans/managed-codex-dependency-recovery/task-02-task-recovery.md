---
id: "02-task-recovery"
title: "Recover task startup and resume once"
status: pending
wave: 2
depends_on:
  - "01-executor-local-preparation"
plan: "plan.md"
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-001
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-004
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-005
acceptance_criteria:
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.4
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.3
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.4
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.7
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-005.1
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-005.2
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-005.3
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-005.5
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-005.6
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-005.7
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-005.8
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-005.9
system_design:
  - ../../specs/agents/system-design/managed-npm-runtime-recovery.md
---

# Task 02: Recover task startup and resume once

## Summary

Connect verified missing-dependency evidence to the existing lifecycle startup
owner. Recover a new task or retained conversation once without replay,
changes to the selected ACP bridge, saved runtime selection, or package
identity, or disturbance to healthy siblings. Transitive `@openai/codex`
resolution may differ; report the actual resulting runtime observation.

## In scope

- Typed initialize-phase gating and agentctl private-preparation directive.
- Existing `beginStartupRecovery`, cleanup confirmation, backoff/deadline,
  event-generation fencing, and original conversation restoration.
- Accurate `managed_runtime_startup` dependency/preparation metadata through
  orchestration, using existing generic localized recovery presentation.
- Healthy/recovered/exhausted concurrent tasks and actual prompt-once proof.

## Out of scope

- Host/profiles, new frontend copy/controls, automatic fallback after session
  creation/load/resume, and an additional retry budget.

## Acceptance

1. `TestManagedDependencyTaskResume` first fails because no repair is
   attempted for a verified live-bridge initialize error, then passes with
   the original session, exact runtime selection, native token, and one prompt.
2. `TestManagedDependencyRetryBudget` covers cleanup refusal, installer
   failure, cancellation/deadline, ETARGET followed by missing dependency,
   unsupported peers/executors, and post-initialize errors with no third attempt.
3. `TestManagedDependencySiblingSafety` and
   `TestManagedDependencyFailureDetails` prove later reads by healthy siblings,
   stale-event fencing, honest attempts, and bounded sanitized final causes.

## Verification

```bash
(cd apps/backend && go test ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/agent/runtime/routingerr ./internal/orchestrator -run 'ManagedDependency|ManagedRuntime|ManagedStartup' -count=1)
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run 'ManagedDependency' -count=1)
make -C apps/backend build
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/managed_runtime_startup.go`.
- `apps/backend/internal/agent/runtime/lifecycle/session.go`, only for preserving typed initialize evidence.
- Existing `managed_runtime_startup_classification_test.go`, `managed_runtime_startup_recovery_test.go`, and new `managed_dependency_recovery_test.go`.
- `apps/backend/internal/agent/runtime/agentctl/client.go` and initialize-error typing.
- `apps/backend/internal/agent/runtime/routingerr/startup_error.go` and matching tests.
- `apps/backend/internal/orchestrator` startup failure projection and its managed-runtime tests.

## Dependencies

Task 01.

## Risks

- Do not count an npm installer as a second started agent process.
- A retry budget consumed by ETARGET cannot be consumed again by this category.
- Preserve authentication recovery before generic missing-dependency handling.

## Parallelism

`sequential`

## Inputs

- Requirement 005 and existing 001/004 identity, budget, and generation criteria.
- Design: Wire and task retry; existing Bounded setup retry.
- Existing lifecycle startup retry fixtures and orchestration recovery tests.

## Results

Pending.
