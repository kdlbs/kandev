---
spec: docs/specs/ui/requirements/workspace-file-transfer.md
created: 2026-09-01
status: draft
---

# Implementation Plan: Workspace File Transfer

## Overview

Give the Files panel a working upload path and put download where people actually look for it.

The work splits cleanly along a dependency line. Download is pure frontend: the viewer already holds
the file content, so adding the buttons touches no transport and can land first as a standalone
change. Upload needs a new streamed write path built bottom up, because no workspace contract accepts
raw bytes today.

Order: the agentctl write primitive and its conflict preflight, then the backend routes over them,
then the web client and hook, then the resolution dialog, then the panel entry points. Download runs
alongside from the start. End-to-end coverage lands once both halves are in.

Drag and drop from the desktop was announced in review and is deliberately **not** in this package.
The file tree already owns HTML5 drag and drop for internal moves, there are other known side-panel
issues outstanding, and adding an external-drop branch on top of both is a regression risk that buys
nothing the menu and context-menu routes do not already deliver.

Verified against `v0.92.2-68-gc51ec0a21`.

## Decisions and where they came from

| Question | Decision | Source |
| --- | --- | --- |
| Scope | Upload and download both, in full | Requested; review confirmed both are wanted |
| Size cap | `models.MaxMessageAttachmentBytes` | Consistency with chat attachments |
| Name collision | Preflight the selection, then prompt per file with Replace / Keep both / Skip | Review: report conflicts and ask before uploading anything |
| Folder upload | In scope, via the picker | Review: do it if it is not hard; both respondents wanted it |
| Drag and drop from the OS | Out of scope | Deferred: other side-panel issues make touching the tree's DnD risky |
| Rollout toggle | None | The workspace is already agent-writable; see the system design |

The collision and folder decisions reverse the earlier answers of auto-rename and flat-files-only.
Both changed on review feedback. Auto-rename survives as the **Keep both** resolution rather than as
the silent default.

---

## Backend

### Write primitive and preflight

- Add `WriteFileStream` to `apps/backend/internal/agentctl/server/process/workspace_files.go`, taking
  the destination, a resolution, and an `io.Reader`.
- Resolve through the existing `resolveMutationPath` so containment, the `os.Root` handle, and
  symlink rules match `CreateFile` and `DeleteFile`. Do not join paths in the handler. **Every
  segment of a folder upload's relative path goes through the same rooted resolution.**
- Create missing intermediate directories through the same rooted handle.
- Take `runWorkspaceMutationBarrier` as the other mutators do.
- Apply the resolution: `replace` overwrites, `keep_both` selects the next free `name-<n>.ext` reusing
  the search `installMaterializedAttachment` already implements, and an absent resolution against an
  existing destination is an error, never a silent overwrite.
- Write to a temporary file in the destination directory, fsync, then rename into place. Remove the
  temporary file on every error path.
- Emit the standard change notification through `mutationNotificationPath`.
- Add `CheckUploadConflicts`, resolving a candidate path list through the same containment rules and
  reporting which already exist. Containment failures are reported as errors, not conflicts.

### agentctl routes

- New `apps/backend/internal/agentctl/server/api/workspace_upload.go`, registered beside the existing
  workspace file routes in `server.go`:
  - `POST /api/v1/workspace/file/upload-preflight`, JSON in and out.
  - `POST /api/v1/workspace/file/upload`, multipart, structured like `handleMaterializeAttachment`:
    bound the body with `http.MaxBytesReader`, read metadata with `multipart.Reader`, validate the
    declared size, and stream the `file` part into `WriteFileStream`.
- Status mapping: `413` oversize, `409` unresolved existing destination, `400` containment rejection
  or size mismatch, `500` IO.

### Backend client and HTTP routes

- New `apps/backend/internal/agent/runtime/agentctl/client_workspace_upload.go` with
  `UploadWorkspaceFile` and `PreflightWorkspaceUpload`. The upload method copies the structure of
  `MaterializeAttachment` in `client_attachments.go`: `io.Pipe`, a goroutine writing the multipart
  body, the long-running HTTP client, a bounded error-body read. No base64 anywhere.
- New `apps/backend/internal/task/handlers/workspace_file_http_handlers.go` exposing
  `POST /api/v1/task-sessions/:id/workspace/files` and
  `POST /api/v1/task-sessions/:id/workspace/files/preflight`. Authenticate through `authn.FromGin` as
  `AttachmentHandlers.owner` does, resolve the session to an agentctl client through the lifecycle
  manager as `RegisterProcessRoutes` does, and stage the bounded file part before forwarding it to the
  client method.
- Register beside `taskhandlers.RegisterProcessRoutes` in
  `apps/backend/internal/backendapp/helpers.go`.

