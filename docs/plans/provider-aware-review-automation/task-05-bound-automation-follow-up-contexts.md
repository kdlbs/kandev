---
id: "05-bound-automation-follow-up-contexts"
title: "Bound automation follow-up contexts"
status: done
wave: 2
depends_on: ["04-make-gitlab-lifecycle-evaluation-lean"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/provider-aware-review-automation.md"
---

# Task 05: Bound automation follow-up contexts

## Acceptance

- Post-side-effect checkpoint, error, response, and publication work uses a
  cancellation-independent context with a finite short timeout.
- Blocking store and event-bus fakes prove evaluation returns and releases the
  per-MR singleflight after timeout.
- Dispatch deadline, dispatch-before-checkpoint ordering, and current at-least-
  once semantics remain unchanged.

## Verification

- `cd apps/backend && go test -race ./internal/orchestrator -run 'Test.*TaskMR.*(Timeout|Context|InFlight|Checkpoint|Publish|Error)'`

Write blocking-fake regressions first. They must demonstrate the current
`context.WithoutCancel` follow-up can outlive the detached automation timeout.

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_gitlab.go`
- `apps/backend/internal/orchestrator/event_handlers_gitlab_mr_automation_eval.go`
- `apps/backend/internal/orchestrator/event_handlers_gitlab_mr_automation_test.go`

## Dependencies

Task 04 establishes the final evaluation flow to which bounded contexts apply.

## Parallelism

Sequential after Task 04 because both tasks touch the GitLab MR evaluator.

## Inputs

- Existing `ciAutomationDetachedTimeout`
- Current post-dispatch `context.WithoutCancel` call sites
- Spec `Bounded automation follow-up work` and `Stalled follow-up dependency`
  scenario

## Risks

- Reusing one timeout across multiple operations can make later operations inherit
  elapsed time. Create and cancel a fresh bounded context per operation.
- Do not treat timeout enforcement as exactly-once delivery or reorder dispatch
  and checkpoint persistence.

## Output contract

RED evidence: the blocking-follow-up suite first failed to compile because the
bounded helper and timeout did not exist. After implementation, each blocking
fake returned at its deadline, and the detached checkpoint regression observed
the durable prompt remain queued while the MR single-flight entry was removed.

The selected follow-up timeout is five seconds. Each checkpoint write, error
record, response load, and event publication gets a fresh
`context.WithTimeout(context.WithoutCancel(parent), timeout)` context; the
existing two-minute detached dispatch deadline and dispatch-before-checkpoint
ordering remain unchanged.

Changed files:

- `apps/backend/internal/orchestrator/event_handlers_gitlab_mr_automation_eval.go`
- `apps/backend/internal/orchestrator/event_handlers_gitlab_mr_automation_test.go`
- `apps/backend/internal/orchestrator/event_handlers_gitlab_mr_automation_timeout_test.go`

Verification:

```text
cd apps/backend && go test -race ./internal/orchestrator -run 'Test.*TaskMR.*(Timeout|Context|InFlight|Checkpoint|Publish|Error)'
ok   github.com/kandev/kandev/internal/orchestrator  1.422s
```

Risk: follow-up persistence remains at-least-once. A dependency that exceeds
the five-second bound can leave the checkpoint unstamped, so a later poll may
retry the notification; the timeout prevents that retry from holding the
single-flight entry indefinitely.
