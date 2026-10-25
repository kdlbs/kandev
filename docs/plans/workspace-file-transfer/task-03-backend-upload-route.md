---
id: "03-backend-upload-route"
title: "Backend upload client and session route"
status: done
wave: 2
depends_on:
  - "01-agentctl-file-write"
plan: "plan.md"
requirements:
  - REQ-UI-WORKSPACE-FILE-TRANSFER-001
  - REQ-UI-WORKSPACE-FILE-TRANSFER-003
  - REQ-UI-WORKSPACE-FILE-TRANSFER-004
acceptance_criteria:
  - AC-UI-WORKSPACE-FILE-TRANSFER-003.2
  - AC-UI-WORKSPACE-FILE-TRANSFER-003.3
  - AC-UI-WORKSPACE-FILE-TRANSFER-003.5
  - AC-UI-WORKSPACE-FILE-TRANSFER-004.1
system_design:
  - ../../specs/ui/system-design/workspace-file-transfer.md
---

# Task 03: Backend upload client and session route

## Summary

Connect the browser to the agentctl write path from task 01: a streaming client method and the
authenticated session-scoped HTTP route that uses it.

## Scope

- `Client.UploadWorkspaceFile` and `Client.PreflightWorkspaceUpload` in a new
  `client_workspace_upload.go`. The upload method copies the structure of `MaterializeAttachment`:
  `io.Pipe`, a goroutine writing the multipart body, the long-running HTTP client, a bounded
  error-body read.
- `POST /api/v1/task-sessions/:id/workspace/files` and
  `POST /api/v1/task-sessions/:id/workspace/files/preflight` in a new
  `internal/task/handlers/workspace_file_http_handlers.go`, authenticating through `authn.FromGin`,
  resolving the session to an agentctl client through the lifecycle manager, and forwarding a bounded
  temporary file after the declared size is checked.
- Registration beside `taskhandlers.RegisterProcessRoutes` in `backendapp/helpers.go`.
- The per-file cap is `models.MaxMessageAttachmentBytes`.

## Exclusions

- Any frontend change.
- Changing the agentctl handler or the write primitive from task 01.
- A new WebSocket action. The transport is HTTP because the WebSocket dispatcher would force base64
  and whole-file buffering.

## Acceptance

- Both routes refuse an unauthenticated request and a request for an unknown or unreachable session,
  before anything is streamed.
- The preflight forwards its path list and returns the conflict set unchanged; the upload forwards
  the resolution so an unresolved existing destination still surfaces as `409`.
- A single file above the attachment limit is rejected with a size-specific status, and one request
  carries one file so a rejection does not affect other files in a selection.
- Network bodies are streamed and bounded. Metadata and error responses use limited reads, and
  nothing is base64-encoded; file bytes do not enter a whole-file memory buffer.

## Verification

Write the auth-refusal and oversize tests first and confirm they fail before the production change.
Then:

```bash
cd apps/backend
go test ./internal/agent/runtime/agentctl -run 'TestUploadWorkspaceFile|TestPreflightWorkspaceUpload'
go test ./internal/task/handlers -run 'TestWorkspaceFileUpload|TestWorkspaceFilePreflight'
gofmt -l internal/agent/runtime/agentctl internal/task/handlers internal/backendapp
```

## Files likely touched

- `apps/backend/internal/agent/runtime/agentctl/client_workspace_upload.go` and its test
- `apps/backend/internal/task/handlers/workspace_file_http_handlers.go` and its test
- `apps/backend/internal/backendapp/helpers.go`

## Dependencies

Task 01.

## Parallelism

Sequential. It calls the route task 01 introduces.

## Inputs

- Requirements: `REQ-UI-WORKSPACE-FILE-TRANSFER-003`.
- System design: `Data and contracts`, `Control flow`, `Security`.
- Existing patterns: `MaterializeAttachment` for the streaming client, `AttachmentHandlers.owner` for
  the auth guard, `RegisterProcessRoutes` for session-to-execution resolution and registration.

## Risks

- Buffering the part to compute a size defeats streaming. Pass the declared size through and let the
  agentctl layer validate against received bytes.
- agentctl may not be same-host in a containerized or remote executor. Stay on the existing agentctl
  HTTP client, which already handles that.
- If a rollout toggle is wanted after all, this route is the natural gate; add it here rather than
  retrofitting later.

## Output contract

Report the route path, its auth and size behavior, files changed, exact commands and results, then
mark this task `done` and update its checkbox in `plan.md`.

## Results

- `UploadWorkspaceFile` and `PreflightWorkspaceUpload` in new
  `internal/agent/runtime/agentctl/client_workspace_upload.go`, copying `MaterializeAttachment`'s
  `io.Pipe` + goroutine multipart structure. `ErrWorkspaceUploadConflict` is a sentinel so a 409
  stays distinguishable from a failure.
- `POST /api/v1/task-sessions/:id/workspace/files` and `.../files/preflight` in new
  `internal/task/handlers/workspace_file_http_handlers.go`, guarded by the existing
  `denySessionAccess` and resolving the client through `execution.AcquireAgentCtlClient()` after
  bounded file validation.
- `RegisterProcessRoutes` now returns `*ProcessHandlers` so `helpers.go` can register the workspace
  routes beside it without constructing a second handler set.
- The upload handler validates `size_bytes` against the actual part size and caps at
  `models.MaxMessageAttachmentBytes`; a session with no live execution returns 503.

### Commands

```
go test ./internal/agent/runtime/agentctl -run 'TestUploadWorkspaceFile|TestPreflightWorkspaceUpload'  ok
go test ./internal/task/handlers -run 'TestWorkspaceFile|TestRegisterWorkspaceFileRoutes'              ok (6)
go test ./internal/task/handlers ./internal/agent/runtime/agentctl                                     ok
go build ./...                                                                                         ok
gofmt -l internal/                                                                                     clean
golangci-lint run ./internal/task/handlers/... ./internal/agent/runtime/agentctl/...                   0 issues
```

The cross-user tests assert the denial lands **before** agentctl is contacted, with the owner request
as the witness that the 404 is scoping rather than an unrelated failure.
