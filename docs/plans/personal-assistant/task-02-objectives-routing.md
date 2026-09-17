---
id: "02-objectives-routing"
title: "Objectives and proportional routing"
status: done
wave: 2
depends_on: ["01-durable-intake"]
plan: "plan.md"
requirements:
  - REQ-ORCHESTRATION-ASSISTANT-001
acceptance_criteria:
  - AC-ORCHESTRATION-ASSISTANT-001.1
  - AC-ORCHESTRATION-ASSISTANT-001.2
  - AC-ORCHESTRATION-ASSISTANT-001.3
  - AC-ORCHESTRATION-ASSISTANT-001.4
system_design:
  - ../../specs/orchestration/system-design/personal-assistant.md
---

# Task 02: Objectives and proportional routing

## Inputs

Read the [requirements](../../specs/orchestration/requirements/personal-assistant.md) and [design](../../specs/orchestration/system-design/personal-assistant.md); legacy scenarios S02, S03, S04, S13, and [plan](plan.md), Backend 2. Read applicable AGENTS.md and implementation skills before editing. The [baseline experiments](experiments.md) are continuation evidence, not completed implementation.

## Acceptance

1. Answer/inspect requests require no delivery task or plan; explicit execute/design routing validates workflow step and repository-policy constraints.
2. New work and follow-ups carry objective, acceptance, context and operation references; adopted tasks retain assigned profiles and multi-session results.
3. Completion requires current acceptance evidence and applicable review gates, not merely REVIEW or a finished agent turn.

## Likely files

- apps/backend/internal/orchestration/models/workspace_task.go; objective.go (new)
- apps/backend/internal/orchestration/repository/sqlite/objectives.go (new)
- apps/backend/internal/orchestration/runtime/objectives.go, objectives_test.go (new); handler.go
- apps/backend/internal/backendapp/adapters_workspace_tasks.go, adapters_workspace_results.go, adapters_workspace_tasks_test.go
- apps/backend/cmd/agentctl/kandev.go, kandev_task.go, kandev_test.go, kandev_orchestration_test.go
- apps/backend/internal/orchestration/instructions/AGENTS.md (application prompt asset, not repository policy)

## Implementation sequence

Persist objectives/links and test pure routing decisions. Extend the existing create/manage DTOs and CLI with validated step, mode, operation and intent fields. Keep orchestration runtime independent of Office and use canonical task services. Add two-session fixtures with implementation and review evidence; direct answers use assistant work records, not throwaway board cards.

## Verification

Run each parenthesized command from the repository root. Use the repository Go/Node/pnpm toolchains. Scoped Go tests are intentional: the available make test target runs the entire backend. New test filters must select the named new tests; a no-tests-to-run result does not satisfy acceptance.

```sh
(cd apps/backend && go test -tags fts5 -count=1 ./internal/orchestration/... ./internal/backendapp -run 'TestAssistant(Objective|Routing|Completion)|TestWorkspaceTask')
(cd apps/backend && go test -count=1 ./cmd/agentctl -run 'Test.*Orchestration|Test.*Workspace')
```

## Dependencies and risks

Dependencies: `01-durable-intake`. Execute in the primary session unless the user explicitly authorizes subagents.

Skipping a Requirements step does not override AGENTS.md. Step names are translated/user-configurable; choose stable validated IDs, never English labels. Concurrent writers cannot share a mutable worktree without serialization.

## Output

A tested proportional-routing and evidence-backed completion slice; no workflow defaults changed.

## Results

Implemented persisted, owner-scoped objectives with acceptance revisions, typed evidence references and task/session links. Answer/inspect objectives create no delivery cards. Explicit delivery carries objective/context/operation references; the adapter validates workflow membership and permitted entry (`is_start_step` or `allow_manual_move`) without changing defaults. Design retains core planning entry selection, and delegation carries the repository-policy handoff constraint. Legacy unbound/unspecified-mode task contracts remain compatible.

Completion checks acceptance coverage/current revision, persisted result identity, all linked sessions, live background work, pending activity, native review findings and required workflow decisions. Results include bounded excerpts for multiple sessions; adoption and explicit profile assignment retain the existing canonical services.

Red evidence: missing objective routes (404), inspect creating a task, missing multi-session results, missing CLI flags, stale completion evidence, failed review incorrectly accepted, a settled session incorrectly considered busy, and retry rejected after a successfully delegated objective was paused. Corrected each with focused tests.

Exact task checks (with `GIN_MODE=release`, `-v` and the local Go toolchain): **10 tests** passed across runtime/backendapp, **9 CLI tests** passed. Additional affected-boundary checks: full orchestration and agentctl suites passed under `-race`; scoped backendapp/orchestrator race tests passed for `TestAssistant|TestWorkspace|TestChiefCanObserve|TestOrchestratorUsesTaskProfiles`, including native background work accounting and account preservation. No production service, workflow defaults or credentials changed.

Owned changes: objective model/schema/repository/runtime routes; delegation references and acceptance prompts; task adapter entry/completion checks and multi-session results; objective/task CLI flags and instructions; one read-only orchestrator activity accessor; focused tests and a canonical workflow-store test fixture. Enforced provider-native read-only execution remains task 05, not a guarantee of the mode label alone. Versioned context contents remain task 03.
