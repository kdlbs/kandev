---
spec: docs/specs/platform/requirements/expected-runtime-log-severity.md
created: 2026-08-23
status: implemented
---

# Implementation Plan: Expected runtime log severity

## Overview

Classify two confirmed normal conditions at their correct severity. First,
workspace file reads will expose missing current-checkout paths as `NotFound`
and debug evidence. Second, initial worktree persistence that races task
environment creation will remain successful but emit debug evidence instead of
a warning. Non-missing file failures and non-environment worktree failures keep
their existing behavior.

## Backend

### Workspace file content classification

- Preserve the server-side `fs.ErrNotExist` distinction in
  `apps/backend/internal/agentctl/server/process/workspace_files.go` and carry
  it across the agentctl HTTP boundary as HTTP 404.
- Map that status to a typed client sentinel, then have
  `apps/backend/internal/agent/handlers/workspace_file_handlers.go` use the
  sentinel in `wsGetFileContent` to log the expected condition at debug level
  and return `ws.ErrorCodeNotFound`.
- Leave non-missing errors on the current error-level and
  `ws.ErrorCodeInternalError` path.
- Add focused coverage for the server status, client sentinel, handler logs,
  and genuine stat-failure path.

### Initial worktree persistence severity

- Update `apps/backend/internal/worktree/manager_state.go` so only the
  `ErrEnvironmentNotResolved` branch uses debug severity. Keep the message
  fields and return behavior stable.
- Add observer-backed coverage in
  `apps/backend/internal/worktree/manager_state_test.go` for the typed
  environment-not-resolved branch and verify that unrelated store errors still
  follow the current error path.

## Tests

- **What:** Missing current-checkout file returns `not_found` and emits debug,
  while a non-missing dependency failure remains `internal_error` and error.
  **File:** `apps/backend/internal/agent/handlers/workspace_file_handlers_test.go`.
  **How:** Use the real `agentctl.Client` against an `httptest.Server` and a
  zap observer logger attached to the handler.
- **What:** Initial worktree persistence is debug-only and remains successful;
  other store errors remain failures.
  **File:** `apps/backend/internal/worktree/manager_state_test.go`.
  **How:** Call the real `Manager.persistWorktree` with a focused fake store
  returning typed errors and an observer logger. Do not create a Git worktree.

## Verification Results

- `cd apps/backend && go test ./internal/agent/handlers -run
  'TestWorkspaceFileHandlers(Missing|NonMissing).*' -count=1` passed after the
  workspace-file change and final formatting.
- `cd apps/backend && go test ./internal/worktree -run
  'TestPersistWorktree_(EnvironmentNotResolved|OtherStoreError)' -count=1`
  passed after the worktree change and final formatting.
- `cd apps/backend && go test ./internal/agent/handlers ./internal/worktree
  -count=1` passed: both packages green.
- PR fixup RED: the client and process tests initially failed to compile
  without the new `ErrFileNotFound` sentinels; the API rejection test reported
  a missing file as 400; and the handler regression classified a 400 response
  containing a legacy `file not found` message as `NOT_FOUND`.
- PR fixup GREEN:
  `cd apps/backend && go test ./internal/agent/runtime/agentctl -run
  'TestRequestFileContent_(Surfaces|DoesNot).*' -count=1` passed;
  `cd apps/backend && go test ./internal/agentctl/server/process -run
  TestReadFileContent_PermissionErrorIsNotMissing -count=1` passed;
  `cd apps/backend && go test ./internal/agentctl/server/api -run
  TestHandleFileContent_Rejections -count=1` passed; and
  `cd apps/backend && go test ./internal/agent/handlers -run
  'TestWorkspaceFileHandlers(Missing|NonMissing).*' -count=1` passed.

## Implementation Waves And Parallel Candidates

Execute sequentially in the primary session. The tasks touch disjoint backend
packages, but no delegation is authorized by this plan.

- [x] [task-01-workspace-file-severity](task-01-workspace-file-severity.md)
- [x] [task-02-worktree-initialization-severity](task-02-worktree-initialization-severity.md)

## Open Questions

None.
