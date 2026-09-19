---
id: "07-workflow-step-action"
title: "run_code_review workflow step entry action"
status: done
wave: 4
depends_on: ["04-run-orchestration"]
plan: "plan.md"
spec: "../../specs/agents/requirements/native-code-review.md"
---

# Task 07: `run_code_review` workflow step entry action

Make a review pass a declarative step behavior, with its own agent profile.

## Inputs

- Spec **API surface** → Workflow step action; the passthrough scenario in **Scenarios** ("the task still enters the step").
- `internal/workflow/models/models.go` — `OnEnterActionType` / `GenericActionType` constants and `StepEvents`.
- `internal/workflow/engine/types.go` `ActionKind`, `internal/workflow/engine/adapters.go` model→engine mapping.
- `internal/orchestrator/workflow_callbacks.go` — `autoStartAgentCallback` is the closest shape; note the conditional registration pattern for optional adapters.
- `internal/workflow/models/export.go` — portable agent-profile references.

## Work

1. `internal/workflow/models/models.go` — `OnEnterRunCodeReview OnEnterActionType = "run_code_review"`, `GenericActionRunCodeReview GenericActionType = "run_code_review"`, and `RunCodeReviewAction struct { AgentProfileID string }` added as a nullable field on the on-enter and generic action structs.
2. `internal/workflow/engine/types.go` — `ActionRunCodeReview ActionKind = "run_code_review"` and the `RunCodeReview *RunCodeReviewSpec` field on the engine action.
3. `internal/workflow/engine/adapters.go` — map the model action to the engine action, carrying `AgentProfileID`.
4. `internal/orchestrator/workflow_callbacks.go` — `runCodeReviewCallback{svc}` registered only when `svc.reviewRunner != nil`. It resolves the task's session, calls `Runner.Run` with `Trigger = workflow_step` and `WorkflowStepID = in.Step.ID`, and on error logs and returns `engine.ActionResult{}, nil` so step entry still succeeds (the run row carries the failure).
5. `internal/workflow/models/export.go` — export/import the action, resolving `AgentProfileID` through `AgentProfileResolver` / `AgentProfileMatcher` like existing step profiles; validate the referenced profile exists on import in `internal/workflow/service/`.
6. `internal/workflow/service/` — reject a `run_code_review` action whose `agent_profile_id` names an unknown profile at save time.

## Acceptance

- Entering a step with the action starts a run with `trigger = workflow_step` and the step's profile agent/model.
- A run failure (including a passthrough profile) does not block the transition.
- Export → import round-trips the action and its profile reference across workspaces.

## Verification

```
cd apps/backend && go test ./internal/workflow/... ./internal/orchestrator/...
```

## Files likely touched

`internal/workflow/models/{models.go,export.go}` + tests, `internal/workflow/engine/{types.go,adapters.go}` + tests, `internal/workflow/service/`, `internal/orchestrator/workflow_callbacks.go` + test.

## Output contract

Summary, files changed, tests run with results, blockers, risks, `status: done`, plan checkbox.
