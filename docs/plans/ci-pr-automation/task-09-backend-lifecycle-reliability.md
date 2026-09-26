---
id: "09-backend-lifecycle-reliability"
title: "Backend lifecycle reliability"
status: done
wave: 6
depends_on: ["08-pr-lifecycle-agent-prompts"]
plan: "plan.md"
spec: "../../specs/ui/requirements/ci-pr-automation.md"
---

# Task 09: Backend lifecycle reliability

## Acceptance

- A changed connected GitHub login is rebound atomically at task scope and
  quietly resets every linked PR's review-request baseline without modifying
  terminal or CI checkpoints.
- Lifecycle prompts use immutable server-owned templates from the spec,
  interpolating only a validated canonical PR URL; delivery failures persist
  and broadcast `last_error`, remain retryable, and a successful accepted/queued
  delivery clears and broadcasts the error.
- Archived/deleted tasks cannot retain an active PR watch or be reactivated by
  lifecycle evaluation.

## Verification

```bash
cd apps/backend && go test -race ./internal/github ./internal/orchestrator
```

## Files likely touched

- `apps/backend/config/prompts/pr-review-requested.md`
- `apps/backend/config/prompts/pr-merged-final.md`
- `apps/backend/config/prompts/pr-closed-final.md`
- `apps/backend/internal/github/store.go`
- `apps/backend/internal/github/service_ci_automation.go`
- `apps/backend/internal/github/store_ci_automation_test.go`
- `apps/backend/internal/github/service_ci_automation_test.go`
- `apps/backend/internal/github/service_task_events_test.go`
- `apps/backend/internal/orchestrator/event_handlers_github_ci_automation.go`
- `apps/backend/internal/orchestrator/event_handlers_github_pr_automation.go`
- `apps/backend/internal/orchestrator/event_handlers_github_pr_automation_test.go`
- This task file

## Dependencies

- Task 08 and the approved lifecycle requirements in the spec.

## Inputs

- Spec: `What`, `State machine`, `Failure modes`, and lifecycle scenarios.
- Plan: `PR lifecycle reliability refinement`.
- Existing patterns:
  `Store.UpdateTaskCIOptions`, `recordCIAutomationError`,
  `publishTaskCIOptionsState`, `dispatchTaskPRAgentPrompt`, and
  `service_task_events.go`.

## Constraints

- Use TDD.
- Keep lifecycle evaluation in the existing CI automation pass.
- Do not add a poller, scheduler, event type, goroutine, in-flight map, schema
  table, replacement session, or workflow-step transition.
- Preserve unrelated auto-fix/auto-merge behavior and checkpoints.
- Do not edit `plan.md`; update only this task file's status.

## Output contract

- Summary of behavior.
- Files changed.
- Tests run with exact results.
- Blockers, divergence, and follow-up risks.
- Set this task file to `done` only after acceptance and targeted verification
  pass.
