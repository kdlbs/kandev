---
id: "04-continuation-step"
title: "Move continued PR tasks to a step"
status: done
wave: 4
depends_on:
  - "03-per-pr-continuation"
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-005
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006
acceptance_criteria:
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-005.1
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-005.2
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-005.3
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-005.4
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-005.5
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-005.6
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006.2
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006.3
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006.5
system_design:
  - ../../specs/office/system-design/automations-pr-follow-up.md
---

# Task 04: Move continued PR tasks to a step

## Summary

Add `pr_continuation_step_id`. When a per-PR firing continues a visible task,
Kandev moves the task to the chosen step without running the step's auto-start,
then sends the automation's prompt as the only turn. The editor offers **Move
to step when continuing**.

## In scope

- The column, the model and payload fields, the retry snapshot field, and the
  MCP field.
- Validation: the field requires `reuse_per_pr`, `normal_task`, and a step in
  the automation's workflow, checked with `authorizeWorkflowStepOwnership`.
  Updates that leave a step that no longer applies are rejected.
- Move before continuing: stamp the marker through `SetTaskMetadataKey`, call
  `MoveTask`, fail the run on error, dispatch, then release the marker.
- Export `workflow.continuation_step`.
- Editor: the step `Select` with the **Stay in current step** sentinel,
  cleared on workflow, target, or policy change. Copy in all locales.
- Public docs.

## Out of scope

- Running the step's auto-start prompt or on-enter actions.

## Acceptance

1. Validation and export tests cover the valid, foreign-workflow,
   wrong-policy, and wrong-target cases, and the key order `name`, `step`,
   `continuation_step`.
2. Orchestrator tests with a real workflow fixture prove four things. The task
   moves to the step. No workflow auto-start turn is sent, and exactly one
   bound turn is sent. A missing step or a move error fails the run without a
   prompt. The marker is released after binding and after failure. New and
   replacement tasks start in the configured start step.
3. The editor selector shows, hides, clears, and saves on desktop and phone.
   The E2E step scenario passes on both projects.

## ASCII UI preview

See [UI-03](plan.md#ui-03-context-between-runs-with-a-github_pr-trigger).

```text
Move to step when continuing
[ Stay in current step                                          v ]
```

This covers AC-006.2, 006.3, and 006.5.

## Verification

```bash
(cd apps/backend && go test ./internal/automation/ -run 'PRContinuationStep|Export' -count=1)
(cd apps/backend && go test ./internal/orchestrator/ -run 'PRContinuation' -count=1)
(cd apps/backend && go test ./internal/mcp/server/ -run 'ConfigAutomation' -count=1)
(cd apps && pnpm --filter @kandev/web test -- components/automations)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/automations-pr-follow-up.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/mobile-automations-pr-follow-up.spec.ts)
```

## Files likely touched

- `apps/backend/internal/automation/models.go`, `service.go`, `store.go`,
  `retry_snapshot.go`, `export.go`, `export_service.go`
- `apps/backend/internal/orchestrator/automation_pr_continuation.go` and its
  test
- `apps/backend/internal/mcp/server/config_automation_handlers.go`
- `apps/web/components/automations/automation-continuation-section.tsx`,
  `automation-payload.ts`, and their tests; `apps/web/src/locales/*/automations.json`
- E2E specs and `docs/public/automation-and-mcp.md`

## Dependencies

Task 03.

## Risks

- `MoveTask` publishes `task.moved`. The marker must already be persisted when
  the workflow handler reads it.
- Moving out of a terminal step can have workflow side effects. Cover this
  with a real workflow fixture.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/office/requirements/automations-pr-follow-up.md)
  REQ-005 and REQ-006.
- [System design](../../specs/office/system-design/automations-pr-follow-up.md):
  Continuation step, Editor, and Export.
- Patterns: `automation_start_ownership.go` and `autoStartTaskForLoadedStep`.

## Results

Done on 2026-11-24.

- `(cd apps/backend && go test -trimpath ./internal/automation/ -run 'ReusePerPRValidation|TestExportPerPRSettings' -count=1)`: ok.
- `(cd apps/backend && go test -trimpath ./internal/orchestrator/ -run 'PRContinuation|NewPRTask' -count=1)`: ok (move with ownership held, skip when unset or already in step, move failure fails the run and releases ownership, new tasks start in the start step). Mutation checks confirmed the tests fail without the ownership stamp or release.
- `(cd apps && pnpm --filter @kandev/web test -- components/automations)`: 214 passed.
- `(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/automations-pr-follow-up.spec.ts)`: 2 passed.
- `(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/mobile-automations-pr-follow-up.spec.ts)`: 1 passed.
