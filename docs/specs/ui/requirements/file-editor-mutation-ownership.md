---
status: active
system: ui
created: 2026-10-03
owners:
  - kandev
---

# File Editor Mutation Ownership Requirements

## Purpose and ownership

An outstanding file action or workspace refresh must not change the editor that
replaces its source or discard newer acknowledged content and unsaved typing.
UI owns this reusable editor presentation and reply-publication contract.
Workspaces retain filesystem mutation and authorization authority; tasks retain
session eligibility and lifecycle. This supplements the stale-view boundary in
[task navigation responsiveness](task-navigation-responsiveness.md), whose
existing read coordination does not define editor mutation completion.

The mutation contract covers both Dockview editors and the tablet TaskCenterPanel
file tabs. Each owns its local buffer lifetime and action consumer. The refresh
criteria cover background reconciliation of already-open Dockview buffers;
independent tablet restoration and phone file-viewer reads are outside that scope.

## Terminology

- **Editor incarnation:** One open buffer lifetime. Editing the buffer preserves
  that lifetime; closing, replacing, or restoring it creates another lifetime.
- **Session visit:** A continuous selection of one session by a mounted action
  consumer. Returning after selecting another session starts another visit.
- **Owned completion:** A completion whose originating consumer, session visit,
  affected editor incarnation, and panel host remain available.
- **Eligible refresh:** A workspace content read admitted by a live consumer for
  its current session visit and existing repository/file editor incarnation.

## Requirements

### REQ-UI-FILE-EDITOR-MUTATION-001: Owned editor completion

**Intent:** Preserve the selected editor's content and status while earlier
filesystem actions finish.

#### Acceptance criteria

- **AC-UI-FILE-EDITOR-MUTATION-001.1:** After a session change, a return to the
  originating session, or disposal of the originating action consumer, an old
  save or delete completion shall not change the current buffer, saved baseline,
  hash, remote-update indication, panel title/dirty state, panel lifetime,
  saving indication, or current error/success feedback. An old save shall not
  synchronize a replacement editor's contents to a language server.
- **AC-UI-FILE-EDITOR-MUTATION-001.2:** Closing and reopening the same
  repository/path, replacing its buffer, or replacing its panel host shall
  prevent an old completion from changing the replacement editor. If a delete
  began without an editor panel, its completion shall not close a panel opened
  afterward.
- **AC-UI-FILE-EDITOR-MUTATION-001.3:** An owned successful save shall advance
  the baseline and hash to the snapshot actually saved. If typing continued,
  it shall preserve that newer buffer and its dirty indication, and synchronize
  language-server live content to that newer buffer with the actual disk-save
  boundary. Without later edits it shall retain the saved content and clear the
  buffer and panel dirty indicators.
- **AC-UI-FILE-EDITOR-MUTATION-001.4:** An owned successful delete shall close
  its pinned or preview editor only after success. An owned rejected response
  or transport failure shall preserve the editor and report the existing error;
  a failed save shall not notify the language server of a successful save.
- **AC-UI-FILE-EDITOR-MUTATION-001.5:** Identical paths in different
  repositories shall remain independent for request routing, buffer updates,
  panel closure and saving indication. Settling a retired action shall not clear
  a replacement action's saving indication.
- **AC-UI-FILE-EDITOR-MUTATION-001.6:** Applying a current remote update shall
  retain the existing reload outcome. If local preparation finishes after its
  consumer, session visit or editor incarnation retires, it shall not overwrite
  the replacement editor. Desktop, tablet and phone shall retain their existing
  composition, navigation, scrolling and touch behavior.
- **AC-UI-FILE-EDITOR-MUTATION-001.7:** When eligible refreshes overlap for the
  same open editor, only the most recently admitted refresh shall publish its
  reply, regardless of completion order or initiating consumer. Once a newer
  refresh is admitted, an older reply shall remain superseded even if the newer
  read fails. Different files and repositories shall refresh independently.
- **AC-UI-FILE-EDITOR-MUTATION-001.8:** A refresh whose consumer, session visit,
  editor incarnation or panel host retires during fetching or content preparation
  shall not change the live buffer, baseline, hash, remote-update indication or
  panel dirty/title state. A same-key reopen or identical replacement shall not
  revive the old reply. A later eligible refresh shall remain able to publish.
