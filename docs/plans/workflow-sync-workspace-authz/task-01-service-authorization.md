---
id: "01-service-authorization"
title: "Service-layer workspace authorization"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/workflow-sync-workspace-authz/spec.md"
---

# Task 01: Service-layer workspace authorization

Add the `workspaceAuthorizer` boundary to `workflowsync.Service` and wire it at app boot, following
the exact pattern already used by `internal/github`, `internal/jira`, `internal/linear`,
`internal/slack`, `internal/azuredevops`, and `internal/automation`.

## Changes

1. `apps/backend/internal/workflowsync/service.go`:
   - Add `workspaceAuthorizer func(context.Context, string) error` field to `Service`.
   - Add `SetWorkspaceAuthorizer` (nil-safe on `s`) and `authorizeWorkspaceAccess` (nil-safe on `s`
     and the field) methods — copy the shape from `internal/github/service.go:169-181`.
   - Call `s.authorizeWorkspaceAccess(ctx, workspaceID)` first in `GetConfigForWorkspace`,
     `SetConfigForWorkspace`, `DeleteConfigForWorkspace`, `SyncWorkspace`, returning the error
     immediately (before touching `s.store` or `s.applier`).
2. `apps/backend/internal/backendapp/services.go`: in `initWorkflowSyncService`, after
   `workflowsync.Provide` succeeds, call `svc.SetWorkspaceAuthorizer(taskSvc.AuthorizeWorkspaceAccess)`
   before returning `svc`.

## Acceptance

- A caller whose identity does not own the target workspace gets `repoerrors.ErrWorkspaceNotFound`
  from all four `Service` methods, and no store read/write or applier call happens for that call.
- A caller with no identity in context, or a synthetic identity, or a `Service` with no authorizer
  wired, behaves exactly as before this change (fully unscoped, no denial).
- `go vet` / build clean; no new lint findings under the changed-file thresholds in
  `apps/backend/.golangci.yml`.

## Verification

```bash
cd apps/backend && go test ./internal/workflowsync/... -run 'Authoriz|SyncWorkspace|ConfigForWorkspace' -v
cd apps/backend && go build ./...
cd apps/backend && golangci-lint run ./internal/workflowsync/... ./internal/backendapp/... --new-from-rev=main --timeout=5m
```

## Results
(Fill in after implementation: exact test output, pass/fail counts.)
