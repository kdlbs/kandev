---
id: "01-workspace-file-severity"
title: "Workspace file missing-file severity"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/expected-runtime-log-severity.md"
---

# Task 01: Workspace file missing-file severity

## Acceptance

- A missing current-checkout file returned by agentctl produces a
  `ws.ErrorCodeNotFound` response and a debug-level handler entry.
- A non-missing file-content failure still produces
  `ws.ErrorCodeInternalError` and an error-level entry.
- The existing at-ref missing-file behavior and all successful file responses
  remain unchanged.

## Verification

```bash
cd apps/backend && go test ./internal/agent/handlers -run 'TestWorkspaceFileHandlers(Missing|NonMissing).*' -count=1
```

## Files likely touched

- `apps/backend/internal/agent/handlers/workspace_file_handlers.go`
- `apps/backend/internal/agent/handlers/workspace_file_handlers_test.go`

## Dependencies

None.

## Parallelism

Sequential.

## Inputs

- Spec: workspace file classification scenarios and failure modes.
- Existing `workspaceHandlerServer` test helper and the at-ref missing-file
  classification in `workspace_file_handlers.go`.
- Agentctl missing-file responses use HTTP 404 and the runtime client exposes a
  typed `ErrFileNotFound` sentinel; non-missing responses retain their status.

## Output contract

Report the classification helper or branch changed, observer assertions, exact
test result, files changed, blockers, and task/plan status updates.

## Results

Implemented the missing current-checkout file classification in
`wsGetFileContent`. Agentctl file-not-found responses now return
`ws.ErrorCodeNotFound` and emit one debug-level entry; non-missing failures
remain `ws.ErrorCodeInternalError` and error-level.

- RED: `cd apps/backend && go test ./internal/agent/handlers -run
  'TestWorkspaceFileHandlers(Missing|NonMissing).*' -count=1` failed because
  the missing-file response was `INTERNAL_ERROR` instead of `NOT_FOUND`.
- GREEN: the same command passed after the handler change and after `gofmt`.
- PR fixup: replaced the error-string classifier with the typed client
  sentinel, added the HTTP 404 status mapping, and preserved non-ENOENT stat
  failures as ordinary errors. Added client, server, and process regression
  coverage for the boundary and permission path.
- PR fixup verification passed:
  `cd apps/backend && go test ./internal/agent/runtime/agentctl -run
  'TestRequestFileContent_(Surfaces|DoesNot).*' -count=1`;
  `cd apps/backend && go test ./internal/agentctl/server/process -run
  TestReadFileContent_PermissionErrorIsNotMissing -count=1`;
  `cd apps/backend && go test ./internal/agentctl/server/api -run
  TestHandleFileContent_Rejections -count=1`; and
  `cd apps/backend && go test ./internal/agent/handlers -run
  'TestWorkspaceFileHandlers(Missing|NonMissing).*' -count=1`.
