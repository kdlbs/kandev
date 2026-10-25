---
id: "14-lifecycle-delivery-review-remediation"
title: "Lifecycle delivery review remediation"
status: done
wave: 8
depends_on:
  - "13-lifecycle-state-review-remediation"
plan: "plan.md"
spec: "../../specs/ui/requirements/ci-pr-automation.md"
---

# Task 14: Lifecycle delivery review remediation

## Acceptance

- Lifecycle session selection includes the existing primary `IDLE` session,
  then another promptable task session, without changing the global meaning of
  active-session repository queries or creating a new session.
- Every lifecycle event first enters the durable message queue with the stable
  task/repository/PR/event coalesce key. Promptable sessions drain immediately;
  busy sessions remain queued. A transient execution retry does not create a
  second visible chat message.
- Task archived/deleted state is revalidated immediately before the durable
  queue/prompt side effect, closing the in-flight race after initial polling or
  GitHub work.

## Verification

```bash
cd apps/backend && go test -race ./internal/task/repository/... ./internal/orchestrator
```

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_github_pr_automation.go`
- `apps/backend/internal/orchestrator/event_handlers_github_ci_automation_test.go`
- `apps/backend/internal/orchestrator/event_handlers_github_test.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/event_handlers_queue_test.go`
- repository tests only if a dedicated promptable-session query is required
- This task file

## Constraints

- Use TDD.
- Prefer `ListTaskSessions` plus explicit promptable-state filtering; do not
  broaden the shared active-session query solely for lifecycle delivery.
- Queue acceptance remains the durable lifecycle checkpoint boundary. Reuse
  the existing queue/drain/in-flight guards and metadata; do not build a second
  queue or scheduler.
- Do not add a replacement session, new poller, workflow-step move, or schema.
- Do not edit `plan.md`, commit, or spawn subagents.

## Output contract

- Set this task to `done` only after the idle, retry/dedupe, and archive-race
  regressions plus targeted race run pass.
- Report files, exact tests, divergence, blockers, and residual risk.
