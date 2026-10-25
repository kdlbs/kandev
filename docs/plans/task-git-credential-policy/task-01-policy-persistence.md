---
id: "01-policy-persistence"
title: "Persist the task Git credential policy"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/requirements/github-authentication.md"
---

# Task 01: Persist The Task Git Credential Policy

## Acceptance

- `github_workspace_settings.task_git_credentials_mode` defaults/normalizes to `managed`, accepts
  only `managed` or `executor`, and survives fresh schema, replay, read, upsert, partial update, and
  settings copy.
- The existing workspace-settings API exposes the policy without changing its authorization model.
- A non-secret GitHub service descriptor returns the policy plus known workspace method/actor
  context without resolving a token.

## Verification

```bash
cd apps/backend && go test ./internal/github ./internal/backendapp -run 'Test(Store_GitHubWorkspaceSettings|Service_UpdateWorkspaceSettings|CopyWorkspaceSettings|TaskGitCredential|GitHubWorkspaceSettings)'
```

## Files likely touched

- `apps/backend/internal/github/models.go`
- `apps/backend/internal/github/store.go`
- `apps/backend/internal/github/workspace_settings_service.go`
- `apps/backend/internal/github/copy.go`
- `apps/backend/internal/github/controller.go`
- `apps/backend/internal/github/workspace_settings_test.go`
- `apps/backend/internal/github/copy_test.go`
- focused GitHub schema/descriptor tests
- `apps/backend/internal/backendapp/services_github_broker_test.go`
- this task file and `plan.md`

## Dependencies

None.

## Parallelism

Sequential. It owns the shared schema and API contract.

## Inputs

- Spec sections `What`, `Data Model`, `API Surface`, and task-policy scenarios.
- ADR-2026-07-27-task-git-credential-policy.
- Existing repository-scope partial-update and workspace-copy patterns.

## Output contract

Report the migration/replay behavior, normalized enum contract, descriptor shape, RED/GREEN command
results, files changed, residual risks, and update this task plus `plan.md` status.
