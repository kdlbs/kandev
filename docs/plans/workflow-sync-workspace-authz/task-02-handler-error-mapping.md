---
id: "02-handler-error-mapping"
title: "Handler error mapping + HTTP regression tests"
status: pending
wave: 1
depends_on: ["01-service-authorization"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/workflow-sync-workspace-authz.md"
---

# Task 02: Handler error mapping + HTTP regression tests

Map the new `ErrWorkspaceNotFound` denial to a sanitized 404 in the HTTP layer, and add the
HTTP-level regression coverage this package currently lacks entirely (`handlers_test.go` does not
exist yet).

## Changes

1. `apps/backend/internal/workflowsync/handlers.go`:
   - Import `errors` (already imported) and
     `github.com/kandev/kandev/internal/task/repository/repoerrors`.
   - Add `func workspaceDenied(err error) bool { return errors.Is(err, repoerrors.ErrWorkspaceNotFound) }`
     (mirror `internal/jira/handlers.go:18-22`).
   - In `httpGetConfig`, `httpSetConfig`, `httpDeleteConfig`, `httpForceSync`: check
     `workspaceDenied(err)` before the existing generic-error branch and respond
     `ctx.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})`. Do not include the
     underlying service error or any config fields in this response.
2. New file `apps/backend/internal/workflowsync/handlers_test.go` — HTTP-level tests using
   `gin.New()` + `RegisterRoutes` + `httptest.NewRequest` + `authn.WithIdentity` on the request
   context (pattern: `internal/github/controller_auth_identity_test.go`).

## Acceptance (from spec Scenarios)

- Owner identity: GET/POST/DELETE/sync all succeed against their own workspace, unchanged from
  pre-fix behavior.
- Foreign member identity against a seeded victim workspace's config: all four routes return 404,
  and the response body contains none of the victim's repo owner/name/branch/path/project_path.
- The victim's config is byte-for-byte unchanged after the denied POST/DELETE attempts (re-read via
  the service and compare).
- Synthetic identity (auth disabled) succeeds on all four routes — no regression for the default
  path.
- The two "foreign member GET/POST" cases are the regression tests: written to fail against
  pre-task-01 code (they'd get 200 + leaked/accepted data) and pass after task 01 + this task.

## Verification

```bash
cd apps/backend && go test ./internal/workflowsync/... -run 'TestHTTP|TestConfig|TestForceSync' -v
cd apps/backend && go test ./internal/workflowsync/... -v
cd apps/backend && golangci-lint run ./internal/workflowsync/... --new-from-rev=main --timeout=5m
cd apps/backend && go test -race ./internal/workflowsync/...
```

## Results

- Status: **done**, including two follow-up fixes from an independent Codex review round.
- `go test ./internal/workflowsync/... -run 'TestHTTPHandlers' -v`: all pass
  (`TestHTTPHandlers_OwnerSucceedsOnAllRoutes`, `TestHTTPHandlers_SyntheticIdentitySucceedsOnAllRoutes`,
  `TestHTTPHandlers_ForeignMemberDeniedOnAllRoutesWithoutLeaking`, each table-driven across
  get/post/delete/sync, plus `TestHTTPHandlers_ForceSyncDeniesSecondLookupAfterAccessRevoked`).
- Regression confirmed: before the task-01 service change, the get/post/delete/sync subtests failed
  with 500s and leaked/accepted the foreign workspace's config (verified during TDD).
- **Codex review finding (blocker, fixed):** `httpForceSync`'s second call
  (`GetConfigForWorkspace`, used to build the response after a successful sync) did not map
  `ErrWorkspaceNotFound` to 404 — a race where access is revoked between the sync and the follow-up
  read fell through to a generic 500. Fixed with the same `workspaceDenied` check; covered by
  `TestHTTPHandlers_ForceSyncDeniesSecondLookupAfterAccessRevoked` (verified to fail pre-fix, pass
  post-fix).
- **Codex review finding (suggestion, fixed):** owner/synthetic-identity coverage was GET-only and
  the leak/mutation assertions didn't cover branch/path or the full config. Rewritten as
  table-driven tests across all four operations (`TestHTTPHandlers_OwnerSucceedsOnAllRoutes`,
  `TestHTTPHandlers_SyntheticIdentitySucceedsOnAllRoutes`), with distinctive branch/path fixture
  values and a full-struct `before`/`after` config comparison in the denial test.
- `go test -race ./internal/workflowsync/... ./internal/backendapp/...`: both `ok`.
- `golangci-lint run ./internal/workflowsync/... --new-from-rev=main --timeout=5m`: 0 issues.
