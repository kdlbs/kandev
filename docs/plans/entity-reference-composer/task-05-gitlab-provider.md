---
id: "05-gitlab-provider"
title: "GitLab mention provider"
status: completed
wave: 2
depends_on: ["01-core-and-task-search"]
plan: "plan.md"
spec: "../../specs/ui/entity-reference-composer.md"
---

# Task 05: GitLab Mention Provider

## Acceptance

- Issue and MR adapters use immutable object ID plus host scope and keep human IID/project details for display.
- Safe service methods construct title-search parameters without accepting raw GitLab filters.
- Search validates workspace-owned config/scope and never leaks install-wide results across workspaces.
- Provider authorizers fail closed unless the result host/project is proven to belong to the active workspace.

## Verification

```bash
cd apps/backend && go test ./internal/gitlab/... ./internal/mentions/...
```

## Files likely touched

- `apps/backend/internal/gitlab/service_mentions.go`
- `apps/backend/internal/gitlab/service_mentions_test.go`
- `apps/backend/internal/gitlab/store.go`
- `apps/backend/internal/mentions/provider_gitlab.go`
- `apps/backend/internal/mentions/provider_gitlab_test.go`

## Dependencies

Task 01.

## Inputs

ADR 0030; GitLab `SearchUserIssuesPaged`/`SearchUserMRsPaged`; existing stable `ID`, `IID`, `ProjectID`, and host models.

## Output contract

Report workspace enforcement/query safety, files changed, exact tests, blockers, risks, and mark task/plan done.
