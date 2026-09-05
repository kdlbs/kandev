---
status: draft
system: ui
created: 2026-09-01
owners:
  - kandev
---

# Workspace File Transfer Requirements

## Overview

The Files panel lets a person read and edit the task workspace, but it cannot move bytes across the
browser boundary in either direction with any reliability. There is no upload at all: the workspace
file contract exposes create, update-by-diff, rename, and delete, and none of them accept raw file
content. Download exists but is reachable from exactly one place, a single-file right-click, so the
surfaces where downloading is the only sensible action offer no way to do it.

That gap forces people out of the workbench. Getting a fixture, a screenshot, a certificate, or a
vendor export into the workspace means a terminal, an agent instruction, or a commit. Getting a
generated archive or PDF back out means the same trip in reverse, from a screen that has already
told the user it cannot preview the file.

Chat attachments partly cover the inbound case, but they are a different tool: they are addressed to
a turn rather than to the workspace, so they interrupt the conversation and are awkward when the
file is remembered late. They also do not help someone running Kandev on a headless machine who
needs to push a file from their laptop, which is the case that motivated this most strongly in
review.

This system owns the contract because the durable outcome is a Files-panel capability: which
surfaces offer transfer, what the person sees while it runs, and how a collision or failure is
presented. The transport it uses (session-scoped agentctl HTTP, the workspace path-containment
rules, and the attachment streaming pattern) belongs to adjacent systems and is consumed here, not
redefined.

## Terminology

- **Transfer:** Moving file bytes between the browser and the task workspace in either direction.
- **Destination folder:** The workspace-relative directory an upload targets. The workspace root is
  the empty path.
- **Unpreviewable file:** A workspace file the editor cannot render as text or as an image, shown
  today on the binary viewer screen.
- **Conflict:** A selected file whose destination path already exists in the workspace.
- **Preflight:** The check that reports conflicts for a whole selection before any byte is uploaded.
- **Resolution:** The choice a person makes for a conflicting file: replace, keep both, or skip.

## Requirements

### REQ-UI-WORKSPACE-FILE-TRANSFER-001: Manual upload into the task workspace

**Intent:** A person working in a task can put a file from their machine into the task workspace
without leaving the workbench, using the same panel they already use to browse it.

**User story:** As a person working in a task, I want to upload a file into a chosen workspace
folder, so that I can supply fixtures, exports, and assets without a terminal or an agent detour.

#### Acceptance criteria

- **AC-UI-WORKSPACE-FILE-TRANSFER-001.1:** The Files panel's create control offers **New File**,
  **Upload Files**, and **Upload Folder**. Choosing New File begins the existing inline
  file-creation flow with unchanged behavior; the two upload choices open the operating system's
  file picker and folder picker respectively.
- **AC-UI-WORKSPACE-FILE-TRANSFER-001.2:** Right-clicking a folder in the file tree offers **Upload
  files here**, targeting that folder. The action is absent for files and absent when more than one
  node is selected.
- **AC-UI-WORKSPACE-FILE-TRANSFER-001.3:** The picker accepts more than one file at a time, and each
  selected file is uploaded to the destination folder. When no folder is the active target, the
  destination is the workspace root.
- **AC-UI-WORKSPACE-FILE-TRANSFER-001.4:** A person can upload a folder. Its directory structure is
  recreated under the destination, intermediate directories are created as needed, and each
  contained file lands at its corresponding relative path.
- **AC-UI-WORKSPACE-FILE-TRANSFER-001.5:** While an upload runs, the panel shows that it is in
  progress. On success, each uploaded file appears in the file tree at its destination without a
  manual refresh, and the confirmation names the path it was written to.
- **AC-UI-WORKSPACE-FILE-TRANSFER-001.6:** A failed upload leaves no partial or truncated file in
  the workspace, removes any optimistic tree entry, and reports why it failed.
- **AC-UI-WORKSPACE-FILE-TRANSFER-001.7:** Upload is offered only when the panel has an active task
  session. Without one, the actions are absent rather than present and failing.

### REQ-UI-WORKSPACE-FILE-TRANSFER-004: Conflicts are resolved before anything is written

**Intent:** A person decides what happens to each colliding file, and makes that decision knowing the
full set of conflicts, before any byte reaches the workspace.

**User story:** As a person uploading files, I want to be told which files already exist and choose
what to do, so that I never silently replace work or discover a surprise rename afterwards.

#### Acceptance criteria

- **AC-UI-WORKSPACE-FILE-TRANSFER-004.1:** Before any content is uploaded, the selection's
  destination paths are checked against the workspace and every conflict is reported as one result.
- **AC-UI-WORKSPACE-FILE-TRANSFER-004.2:** When there is no conflict, the upload proceeds with no
  additional prompt.
