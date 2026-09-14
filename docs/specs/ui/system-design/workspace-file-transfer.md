---
status: draft
system: ui
requirements:
  - REQ-UI-WORKSPACE-FILE-TRANSFER-001
  - REQ-UI-WORKSPACE-FILE-TRANSFER-002
  - REQ-UI-WORKSPACE-FILE-TRANSFER-003
  - REQ-UI-WORKSPACE-FILE-TRANSFER-004
---

# Workspace File Transfer System Design

## Purpose and boundaries

This system owns which Files-panel surfaces offer transfer, what the person sees while a transfer
runs, and how collisions and failures are presented. It also owns the shape of the new upload
contract, because no workspace contract accepts raw bytes today.

Adjacent contracts this design consumes but does not own:

- **Workspace path containment.** `WorkspaceTracker.resolveMutationPath` and the `os.Root` rules in
  `apps/backend/internal/agentctl/server/process/workspace_files.go` decide what a workspace-relative
  path may resolve to. Upload routes through them rather than joining paths itself.
- **Attachment streaming.** The browser to backend to agentctl multipart pattern in
  `attachment_handlers.go`, `client_attachments.go`, and `server/api/attachments.go` is reused
  verbatim in structure. Its size constant is reused as the per-file limit.
- **Session to execution resolution.** Reaching an agentctl client from a session id belongs to the
  lifecycle manager, as used by `RegisterProcessRoutes`.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-WORKSPACE-FILE-TRANSFER-001` | [Components and responsibilities](#components-and-responsibilities), [Control flow](#control-flow) |
| `REQ-UI-WORKSPACE-FILE-TRANSFER-002` | [Components and responsibilities](#components-and-responsibilities), [Control flow](#control-flow) |
| `REQ-UI-WORKSPACE-FILE-TRANSFER-003` | [Security](#security), [Failure and recovery](#failure-and-recovery), [Persistence](#persistence) |
| `REQ-UI-WORKSPACE-FILE-TRANSFER-004` | [Data and contracts](#data-and-contracts), [Control flow](#control-flow) |

## Components and responsibilities

### Backend

- **`WorkspaceTracker.WriteFileStream`** (new, in `server/process/workspace_files.go`) is the single
  write-bytes primitive. It resolves the destination through the existing `resolveMutationPath`,
  takes the existing `runWorkspaceMutationBarrier`, applies the caller's resolution, creates missing
  intermediate directories under the same rooted handle, writes to a temporary file in the
  destination directory, fsyncs, renames into place, and emits the standard change notification
  through `mutationNotificationPath`. It returns the final path and byte count.
- **`WorkspaceTracker.CheckUploadConflicts`** (new, same file) resolves a list of candidate
  destination paths and reports which already exist. It performs the same containment resolution as
  the write, so an unwritable path is rejected at preflight rather than after the person has chosen a
  resolution.
- **agentctl upload handlers** (new, `server/api/workspace_upload.go`) registered beside the existing
  workspace routes in `server.go`: `POST /api/v1/workspace/file/upload-preflight` taking a JSON path
  list and returning conflicts, and `POST /api/v1/workspace/file/upload` taking the multipart body.
  The upload handler bounds the request with `http.MaxBytesReader`, reads metadata with
  `multipart.Reader`, and streams the file part into `WriteFileStream`. It uses a bounded temporary
  file only when a caller sends the file part before its metadata. It owns the HTTP status mapping for
  oversize, containment, missing-resolution, and IO failures.
- **`Client.UploadWorkspaceFile`** (new, `internal/agent/runtime/agentctl/client_workspace_upload.go`)
  mirrors `MaterializeAttachment`: an `io.Pipe`, a goroutine writing the multipart body, the
  long-running HTTP client, and a bounded error-body read.
- **Workspace file HTTP handlers** (new, `internal/task/handlers/workspace_file_http_handlers.go`)
  expose `POST /api/v1/task-sessions/:id/workspace/files`. They authenticate through `authn.FromGin`,
  resolve the session to an agentctl client through the lifecycle manager, stage one bounded file part
  so its declared size is checked before forwarding, and stream the staged file into
  `UploadWorkspaceFile`. Registered next to `RegisterProcessRoutes`.

### Frontend

- **`lib/api/domains/workspace-file-api.ts`** (new) posts the `FormData`, sets
  `credentials: "include"` and the interim settings interlock header, and raises `ApiError` on
  failure. It models one API call per file so a per-file result is available.
- **`hooks/use-file-upload.ts`** (new) owns the two-phase flow and per-file state, keyed by a
  client-side id, using the `pending | uploading | ready | failed` vocabulary already used by chat
  attachments, plus a `blocked` state for a conflict awaiting resolution.
- **`components/task/file-upload-conflict-dialog.tsx`** (new) presents the conflict set and collects
  a resolution per file, with an apply-to-all control.
- **`lib/utils/upload-selection.ts`** (new) flattens a browser selection into destination-relative
  paths. A picker folder selection carries `webkitRelativePath`; a flat multi-file pick yields bare
  names. Both normalize to the same `{ file, relativePath }` list so the hook and the dialog see one
  shape.
- **`components/task/use-file-upload-entry-points.tsx`** owns upload state and mounts the same flow for
  both Files-panel entry points. File operations keep the existing CRUD and download responsibilities.
- **`components/task/file-browser-toolbar.tsx`** turns the create control into a menu.
- **`components/task/file-context-menu.tsx`** adds the folder upload item.
- **`components/editors/monaco/monaco-editor-toolbar.tsx`** and
  **`components/editors/codemirror/codemirror-code-editor.tsx`** each gain a download control.
- **`components/task/file-editor-panel.tsx`** extends the `headerActions` fragment it already passes
  to the binary and image viewers, which lights up both screens and the mobile viewer at once.

## Data and contracts

### Preflight routes

The backend route is `POST /api/v1/task-sessions/:id/workspace/files/preflight`. It forwards JSON to
the agentctl route `POST /api/v1/workspace/file/upload-preflight`.

```json
{ "dir": "fixtures", "repo": "", "paths": ["a.json", "nested/b.json"] }
```

Response `200`:

```json
{ "conflicts": [{ "path": "fixtures/a.json", "is_dir": false }] }
```

`paths` are destination-relative, so a folder upload sends its structure here and learns about every
collision in one call. A path that fails containment is reported as an error rather than a conflict,
so it never reaches the resolution dialog.

### Upload routes

The backend route is `POST /api/v1/task-sessions/:id/workspace/files`. It forwards a multipart form to
the agentctl route `POST /api/v1/workspace/file/upload`.

| Field | Type | Meaning |
| --- | --- | --- |
| `dir` | text | Workspace-relative destination directory. Empty means the workspace root. |
| `relative_path` | text | Path of this file beneath `dir`, for a folder upload. A bare name for a flat upload. |
| `repo` | text | Optional repository subpath for a multi-repository task. |
| `size_bytes` | text | Declared size, validated against the received byte count. |
| `resolution` | text | `replace`, `keep_both`, or absent when the preflight found no conflict. |
| `file` | file | The content. Streamed, never buffered whole. |

Response `201`:

```json
{ "path": "fixtures/a-1.json", "size_bytes": 20481, "resolution_applied": "keep_both" }
```

A file resolved as **Skip** is never sent. If the destination exists and no `resolution` was
supplied, the request is refused with `409` rather than overwriting, which is what makes
`AC-UI-WORKSPACE-FILE-TRANSFER-004.6` hold even when the workspace changes between the two phases.

One request carries one file; a selection issues one request per file so that one rejection does not
fail the rest, satisfying `AC-UI-WORKSPACE-FILE-TRANSFER-003.2`.

### Limits

The per-file cap is `models.MaxMessageAttachmentBytes`, the same constant chat attachments use. The
agentctl request bound follows the existing precedent of that constant plus a multipart overhead
allowance, as `maxMaterializedAttachmentRequestBytes` already does.

## Control flow

Upload is two-phase, so that no byte is written before the person has decided what happens to every
conflict:

1. A Files-panel surface collects a selection and a destination folder. `upload-selection`
   normalizes it to `{ file, relativePath }`, flattening a picked folder through
   `webkitRelativePath`.
2. `useFileUpload` posts the destination-relative path list to the preflight route.
3. No conflicts: proceed straight to step 5.
4. Conflicts: the dialog collects a resolution per file. Cancelling ends the flow having written
   nothing. **Skip** removes that file from the batch.
5. Each remaining file is marked `uploading`, gets an optimistic tree node, and is posted as one
   multipart request carrying its `relative_path` and its `resolution`.
6. The backend authenticates, resolves the session to an agentctl client, and streams the part into
   `UploadWorkspaceFile`, which pipes a fresh multipart body to agentctl.
7. The agentctl handler streams into `WriteFileStream`, which resolves, barriers, creates missing
   intermediate directories, applies the resolution, writes to a temporary file, fsyncs, renames,
   and notifies.
8. The response path replaces the optimistic node; a failure removes it and raises a toast.

The preflight is advisory, not a lock. It exists to let one person make an informed decision, and the
`409` on an unresolved existing destination is what keeps the write itself safe if the workspace
changed underneath it.

Download needs no new transport. The viewer already holds the content that
`workspace.file.get` returned, so the button hands that content to the existing
`triggerFileDownload`, which base64-decodes binary content and clicks an anchor. This is why
`REQ-UI-WORKSPACE-FILE-TRANSFER-002` carries no backend work: `useFileLoader` fetches before either
viewer renders and `resolveFileCategory` only chooses a viewer after that fetch resolves, so any
file that can display a download button is already loaded and already under the read cap.

## Failure and recovery

- **Oversize.** Rejected at the backend edge with a size-specific status before the workspace is
  touched. Reported against the individual file.
- **Containment rejection.** `resolveMutationPath` refuses; nothing is written; the response
  distinguishes this from an IO error.
- **Transport failure mid-stream.** The temporary file is removed on any error path, so the
  destination is never left holding a truncated file.
- **Conflict.** Not a failure, and not resolved by the server on its own. Preflight reports it, the
  person decides, and the write refuses with `409` if it arrives without a decision.
- **Conflict appearing after preflight.** The `409` catches it. The UI re-runs the preflight for the
  remaining files rather than guessing.
- **No active session.** The surfaces are not rendered, so there is no request to fail.
- **Partial multi-file selection.** Each file reports independently; successful files stay.
- **Unreadable entry in a picked folder.** That entry is reported and skipped; the rest of the
  selection still uploads. One bad entry never aborts the whole batch.

## Persistence

Uploaded content lands in the task workspace on disk and has no database representation. There is no
migration, no retention policy, and no restart behavior: a file that finished writing is an ordinary
workspace file from that moment on, indistinguishable from one the agent wrote. Collision handling is
explicit: **Replace** can overwrite an existing file, while **Keep both** and **Skip** preserve it. The
workspace has no undo record, so the UI keeps **Keep both** as the safe default.

## Security

Upload is the first path that writes caller-supplied bytes into a workspace, so containment is the
load-bearing control:

- Every destination resolves through `resolveMutationPath` and its `os.Root` handle. The handler
  never joins paths itself, so traversal, absolute paths, and symlinked directories are refused by
  the same code that already guards create, rename, and delete.
- Folder upload widens the attack surface, because the client supplies interior path segments rather
  than a single file name. Every segment of a `relative_path` is resolved through the same
  rooted handle, and intermediate directory creation happens through that handle too, so a crafted
  `relative_path` cannot escape by traversal or by landing on an existing symlink.
- The backend route requires an authenticated identity through `authn.FromGin`, matching the
  attachment upload guard.
- Request bodies are bounded by `http.MaxBytesReader` before parsing, so an oversize body cannot be
  used to exhaust memory.
- Content is streamed through `io.Pipe` and `io.Copy` at every network hop. Metadata and error bodies
  use bounded reads, and no layer base64-encodes file content. This keeps memory bounded for large
  files.

This capability does not widen the workspace trust boundary. The task workspace is already fully
writable by the agent running in it, so upload adds a new route to a capability that exists, under
authentication the agent path does not require. The marginal risk is therefore containment
correctness rather than a new class of access, which is why the write is expressed as a thin
addition to the existing mutation primitives rather than a new subsystem behind a rollout toggle.

## Observability

Failed uploads log at the agentctl and backend boundaries with the workspace-relative path and the
failure class, following the existing workspace mutation handlers. Successful uploads are not
logged individually, matching the surrounding file mutations. No new metric is introduced; the
existing workspace change notification is the signal that a write landed.

## Related decisions

No new architecture decision record is required. The transport reuses the attachment streaming
pattern, and the write reuses the workspace mutation primitives; neither establishes a new boundary.
