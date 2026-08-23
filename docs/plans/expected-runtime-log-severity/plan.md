---
spec: docs/specs/platform/expected-runtime-log-severity.md
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

- Update `apps/backend/internal/agent/handlers/workspace_file_handlers.go` in
  `wsGetFileContent` to recognize the existing agentctl missing-file error
  wording, log the expected condition at debug level, and return
  `ws.ErrorCodeNotFound`.
- Leave non-missing errors on the current error-level and
  `ws.ErrorCodeInternalError` path.
- Add focused observer-backed coverage in
  `apps/backend/internal/agent/handlers/workspace_file_handlers_test.go` for
  both the missing-file and genuine-failure paths.

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

## Implementation Waves And Parallel Candidates

Execute sequentially in the primary session. The tasks touch disjoint backend
packages, but no delegation is authorized by this plan.

- [x] [task-01-workspace-file-severity](task-01-workspace-file-severity.md)
- [x] [task-02-worktree-initialization-severity](task-02-worktree-initialization-severity.md)

## Open Questions

None.