HTTP rather than a new WebSocket action, because the WebSocket dispatcher is JSON request and
response, so bytes would have to be base64-encoded and buffered whole. That is the same defect the
existing download already has, and attachments set the HTTP precedent.

---

## Frontend

### Selection normalization

- New `apps/web/lib/utils/upload-selection.ts` flattening any selection to `{ file, relativePath }`.
  A picked folder carries `webkitRelativePath`; a flat multi-file pick yields bare names. Both
  normalize to one shape so the hook, the dialog, and the API client never branch on how the files
  were chosen.
- An unreadable entry is reported and skipped rather than aborting the batch.

### Upload client and state

- New `apps/web/lib/api/domains/workspace-file-api.ts` with `preflightWorkspaceUpload` and
  `uploadWorkspaceFile`, mirroring `attachment-api.ts`: `FormData` for the upload, JSON for the
  preflight, `credentials: "include"`, the interim settings interlock header from
  `readInterimSettingsInterlockToken()`, and `ApiError` on failure.
- One request per file, so one rejected file does not fail the rest of a selection.
- New `apps/web/hooks/use-file-upload.ts` owning the two-phase flow and returning
  `{ uploadFiles(dir, selection), uploads, conflicts, resolveConflicts, cancel }`. Per-file status
  uses the `pending | uploading | ready | failed` vocabulary already in
  `components/task/chat/file-attachment.ts`, plus `blocked` for a conflict awaiting a decision.
- Keep upload state in `apps/web/components/task/use-file-upload-entry-points.tsx`, which mounts one
  flow for both Files-panel entry points. `use-file-operations.ts` remains the owner of CRUD and
  download calls.
- On success, insert with `insertNodeInTree` as `handleCreateFileSubmit` already does, using the
  server-reported path; on failure, remove the optimistic node and toast.

### Conflict resolution dialog

- New `apps/web/components/task/file-upload-conflict-dialog.tsx` listing conflicting paths with
  **Replace**, **Keep both**, and **Skip** per file, plus an apply-to-all control.
- Cancelling uploads nothing, including the non-conflicting files.
- Use the existing dialog primitives; `file-delete-confirmation.tsx` is the nearest neighbour for a
  destructive-choice dialog in this panel.

### Upload entry points

- `apps/web/components/task/file-browser-toolbar.tsx`: the create control becomes a `DropdownMenu`
  with **New File**, **Upload Files**, and **Upload Folder**. Use the sibling `WorkspaceActionsMenu`
  in the same file as the template, since it already carries the trigger, tooltip,
  `min-h-11 ... sm:min-h-8` touch sizing, and the `onCloseAutoFocus` handling. New File must keep
  firing `onStartCreate` with identical semantics.
- `apps/web/components/task/file-context-menu.tsx`: add **Upload files here**, guarded as the exact
  inverse of the existing download guard at line 114, so it appears for a folder and not for a file
  or a multi-selection. Thread `onUploadFiles` down the same prop chain as `onDownloadFile`.
- Folder picking uses a second hidden input carrying `webkitdirectory`.

### Download surfaces

- `apps/web/components/editors/monaco/monaco-editor-toolbar.tsx`: add a download control shaped like
  its `DeleteButton` neighbour, placed between the open-with dropdown and delete, driven by a new
  optional `onDownload` prop.
- `apps/web/components/editors/codemirror/codemirror-code-editor.tsx`: the same control in the
  parallel toolbar beside `CodeMirrorDeleteButton`. Doing only one editor recreates the
  inconsistency this change exists to remove.
- `apps/web/components/task/file-editor-panel.tsx`: append a download control to the `headerActions`
  fragment already passed to `FileBinaryViewer` and `FileImageViewer`. Both viewers and
  `components/task/mobile/mobile-file-viewer-panel.tsx` consume the same contract, so one change
  covers all three.
- Download from the content already in the dockview store. No refetch.

### Mobile design contract

- **Desktop outcome and phone entry:** both platforms use the same Files panel and the same viewer
  header. There is no separate mobile path to build.
- **Nearest exemplar:** `WorkspaceActionsMenu` in the file browser toolbar supplies the established
  touch target sizing and dropdown behavior for the create menu; `FileViewerHeader` supplies the
  viewer action row; `file-delete-confirmation.tsx` supplies the dialog shape.
- **Hierarchy:** upload is an action on the panel chrome; download is an action on the file being
  viewed. Neither introduces new chrome.
- **Presentation:** a dropdown rather than a modal for the create control, matching the adjacent
  actions menu, so every action stays one tap away and no focus trap is introduced. The conflict
  dialog is a real dialog because it is a decision, not a navigation.
- **Geometry:** existing panel and viewer layout is authoritative. No new scroll owner. The conflict
  list scrolls within the dialog.

