---
id: "02-gate-comment-prompt"
title: "Verdict prompt for task_comment runs at a review or approval stage"
status: todo
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-GATE-COMMENT-003
acceptance_criteria:
  - AC-OFFICE-GATE-COMMENT-003.1
  - AC-OFFICE-GATE-COMMENT-003.2
  - AC-OFFICE-GATE-COMMENT-003.3
  - AC-OFFICE-GATE-COMMENT-003.4
  - AC-OFFICE-GATE-COMMENT-003.5
  - AC-OFFICE-GATE-COMMENT-003.6
system_design:
  - ../../specs/office/system-design/gate-comment-wake-01.md
---

# Task 02: Verdict Prompt For Gate Comment Runs

## Summary

A `task_comment` run whose payload carries `stage_type` resolves its stage from
the step it was queued at (falling back to the payload `stage_type`), and when that stage is `review` or `approval` its
prompt frames the review, quotes the comment, asks for a verdict and carries the
decision contract when the agent holds a decision seat. Runs without
`stage_type` are byte-identical to today.

## In scope

- `buildPromptContext` in `internal/office/service/scheduler_integration.go`:
  for `task_comment` with non-empty `stage_type`, set `pc.StageType` from a new
  `resolveGateCommentStage(ctx, parsed)`.
- `resolveGateCommentStage` in `internal/office/service/review_stage.go`: the
  `GetWorkflowStepStageType` of the payload's `workflow_step_id` when present,
  read without error and non-empty; otherwise the payload `stage_type`. It never
  reads the task's current step.
- `buildGateCommentPrompt` in `internal/office/service/prompt_builder.go` and
  the `RunReasonTaskComment` branch choosing it.

## Out of scope

- The fan-out and template (Tasks 01 and 03).
- Changing `buildReviewStagePrompt`, `buildApprovalStagePrompt`,
  `buildTaskCommentPrompt` output or the decision contract text.
- Web UI copy; this is agent-facing prompt text.
- `resolveReviewStage`, `shouldResolveAuthoritativeStage` and
  `resolveMissingReviewStepID`: unchanged.

## Acceptance

- Review and approval `task_comment` prompts contain the task reference, the
  "reviewing"/"approving" framing, the `From:` author label, the quoted comment
  and the verdict instruction; `buildGateCommentPrompt`'s output ends with the
  decision contract exactly when `record_step_decision` is among
  `AllowedActions`, and the sections `BuildPrompt` appends to every reason
  follow it unchanged.
- An unloadable comment yields the review/approval framing with the
  no-longer-available sentence and no quote; a step whose authoritative stage is
  `work` yields the existing comment prompt despite payload `stage_type: review`;
  a step read error, an empty step stage and a missing `workflow_step_id` each
  use the payload `stage_type`; a task whose current step differs from the
  run's step resolves the run's step.
- A `task_comment` run without `stage_type` produces a prompt byte-identical to
  the current `buildTaskCommentPrompt` output (golden assertion), including one
  whose `workflow_step_id` names a review step: no stage is resolved without
  `stage_type` (AC-003.4, .5).

## Verification

```bash
# From the repository root:
cd apps/backend && go test ./internal/office/service/... -run 'Prompt|ReviewStage|TaskComment' -race -count=1
cd apps/backend && go test ./internal/office/service/... -race -count=1
make -C apps/backend lint
git diff --cached --name-only | grep '\.go$' | xargs -r gofmt -l
git diff --check
```

## Files likely touched

- `apps/backend/internal/office/service/prompt_builder.go`
- `apps/backend/internal/office/service/prompt_builder_test.go`
- `apps/backend/internal/office/service/review_stage.go`
- `apps/backend/internal/office/service/scheduler_integration.go`
- `apps/backend/internal/office/service/scheduler_integration_test.go`

## Dependencies

None.

## Risks

- `resolveReviewStage` is shared with `task_assigned` and
  `task_review_requested`; it stays untouched so their resolution cannot drift.
  Keep their existing tests green unmodified.
- `enrichBuilderComments` is not needed here; do not add a second copy of the
  comment to the prompt.

## Parallelism

`parallel-safe` with Task 01.

## Inputs

- `docs/specs/office/system-design/gate-comment-wake-01.md`, section "Prompt".
- `apps/backend/internal/office/service/prompt_builder.go`
  (`buildTaskCommentPrompt`, `buildReviewStagePrompt`, `writeDecisionContract`,
  `hasDecisionAction`).

## Results

Not started.
