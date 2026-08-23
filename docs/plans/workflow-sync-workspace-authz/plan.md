---
spec: docs/specs/tasks/requirements/workflow-sync-workspace-authz.md
created: 2026-08-06
status: building
---

# Implementation Plan: Workflow Sync — Per-User Workspace Authorization

## Overview
Close the workspace-scoping gap in `internal/workflowsync` by reusing the exact pattern already
used by every other per-workspace integration service in this codebase (GitHub, Jira, Linear,
Slack, Azure DevOps, Automation): a nil-safe `workspaceAuthorizer` field on `workflowsync.Service`,
set post-construction from `backendapp` with `taskSvc.AuthorizeWorkspaceAccess`, checked first in
every user-facing service method. Handlers map the resulting `ErrWorkspaceNotFound` to a sanitized
404. Two tasks, sequential (the second's handler test depends on the first's service change):
service-layer authorization + wiring, then handler-layer error mapping + HTTP-level regression
tests.

---

## Backend

### Service layer (`internal/workflowsync/service.go`)
Add, mirroring `internal/github/service.go:137,169-181` exactly:
```go
type Service struct {
    // ...existing fields...
    workspaceAuthorizer func(context.Context, string) error
}

func (s *Service) SetWorkspaceAuthorizer(authorizer func(context.Context, string) error) {
    if s != nil {
        s.workspaceAuthorizer = authorizer
    }
}

func (s *Service) authorizeWorkspaceAccess(ctx context.Context, workspaceID string) error {
    if s == nil || s.workspaceAuthorizer == nil {
        return nil
    }
    return s.workspaceAuthorizer(ctx, workspaceID)
}
```
Call `if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil { return nil, err }`
(or `return err` for the delete path) as the first line of the body in:
- `GetConfigForWorkspace`
- `SetConfigForWorkspace` (before `req.Normalize()` — no need to validate an attacker's payload)
- `DeleteConfigForWorkspace`
- `SyncWorkspace` (covers `httpForceSync`, and is a no-op for the internal poller's identity-free
  calls since `authorizeWorkspaceID`'s `callerScope` already treats no-identity as unscoped)

### Wiring (`internal/backendapp/services.go`)
In `initWorkflowSyncService` (already receives `taskSvc *taskservice.Service` — currently used only
for `workflowSvc.SetSyncWorkflowOps(taskSvc)`), add immediately after `workflowsync.Provide`
succeeds, matching the azuredevops in-function style at services.go:609-612:
```go
svc.SetWorkspaceAuthorizer(taskSvc.AuthorizeWorkspaceAccess)
```

### Handler layer (`internal/workflowsync/handlers.go`)
Add a `workspaceDenied` helper (mirrors `internal/jira/handlers.go:18-22`) and use it in all four
handlers to return 404 instead of falling into the generic `internalError` 500 path:
```go
func workspaceDenied(err error) bool {
    return errors.Is(err, repoerrors.ErrWorkspaceNotFound)
}
```
`httpGetConfig`, `httpSetConfig`, `httpDeleteConfig`, `httpForceSync`: check
`workspaceDenied(err)` before the generic error branch and respond
`ctx.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})` (no config/repo data in the
body). Import `github.com/kandev/kandev/internal/task/repository/repoerrors`.

---

## Tests

- **What:** foreign-workspace denial at the service layer for all four entry points.
  **File:** `apps/backend/internal/workflowsync/service_authorization_test.go` (new).
  **How:** unit test constructing `Service` directly (no HTTP), installing a fake
  `SetWorkspaceAuthorizer` that asserts the identity/workspaceID it receives (pattern:
  `internal/github/handlers_authorization_test.go`'s
  `TestReviewWatchWebSocketDeniesForeignWorkspaceListAndMutations`), returning
  `repoerrors.ErrWorkspaceNotFound`; assert each of `GetConfigForWorkspace`,
  `SetConfigForWorkspace`, `DeleteConfigForWorkspace`, `SyncWorkspace` returns that sentinel and
  performs no store mutation (re-read the config afterward to confirm it's unchanged).
- **What:** unscoped callers (nil authorizer, and an authorizer that itself no-ops on missing
  identity) are unaffected. **File:** same new file. **How:** call the same four methods with no
  authorizer wired and confirm they succeed as before (regression guard against a nil-safety
  mistake in `authorizeWorkspaceAccess`).
- **What:** HTTP-level 404 + no-leak behavior end to end.
  **File:** `apps/backend/internal/workflowsync/handlers_test.go` (new — this package currently has
  zero handler-level tests). **How:** `gin.New()` + `RegisterRoutes`, `httptest.NewRequest` +
  `authn.WithIdentity` on the request context (pattern:
  `internal/github/controller_auth_identity_test.go`), covering:
  - Owner identity succeeds on GET/POST/DELETE/sync against their own workspace.
  - Foreign member identity gets 404 on all four routes against a seeded victim workspace's config,
    and the response body never contains the victim's repo owner/name/branch/path.
  - Victim's config is unchanged after the denied POST/DELETE attempts.
  - Synthetic identity (auth disabled) still succeeds — no regression for the default (auth-off)
    path.
- **Regression test that must fail before the fix and pass after:** the foreign-member GET/POST
  cases above — today they succeed and leak/accept a foreign workspace's config; after the fix they
  404.

Backend-only change; no frontend/E2E surface (no UI, WS, or client-visible contract changes).

---

## Verification Results

Both tasks done; see each task file's `## Results` for exact per-task output, including two
Codex-review-driven follow-up fixes (untested production wiring, force-sync's second error path)
and expanded test coverage. Whole-package summary:
- `go build ./...`: clean.
- `gofmt -l internal/workflowsync internal/backendapp`: no output (already formatted).
- `go vet ./internal/workflowsync/... ./internal/backendapp/...`: clean.
- `go test -race ./internal/workflowsync/... ./internal/backendapp/...`: both `ok`.
- `golangci-lint run ./internal/workflowsync/... ./internal/backendapp/... --new-from-rev=main --timeout=5m`: 0 issues.
- Full `go build ./... && go test -race ./...` (whole backend): all packages `ok` except a
  pre-existing, unrelated flake in `internal/repoclone`
  (`TestEnsureWorkspaceClonedWithBasicAuthKeepsCredentialScopedToGitChild/context_cancellation`,
  a subprocess-timing test) — confirmed unrelated by re-running it in isolation (passes) and by this
  branch touching neither that package nor anything it depends on.

**Severity correction:** Codex review found that `origin/main` already has a global
`integrationWorkspaceScopeMiddleware` (`internal/backendapp/main.go:1807`) that path-matches
`/api/v1/workflow-sync/` and authorizes before any handler runs — the HTTP-level exploit originally
described in this spec's first draft was already blocked. See spec.md's "Correction" note; this fix
closes a real but narrower gap (service-layer defense-in-depth, consistency with every sibling
integration, protection for any future non-HTTP caller), not a currently-open HTTP vulnerability.

---

## Implementation Waves And Parallel Candidates

Sequential (2 tasks; task 2's handler tests depend on task 1's service-layer change):
- [x] [task-01-service-authorization](task-01-service-authorization.md)
- [x] [task-02-handler-error-mapping](task-02-handler-error-mapping.md)

## Open Questions
(none)