### Internationalization

New keys in `apps/web/src/locales/en/task.json`: `uploadFiles`, `uploadFolder`, `uploadFilesHere`,
`uploadComplete_one`, `uploadComplete_other`, `uploadedTo`, `failedToUploadFile`,
`uploadPartiallyFailed_one`, `uploadPartiallyFailed_other`, `uploadConflictTitle_one`,
`uploadConflictTitle_other`, `uploadConflictBody`, `uploadConflictApplyToAll`, `uploadConflictKeepBoth`,
`uploadConflictReplace`, `uploadConflictSkip`, `uploadConflictConfirm`. New
`downloadFile` in `apps/web/src/locales/en/editors.json`. `newFile` and `task:download` already
exist.

Per the repository internationalization rules, every one of these must also land in `pt-pt`,
`zh-cn`, `zh-hk`, and `zh-tw`, since `check-i18n-keys.mjs` fails on a missing key, a dropped
placeholder, or a value left identical to English. Use `pnpm run i18n:zh-hant` for the Traditional
Chinese pair. No em dashes in any catalog value. Conflict counts use `count` with `_one` / `_other`,
never a concatenated plural.

---

## Tests

- **Write primitive containment, resolution, and atomicity**
  - **File:** `apps/backend/internal/agentctl/server/process/workspace_files_test.go`
  - **How:** table tests for `..`, absolute paths, a symlinked directory, a `.git/` target, and a
    crafted interior segment in a folder upload's relative path, each asserting nothing is written.
    Separate tests for each resolution, for an absent resolution against an existing destination
    erroring rather than overwriting, for a failed source reader leaving no destination file, for
    intermediate directory creation, and for the change notification firing.
- **Preflight**
  - **File:** same
  - **How:** a mixed list returns exactly the existing paths as conflicts, and a containment failure
    is reported as an error rather than a conflict.
- **agentctl handlers**
  - **File:** `apps/backend/internal/agentctl/server/api/workspace_upload_test.go`
  - **How:** oversize returns `413`, unresolved existing destination returns `409`, containment
    rejection returns `400`, a well-formed request returns `201` with the written path, and a
    declared size that disagrees with the received bytes is rejected.
- **Backend client round trip**
  - **File:** `apps/backend/internal/agent/runtime/agentctl/client_workspace_upload_test.go`
  - **How:** mirror `client_attachments_test.go` against an `httptest` server, asserting the
    multipart body arrives intact and the error body is surfaced.
- **Backend HTTP routes**
  - **File:** `apps/backend/internal/task/handlers/workspace_file_http_handlers_test.go`
  - **How:** unauthenticated is refused, unknown session is refused, a valid request forwards.
- **Selection normalization**
  - **File:** `apps/web/lib/utils/upload-selection.test.ts`
  - **How:** `webkitRelativePath` entries flatten to the expected shape, nested directories keep
    their relative paths, and an unreadable entry is skipped rather than fatal.
- **Web API client and hook**
  - **Files:** `apps/web/lib/api/domains/workspace-file-api.test.ts`,
    `apps/web/hooks/use-file-upload.test.ts`
  - **How:** form fields and headers are correct, a non-OK response raises `ApiError`, no-conflict
    selections skip the dialog, cancelling uploads nothing, Skip omits its file, and one failing file
    does not fail the others.
- **Conflict dialog**
  - **File:** `apps/web/components/task/file-upload-conflict-dialog.test.tsx`
  - **How:** every conflict is listed, per-file and apply-to-all resolutions are collected, and
    cancel reports cancellation.
- **Upload entry points**
  - **Files:** `apps/web/components/task/file-browser-toolbar.test.tsx`,
    `apps/web/components/task/file-context-menu.test.tsx`
  - **How:** the create menu renders all three items and New File still triggers inline creation;
    Upload appears for a folder and is absent for a file and for a multi-selection.
- **Download surfaces**
  - **Files:** `apps/web/components/editors/monaco/monaco-editor-toolbar.test.tsx`, a new CodeMirror
    toolbar test, and a new `apps/web/components/task/file-editor-panel.download.test.tsx`
  - **How:** each control invokes its handler; both viewers render the action from `headerActions`;
    binary content round-trips without corruption.

---

## E2E Tests

- **Scenario:** Upload files through the create menu and see them appear in the tree.
  - **File:** `apps/web/e2e/tests/task/workspace-file-transfer.spec.ts`
  - **What to verify:** `setInputFiles` against the hidden input, the nodes appear at the destination
    without a refresh, and the confirmation names the written paths.
- **Scenario:** Upload onto existing names and resolve the conflicts.
  - **File:** same
  - **What to verify:** the dialog lists exactly the conflicts, Replace overwrites, Keep both
    produces the `-1` name beside the untouched original, Skip writes nothing, and cancel writes
    nothing at all.
