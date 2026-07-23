---
id: "02-jira-provider"
title: "Jira mention provider"
status: completed
wave: 2
depends_on: ["01-core-and-task-search"]
plan: "plan.md"
spec: "../../specs/ui/entity-reference-composer.md"
---

# Task 02: Jira Mention Provider

## Acceptance

- Jira ticket projections preserve immutable upstream issue ID plus key/title/URL/site scope.
- Plain text becomes backend-generated, escaped key-or-title JQL; callers cannot inject raw JQL.
- Adapter enforces explicit workspace config and maps auth/rate/timeout/upstream failures safely.
- Provider authorizer accepts only the configured workspace site origin/scope for search and submission.

## Verification

```bash
cd apps/backend && go test ./internal/jira/... ./internal/mentions/...
```

## Files likely touched

- `apps/backend/internal/jira/models.go`
- `apps/backend/internal/jira/cloud_client.go`
- `apps/backend/internal/jira/cloud_client_test.go`
- `apps/backend/internal/jira/service_mentions.go`
- `apps/backend/internal/jira/service_mentions_test.go`
- `apps/backend/internal/mentions/provider_jira.go`
- `apps/backend/internal/mentions/provider_jira_test.go`

## Dependencies

Task 01.

## Inputs

Spec sources/security/failure requirements; `SearchTicketsForWorkspace`; Jira response conversion tests.

## Output contract

Report identity and escaping behavior, files changed, exact tests, blockers, risks, and mark task/plan done.
