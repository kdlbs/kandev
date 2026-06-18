---
id: "03-backend-automation-execution"
title: "Backend automation execution"
status: done
wave: 2
depends_on: ["01-backend-persistence-prompts"]
plan: "plan.md"
spec: "../../specs/ui/ci-pr-automation.md"
---

# Task 03: Backend Automation Execution

## Acceptance

- Auto-fix evaluates existing PR watch events, fetches full feedback only for candidate PRs, and sends or queues one prompt per new actionable feedback snapshot.
- Auto-merge merges only ready PRs and does not retry the same failed readiness signature every poll.
- Unit tests cover disabled no-op, dedupe, new-feedback delta prompting, busy-session queueing, merge readiness, and merge retry throttling.

## Verification

```bash
cd apps/backend && rtk go test ./internal/orchestrator ./internal/github
```

## Files Likely Touched

- `apps/backend/internal/orchestrator/event_handlers_github.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/messagequeue/types.go`
- `apps/backend/internal/orchestrator/event_handlers_github_ci_automation_test.go`
- `apps/backend/internal/orchestrator/event_handlers_github_test.go`
- `apps/backend/internal/github/service_pr.go`
- `apps/backend/internal/github/service_pr_watch.go`
- `apps/backend/internal/events/types.go` if a new internal event is needed

## Dependencies

- `01-backend-persistence-prompts`

## Inputs

- Spec sections: State machine, Failure modes, Scenarios.
- Plan sections: Backend > Automation execution.
- Existing patterns: `handlePRFeedback`, `subscribeGitHubEvents`, `PromptTask`, and `QueueMessageWithMetadata`.

## Output Contract

When complete, update this file's `status` to `done`, update the Wave 2 checkbox in `plan.md`, and report changed files, tests run, blockers, and residual risks.
