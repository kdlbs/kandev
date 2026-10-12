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

## Out of scope

- Cancellation or rollback of a filesystem write/delete already accepted by
  the server, and background persistence of inactive editor buffers.
- New mutation ordering, deduplication, retry or conflict-resolution policy
  within an unchanged editor lifetime.
- Workspace read races outside background refresh of already-open Dockview
  buffers; ordering reads against saves/deletes, transport/authorization
  redesign, backend APIs, new settings, and responsive presentation changes.

## System design

- [File editor mutation ownership](../system-design/file-editor-mutation-ownership.md)

## Implementation plans

- [File editor mutation ownership](../../../plans/file-editor-mutation-ownership/plan.md)
- [Preserve editor workspace refresh](../../../plans/editor-workspace-refresh-ownership/plan.md)
