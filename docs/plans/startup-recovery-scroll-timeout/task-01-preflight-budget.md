---
id: "01-preflight-budget"
title: "Extend the bounded preflight budget"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-REMOTE-CONTRIBUTION-TASKS-002
acceptance_criteria:
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-002.3
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-002.4
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-002.6
system_design:
  - ../../specs/tasks/system-design/remote-contribution-tasks.md
---

# Task 01: Extend the Bounded Preflight Budget

## Summary

Give contribution startup checks one two-minute budget across all repositories.
Make the preflight HTTP path honor that budget without changing unrelated requests.

## In scope

- Extend the lifecycle default and bound direct preflight callers.
- Reuse shared transport and response handling without mutating the shared client.
- Preserve earlier deadlines, cancellation, rejection, and history-only admission rules.

## Out of scope

Global HTTP, ACP load, workspace refresh, and readiness timeout changes.
No new operator setting, retry loop, or credential policy.

## Acceptance

- A preflight can complete within two minutes despite the ordinary client's shorter timeout.
- Expiry or cancellation blocks startup, with no late agent launch or permission bypass.
- All repositories share one budget. One rejection blocks the complete operation.

## Verification

First add `TestGitPushPreflightHonorsOperationBudget` with a short ordinary-client
timeout and a controlled response. Its expected RED is premature transport expiry.
Extend `TestStartAgentProcessBoundsContributionPreflight` for the default budget,
short injected expiry, and caller cancellation. Preserve existing admission tests.
Use barriers or a controlled transport. Do not run two-minute sleeps or CPU stress.

```bash
(cd apps/backend && go test -race ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle)
git diff --check
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_startup.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_startup_test.go`
- `apps/backend/internal/agent/runtime/agentctl/git.go`
- `apps/backend/internal/agent/runtime/agentctl/git_test.go`
- `apps/backend/internal/agent/runtime/agentctl/client.go` (only if the scoped request path needs it)

## Dependencies

None.

## Risks

A transport-specific timeout or shared client mutation can defeat the intended bound.
Tests must cover ordinary request isolation and earlier caller deadlines.

## Parallelism

`sequential`

## Inputs

- [Plan](plan.md), evidence and technical approach.
- [Contribution requirements](../../specs/tasks/requirements/remote-contribution-tasks.md), requirement 002.
- [Contribution design](../../specs/tasks/system-design/remote-contribution-tasks.md), proposed timing amendment.
- Existing startup preflight and client Git request tests.

## Results

Implemented a two-minute default for the complete contribution preflight loop.
`GitPushPreflight` uses a per-operation client copy with the same transport and
headers, while the ordinary Git client keeps its 60-second timeout. Direct
preflight callers receive the same bounded context, and earlier cancellation or
deadlines still win. Rejections continue to block startup before agent launch.

Validation passed with the agentctl and lifecycle race suite, targeted preflight
tests, and `git diff --check`.