- **AC-UI-FILE-EDITOR-MUTATION-001.9:** A current refresh shall reconcile against
  the live buffer after content preparation, preserving typing made while that
  preparation was pending. Matching dirty content shall advance its baseline
  and clear buffer/panel dirtiness; different dirty content shall retain local
  typing and offer the current remote content for explicit reload. A clean
  buffer shall accept the current content, including an empty file, with its
  actual content hash and existing binary/resolved-path metadata. A failed read
  shall leave the editor usable and unchanged. Desktop, tablet and phone shall
  retain their existing composition and interaction paths.

- **AC-UI-FILE-EDITOR-MUTATION-001.10:** An owned Dockview save receipt shall
  preserve a distinct workspace snapshot observed after save admission, including
  its Reload affordance and hash when local text was retained. It shall record
  the successful saved baseline without discarding later typing. A matching save
  echo shall not produce a conflict; a warning already present at save admission
  may be resolved by that successful save. If a subsequent refresh already
  accepted newer content as its baseline, the older receipt shall not rewind it.
  Receipt-before-refresh shall retain the existing clean/dirty reconciliation.
  Reload preparation shall not replace a newer observed snapshot with an older one.

### REQ-UI-FILE-EDITOR-MUTATION-002: Open editor continuity after Files rename

**Intent:** Keep an open editor attached to its file when a user renames or
moves that file, or an enclosing folder, through Files.

#### Acceptance criteria

- **AC-UI-FILE-EDITOR-MUTATION-002.1:** After a successful Files rename of an
  open file, the existing editor shall identify the new path and retain its
  current content, saved baseline, dirty state, active tab and pinned or preview
  status. Saving a dirty buffer shall write the renamed file. Reopening the new
  path shall select the existing editor rather than create a duplicate.
- **AC-UI-FILE-EDITOR-MUTATION-002.2:** A successful folder rename shall apply
  the same continuity to open descendant files, preserving their relative paths.
  Unrelated paths, including names sharing only a textual prefix, and files in
  other repositories or task environments shall remain unchanged.
- **AC-UI-FILE-EDITOR-MUTATION-002.3:** A rejected or failed rename shall leave
  editor paths and buffers unchanged. A completion from an earlier task
  environment or retired editor shall not retarget a different editor.
- **AC-UI-FILE-EDITOR-MUTATION-002.4:** The shared Files move operation shall
  preserve the same continuity for each successful move; failed entries in a
  partial move shall not retarget. Existing desktop and tablet interaction and
  phone viewer composition shall remain unchanged.
- **AC-UI-FILE-EDITOR-MUTATION-002.5:** If a different open editor already
  represents a rename destination, Files shall reject that entry before the
  filesystem mutation, retain both editors and show existing rename-failure
  feedback. While an admitted rename is pending, opening its source or
  destination path (including folder descendants) shall be refused with the
  existing open-failure feedback; other paths remain available. A partial move shall continue
  independent entries and report its existing failure feedback. If a distinct
  destination editor nevertheless appears before an accepted response is
  published, both buffers shall be retained and the existing open-failure
  feedback shall be shown; the filesystem rename has already succeeded.

## Out of scope

- Cancellation or rollback of a filesystem write/delete already accepted by
  the server, and background persistence of inactive editor buffers.
- New mutation ordering, deduplication, retry or conflict-resolution policy
  within an unchanged editor lifetime.
- Inferring external filesystem renames, adding phone editing or a new phone
  rename entry point, and general save/rename transaction ordering.
- Preserving editor undo history across a Files rename. Current text and its
  saved baseline are retained, but the editor may start a new undo history.
- Workspace read races outside background refresh of already-open Dockview
  buffers; general filesystem transaction ordering, transport/authorization
  redesign, backend APIs, new settings, and responsive presentation changes.

## System design

- [File editor mutation ownership](../system-design/file-editor-mutation-ownership.md)

## Implementation plans

- [File editor mutation ownership](../../../plans/file-editor-mutation-ownership/plan.md)
- [Preserve editor workspace refresh](../../../plans/editor-workspace-refresh-ownership/plan.md)
- [Save and workspace observation reconciliation](../../../plans/editor-save-workspace-reconciliation/plan.md)
- [Open editor rename continuity](../../../plans/open-editor-rename-continuity/plan.md)
