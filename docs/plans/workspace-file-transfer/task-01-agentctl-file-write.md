---
id: "01-agentctl-file-write"
title: "agentctl streamed workspace file write and conflict preflight"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-WORKSPACE-FILE-TRANSFER-001
  - REQ-UI-WORKSPACE-FILE-TRANSFER-003
  - REQ-UI-WORKSPACE-FILE-TRANSFER-004
acceptance_criteria:
  - AC-UI-WORKSPACE-FILE-TRANSFER-001.4
  - AC-UI-WORKSPACE-FILE-TRANSFER-003.1
  - AC-UI-WORKSPACE-FILE-TRANSFER-003.2
  - AC-UI-WORKSPACE-FILE-TRANSFER-003.4
  - AC-UI-WORKSPACE-FILE-TRANSFER-003.5
  - AC-UI-WORKSPACE-FILE-TRANSFER-003.6
  - AC-UI-WORKSPACE-FILE-TRANSFER-004.1
  - AC-UI-WORKSPACE-FILE-TRANSFER-004.4
  - AC-UI-WORKSPACE-FILE-TRANSFER-004.6
system_design:
  - ../../specs/ui/system-design/workspace-file-transfer.md
---

# Task 01: agentctl streamed workspace file write and conflict preflight

## Summary

Add the single write-bytes primitive the workspace has never had, its conflict preflight, and the two
agentctl routes over them. This is the bottom of the upload stack; nothing above it can exist first.

## Scope

- `WorkspaceTracker.WriteFileStream` in `server/process/workspace_files.go`, taking a destination, a
  resolution, and an `io.Reader`. It resolves through the existing `resolveMutationPath`, holds
  `runWorkspaceMutationBarrier`, creates missing intermediate directories through the same rooted
  handle, applies the resolution, writes to a temporary file in the destination directory, fsyncs,
  renames into place, and emits the change notification through `mutationNotificationPath`.
- Resolutions: `replace` overwrites; `keep_both` selects the next free `name-<n>.ext`, reusing the
  search in `installMaterializedAttachment`; an absent resolution against an existing destination is
  an error, never a silent overwrite.
- `WorkspaceTracker.CheckUploadConflicts`, resolving a candidate path list through the same rules and
  reporting which already exist. A containment failure is an error, not a conflict.
- `POST /api/v1/workspace/file/upload-preflight` (JSON) and `POST /api/v1/workspace/file/upload` (multipart) in a
  new `server/api/workspace_upload.go`, registered beside the existing workspace file routes. The
  upload route is bounded by `http.MaxBytesReader` and streams the `file` part.
- Status mapping: `413` oversize, `409` unresolved existing destination, `400` containment rejection
  or size mismatch, `500` IO.

## Exclusions

- The backend client method and the backend HTTP routes. Those are task 03.
- Any frontend change.

## Acceptance

- A destination that escapes the workspace by traversal, absolute path, symlinked directory, or a
  `.git/` target is rejected and writes nothing. This holds for every interior segment of a folder
  upload's relative path, not only the destination folder.
- Each resolution behaves as specified, and an absent resolution against an existing destination
  errors rather than overwriting. Concurrent uploads choosing `keep_both` for the same name do not
  collide, because the name search and the write share one mutation barrier.
- A reader that fails mid-stream leaves no file at the destination and no temporary file behind.

## Verification

Write the containment table test first, including the crafted interior-segment case, and confirm it
fails before the production change. Then:

```bash
cd apps/backend
go test ./internal/agentctl/server/process -run 'TestWriteFileStream|TestCheckUploadConflicts'
go test ./internal/agentctl/server/api -run 'TestHandleFileUpload|TestHandleUploadPreflight'
gofmt -l internal/agentctl/server/process internal/agentctl/server/api
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/workspace_files.go`
- `apps/backend/internal/agentctl/server/process/workspace_files_test.go`
- `apps/backend/internal/agentctl/server/api/workspace_upload.go`
- `apps/backend/internal/agentctl/server/api/workspace_upload_test.go`
- `apps/backend/internal/agentctl/server/api/server.go`

## Dependencies

None.

## Parallelism

Parallel-safe with task 02, which shares no file and no contract.

## Inputs

- Requirements: `REQ-UI-WORKSPACE-FILE-TRANSFER-003` and `-004`.
- System design: `Components and responsibilities > Backend`, `Data and contracts`, `Security`.
- Existing patterns: `CreateFile` and `DeleteFile` for the mutation shape,
  `handleMaterializeAttachment` for the bounded multipart handler, and
  `installMaterializedAttachment` for the `name-<n>.ext` search.

## Risks

- Resolving paths in the handler instead of delegating to `resolveMutationPath` reopens traversal and
  symlink escape. Folder upload makes this sharper: the client now supplies interior segments.
- Creating intermediate directories outside the rooted handle defeats the containment guarantee even
  when the final path check passes.
- Selecting the `keep_both` name outside the mutation barrier makes concurrent uploads race.
- Buffering the part to check its size defeats streaming. Validate against the byte count as it
  streams.
- Treating preflight as authoritative would let a race reintroduce silent overwrite. The `409` is the
  real guarantee.

## Output contract

Report the primitive's signature, the resolution semantics, the status mapping, files changed, exact
commands and results, then mark this task `done` and update its checkbox in `plan.md`.

## Results

Implemented on `v0.92.2-68-gc51ec0a21`.

- `WriteFileStream` and `CheckUploadConflicts` in new
  `apps/backend/internal/agentctl/server/process/workspace_upload.go`, with `UploadResolution`
  (`""` / `replace` / `keep_both`) and a distinct `ErrUploadConflict` so the HTTP layer can map 409.
- Staging is a `.kandev-upload-<pid>-<n>` temporary in the destination directory, opened through the
  rooted handle, then fsync and rename. Every error path removes it.
- `keep_both` produces `name-1.ext`, matching the existing attachment installer. The Windows
  `name (1).ext` form was considered and rejected: these land in a code workspace, where a space and
  parentheses need quoting in every shell command and show up in git paths.
- Two agentctl routes in new `server/api/workspace_upload.go`, registered in `server.go` beside
  `/workspace/file/create`.

**Correction made during implementation:** the first cut of `scopedUploadPath` normalized `../x` to
`x` instead of rejecting it. It was contained, but it silently relocated the file, which contradicts
`AC-UI-WORKSPACE-FILE-TRANSFER-003.1`. Replaced with `sanitizeUploadSegment`, which rejects absolute
paths and any `..` component outright. The containment test caught this.

### Commands

```text
go test ./internal/agentctl/server/process -run 'TestWriteFileStream|TestCheckUploadConflicts'  ok (16 subtests)
go test ./internal/agentctl/server/api -run 'TestHandleFileUpload|TestHandleUploadPreflight'    ok (10 tests)
go test ./internal/agentctl/server/process ./internal/agentctl/server/api                       ok
gofmt -l ...                                                                                    clean
golangci-lint run ...                                                                           0 issues
```