- **Scenario:** Upload a folder and see its structure recreated.
  - **File:** same
  - **What to verify:** nested directories exist at the right relative paths.
- **Scenario:** Open an unpreviewable file and download it from that screen.
  - **File:** same
  - **What to verify:** the control is present on the binary viewer and the bytes match the source.

`file-tree-multi-select.spec.ts` is the nearest existing neighbour for the tree interactions.

---

## Public documentation

The Files panel has no dedicated public page today, and this adds no CLI command, configuration key,
or deployment concern. No public documentation change is planned. If a Files-panel page is added
later, upload and download belong on it.

---

## Implementation Waves And Parallel Candidates

Execution remains sequential in the primary conversation unless the user authorizes otherwise.

Wave 1:

- [x] [task-01-agentctl-file-write](task-01-agentctl-file-write.md)
- [x] [task-02-editor-download-actions](task-02-editor-download-actions.md)

Wave 2:

- [x] [task-03-backend-upload-route](task-03-backend-upload-route.md)

Wave 3:

- [x] [task-04-web-upload-client](task-04-web-upload-client.md)

Wave 4:

- [x] [task-05-conflict-resolution-dialog](task-05-conflict-resolution-dialog.md)

Wave 5:

- [x] [task-06-upload-entry-points](task-06-upload-entry-points.md)

Wave 6:

- [ ] [task-07-file-transfer-e2e](task-07-file-transfer-e2e.md)

Tasks 01 and 02 are genuinely parallel-safe: they share no file and no contract, because download
needs no transport. Everything from 03 onward is a chain up the upload stack.

---

## Risks

- **Containment is the whole security story, and folder upload widens it.** The client now supplies
  interior path segments, not just a file name. Every segment must resolve through
  `resolveMutationPath`, including during intermediate directory creation. The table tests exist to
  make that non-negotiable.
- **The file tree's existing drag-to-move is untouched.** No external-drop branch is added, so the
  known side-panel issues are not compounded. If drag and drop is revived later, note that an
  external drag currently dies in `handleDragOver` because `dragPathsRef` is empty and
  `allSameParent` is vacuously true, so the new branch would have to run ahead of that early return.
- **Two-phase upload is not a lock.** Preflight informs a decision; it does not reserve the path. The
  `409` on an unresolved existing destination is the actual safety property. Treating preflight as
  authoritative would reintroduce silent overwrite under a race.
- **Cancel must mean nothing was written.** Because the dialog appears after preflight and before any
  upload, this is achievable, but only if the flow never starts uploading non-conflicting files
  early as an optimization.
- **Regressing New File.** Inline creation is currently one click on the `+`. It becomes two through
  the menu. Keep `onStartCreate` semantics byte-identical and cover the existing flow in the toolbar
  test.
- **Two editor toolbars.** Monaco and CodeMirror both need the download control in the same change.
- **Writing under a running agent.** The mutation barrier plus temp-then-rename covers atomicity, but
  an upload during an active turn can still surprise the agent. Surface the destination path in the
  confirmation.
- **Translation gate.** Five locales, and the key check fails the build on a missing or untranslated
  value. Budget for it in the task that adds the copy.
- **`pnpm run typecheck` OOMs at the default heap on this host.** It needs roughly 4 GB; run it as
  `node --max-old-space-size=8192 node_modules/typescript/bin/tsc --noEmit`, because the repo's mise
  `[env]` block overwrites an inherited `NODE_OPTIONS`.
- **No rollout toggle.** Reasoning is in the system design: the workspace is already fully
  agent-writable, so upload adds an authenticated route to an existing capability. If that judgment
  is not wanted, add the flag before task 03 lands, since the backend route is the natural gate.

---

## Manual verification (2026-09-02)

Driven with Playwright against a live dev instance at `v0.92.2-68-gc51ec0a21`, exercising the real
UI rather than mocks.

| Scenario | Result |
| --- | --- |
| Create menu shows New file / Upload files / Upload folder | pass |
| Upload two files, appear in tree without refresh | pass |
| Repeat upload raises the conflict dialog before any write | pass |
| Keep both writes `name-1.ext` beside an untouched original | pass |
| Folder upload recreates `cpu-z/docs/` and both executables | pass |
| Download from the image viewer | pass |
| Download from the unpreviewable-file screen | pass |
| Upload item on folder right-click | pass |
| Editor header download, text file | pass |
| Tree right-click download unchanged | pass |

A 5,478,310-byte zip uploaded and downloaded byte-for-byte identical, exercising the streamed
multipart write and the base64 binary download path on a real 5.4 MB file.

PR media (10 stills, an MP4, and a GIF) was captured from this run.
