---
id: "02-open-session-exec-scope"
title: "Opening editors and folders requires session.exec"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AUTH-ROLES-AND-SCOPES-001
acceptance_criteria:
  - AC-AUTH-ROLES-AND-SCOPES-001.1
  - AC-AUTH-ROLES-AND-SCOPES-001.7
system_design:
  - ../../specs/auth/system-design/editor-authorization-boundary.md
---

# Task 02: Opening editors and folders requires session.exec

## Summary

`Service.OpenEditor` and `Service.OpenFolder` authorize the session with
`session.exec` before reading anything, through an access checker wired to the
task service.

## In scope

- `apps/backend/internal/editors/service/service.go`:
  - Add `ErrSessionForbidden` next to the existing sentinels.
  - Add `sessionAccessChecker func(context.Context, string) error` to
    `Service` and `SetSessionAccessChecker`.
  - Add `authorizeSession(ctx, sessionID)`. Wired: return the checker's error.
    Unwired: allow no identity or a synthetic identity
    (`authn.IdentityFromContext`); refuse a real identity with
    `ErrSessionForbidden`.
  - Call it in `OpenEditor` and `OpenFolder` immediately after the
    empty-session-ID check, before `GetTaskSession`. In `OpenFolder` it also
    precedes `FolderOpeningAvailable`.
- `apps/backend/internal/editors/handlers/handlers.go`: `serviceErrorStatus`
  maps `ErrSessionForbidden` to 403.
- `apps/backend/internal/backendapp/services.go`: next to
  `workflowSvc.SetSessionAccessChecker`, call
  `editorSvc.SetSessionAccessChecker` with a closure that reads the session
  from `repos.Task.GetTaskSession` and checks
  `taskSvc.AuthorizeTaskScope(ctx, session.TaskID, authz.ScopeSessionExec)`.
  Map `taskservice.IsForbidden(err)` to `editorservice.ErrSessionForbidden`
  and other lookup/authorization errors to `editorservice.ErrWorkspaceNotFound`.
  Do not authorize the session's executor configuration: editor launches run
  on the backend host, and historical editor access must survive executor
  profile deletion and preserve auth-off behavior.
- Add a package-level process-start seam in the editors service (for example
  `var startProcess = func(cmd *exec.Cmd) error { return cmd.Start() }`) used by
  `launchCommand`, `openBuiltinEditor` and `OpenFolder`, so tests can assert
  that no process starts.

## Out of scope

Management routes (task 01), argument handling (task 03), UI (task 04),
target containment rules, and a WS path (editors have none).

## Acceptance

1. Service tests (`editors/service/session_access_test.go`, new): with a
   checker returning `ErrSessionForbidden`, both methods return that error,
   never call `GetTaskSession`, and start no process; the embedded VS Code
   early return is also refused. With a checker returning nil, the same calls
   succeed. Unwired: a real identity is refused, a synthetic identity and no
   identity succeed.
2. Wiring test in `backendapp` against a real task service and SQLite
   fixtures: a workspace `viewer` gets `ErrSessionForbidden` (403), a user
   without reach gets `ErrWorkspaceNotFound` (404), and a `collaborator`, the
   owner, and a synthetic identity succeed even when the completed session
   references a deleted executor profile. This is the regression test that
   fails before the change.
3. `TestServiceErrorStatus` covers `ErrSessionForbidden` mapping to 403.

## Verification

Write the wiring test first and confirm that the viewer case succeeds before
the guard is added.

```bash
(cd apps/backend && go test -count=1 ./internal/editors/...)
(cd apps/backend && CGO_ENABLED=1 go test -tags fts5 -count=1 -run 'Editor' ./internal/backendapp/...)
git ls-files --modified --others --exclude-standard -z -- 'apps/backend/**/*.go' | xargs -0 -r bash scripts/lint-go-changed
```

Mutation: remove the guard call from each method in turn, and remove the
`backendapp` wiring; the matching test must fail.

## Likely files

- `apps/backend/internal/editors/service/service.go`
- `apps/backend/internal/editors/service/folder.go` (if `OpenFolder` helpers move)
- `apps/backend/internal/editors/service/session_access_test.go` (new)
- `apps/backend/internal/editors/handlers/handlers.go`
- `apps/backend/internal/editors/handlers/handlers_test.go`
- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/backendapp/editor_session_access_test.go` (new)

## Results

Editor and folder opens now check `session.exec` before service-level session
reads or process starts. The wired check resolves the session and authorizes
task reach without checking executor configuration, since editor launches run
on the backend host. The backend test uses the real task service and SQLite;
viewers receive 403, users without reach receive 404, and collaborators,
owners, and synthetic callers succeed even when the completed session refers
to a deleted executor profile. Service tests also cover the embedded VS Code
empty-file-path case, which is denied before any session read. The handler maps
`ErrSessionForbidden` to 403, and service tests cover wired, unwired, synthetic,
and no-identity callers.

Verification passed:

- `go test -count=1 ./internal/editors/... ./internal/authz/...`
- `CGO_ENABLED=1 go test -tags fts5 -count=1 -run 'Editor' ./internal/backendapp/...`
- `git ls-files --modified --others --exclude-standard -z -- 'apps/backend/**/*.go' | xargs -0 -r bash scripts/lint-go-changed`
- `make -C apps/backend build`