- **AC-UI-WORKSPACE-FILE-TRANSFER-004.3:** When there is at least one conflict, the person is shown
  which files conflict and chooses **Replace**, **Keep both**, or **Skip** for them. A choice can be
  applied to every remaining conflict at once.
- **AC-UI-WORKSPACE-FILE-TRANSFER-004.4:** **Keep both** writes the incoming file under the next
  available `name-<n>.ext` variant and leaves the existing file untouched. **Skip** writes nothing for that file. **Replace** overwrites it.
- **AC-UI-WORKSPACE-FILE-TRANSFER-004.5:** Cancelling the resolution uploads nothing at all, not
  even the files that had no conflict.
- **AC-UI-WORKSPACE-FILE-TRANSFER-004.6:** A resolution is honored per file. Uploading without a
  resolution for a conflicting path is refused rather than defaulting to overwrite, including when a
  conflicting path is created between the preflight and the upload.

### REQ-UI-WORKSPACE-FILE-TRANSFER-002: Download reachable from every file surface

**Intent:** Downloading a file is offered from the surfaces where a person is actually looking at
that file, not only from a right-click in the tree.

**User story:** As a person viewing a workspace file, I want to download it from the screen I am
already on, so that I do not have to hunt for the file again in the tree.

#### Acceptance criteria

- **AC-UI-WORKSPACE-FILE-TRANSFER-002.1:** The file editor header offers a download action for the
  open file, in both the Monaco and CodeMirror editors, with the same placement, icon, and
  accessible name in each.
- **AC-UI-WORKSPACE-FILE-TRANSFER-002.2:** The unpreviewable-file screen offers a download action.
  Download is the primary useful action on that screen, which otherwise offers none.
- **AC-UI-WORKSPACE-FILE-TRANSFER-002.3:** The image viewer offers the same download action, since
  it shares the viewer header contract with the unpreviewable-file screen.
- **AC-UI-WORKSPACE-FILE-TRANSFER-002.4:** Downloading from a viewer uses the file content the
  viewer already holds and does not re-request it.
- **AC-UI-WORKSPACE-FILE-TRANSFER-002.5:** A downloaded file keeps its original bytes and its
  original file name. Content that is not text is not corrupted by the transfer.
- **AC-UI-WORKSPACE-FILE-TRANSFER-002.6:** The existing single-file download in the tree's
  right-click menu keeps its current behavior and placement.

### REQ-UI-WORKSPACE-FILE-TRANSFER-003: Safe and bounded transfer

**Intent:** Upload is the first path that writes caller-supplied bytes into a task workspace, so its
containment, size, and concurrency behavior are part of the contract rather than an implementation
detail.

**User story:** As an operator, I want workspace uploads to be bounded and contained, so that the
feature cannot write outside the workspace, exhaust memory, or corrupt a file an agent is reading.

#### Acceptance criteria

- **AC-UI-WORKSPACE-FILE-TRANSFER-003.1:** An upload destination that escapes the task workspace, by
  traversal, by absolute path, or through a symlinked directory, is rejected and writes nothing.
  This applies to every path segment supplied by a folder upload, not only the destination folder.
- **AC-UI-WORKSPACE-FILE-TRANSFER-003.2:** A single uploaded file larger than the message-attachment
  limit is rejected with a size-specific message, and the rejection is reported per file rather than
  failing the whole selection.
- **AC-UI-WORKSPACE-FILE-TRANSFER-003.3:** Upload requires the same authenticated identity as other
  workspace mutations. An unauthenticated request is refused.
- **AC-UI-WORKSPACE-FILE-TRANSFER-003.4:** An upload is never observable by the agent as a partially
  written file. The destination path either does not exist or holds the complete uploaded content.
- **AC-UI-WORKSPACE-FILE-TRANSFER-003.5:** Uploaded bytes are streamed end to end. No layer holds a
  whole file in memory or encodes it as base64.
- **AC-UI-WORKSPACE-FILE-TRANSFER-003.6:** A completed upload emits the same workspace change
  notification as the other file mutations, so the file tree and Git status converge without a
  manual refresh.

## Out of scope

- Dragging files or folders from the operating system onto the file tree. The tree's existing
  internal drag-to-move behavior is unchanged, and no external-drop branch is added to it.
- Downloading a folder, or downloading more than one selected file, as an archive.
- Replacing the 10 MB workspace read cap, or the streaming download endpoint that would be needed to
  transfer a file above it.
- The separate defect where a file above the read cap leaves the editor on its loading state
  indefinitely.
- Uploading into anything other than the active task session's workspace.
- Version history, restore, or any retention policy for replaced files.

## Related work

- Chat attachments already stream multipart from the browser through the backend into agentctl. That
  contract is owned by the task-attachment path and is the exemplar this capability follows; it is
  not modified here.
