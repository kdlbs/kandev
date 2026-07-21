---
id: "04-reviewers-subscriptions"
title: "GitLab reviewers and notification subscriptions"
status: done
wave: 2
depends_on: ["01-workspace-connection"]
plan: "plan.md"
spec: "../../specs/gitlab-integration/spec.md"
---

# Task 04: GitLab Reviewers And Notification Subscriptions

## Acceptance

- Project-member search returns active members with numeric IDs and reviewer
  replacement sends `reviewer_ids`, including an empty list to clear reviewers.
- MR and issue subscription reads/writes call the correct GitLab resource and
  return live upstream state without creating or changing Kandev watches.
- PAT, glab, mock, and noop clients plus workspace-scoped HTTP routes share the
  contract and sanitize provider failures.

## Verification

```bash
cd apps/backend && rtk go test -run 'Test.*(ProjectMembers|Reviewers|Subscription|Subscribe|Unsubscribe)' ./internal/gitlab/...
cd apps/backend && rtk go test ./internal/gitlab/...
```

## Files Likely Touched

- `apps/backend/internal/gitlab/models.go`
- `apps/backend/internal/gitlab/client.go`
- `apps/backend/internal/gitlab/client_helpers.go`
- `apps/backend/internal/gitlab/client_helpers_test.go`
- `apps/backend/internal/gitlab/pat_client.go`
- `apps/backend/internal/gitlab/pat_client_actions.go`
- `apps/backend/internal/gitlab/pat_client_test.go`
- `apps/backend/internal/gitlab/glab_client.go`
- `apps/backend/internal/gitlab/glab_client_test.go`
- `apps/backend/internal/gitlab/mock_client.go`
- `apps/backend/internal/gitlab/mock_client_test.go`
- `apps/backend/internal/gitlab/noop_client.go`
- `apps/backend/internal/gitlab/noop_client_test.go`
- `apps/backend/internal/gitlab/service.go`
- `apps/backend/internal/gitlab/controller_member_subscriptions.go` (new)
- `apps/backend/internal/gitlab/controller_member_subscriptions_test.go` (new)
- `apps/backend/internal/gitlab/controller_watches.go`

## Dependencies

Task 01 provides workspace client selection. This task owns
`controller_member_subscriptions.go`; it must not edit Task 03's task-MR handler.

## Inputs

- Spec: reviewer/subscription `What`, Reviewers and notifications API,
  Permissions, action failure modes, and reviewer/subscription scenarios.
- Existing GitLab patterns: `SetMRAssignees`, `SetMRLabels`, PAT action helpers,
  and `parseMR` in `client_helpers.go`.
- Upstream contract: GitLab REST v4 project members, MR `reviewer_ids`, and issue
  and MR subscribe/unsubscribe endpoints.
- Constraint: notification subscription is upstream state only; do not add a
  SQLite table or reuse automation watch APIs.

## Output Contract

Report client signatures, REST/glab commands, error mapping, numeric-ID model
changes, files changed, tests run, blockers, and GitLab-version compatibility
risks. Mark this task `done` and update `plan.md` after verification.
