---
status: active
system: ui
created: 2026-10-03
owners:
  - kandev
---

# File Editor Mutation Ownership Requirements

## Purpose and ownership

An outstanding file action must not change the editor that replaces its source.
UI owns this reusable editor presentation and reply-publication contract.
Workspaces retain filesystem mutation and authorization authority; tasks retain
session eligibility and lifecycle. This supplements the stale-view boundary in
[task navigation responsiveness](task-navigation-responsiveness.md), whose
existing read coordination does not define editor mutation completion.

## Terminology

- **Editor incarnation:** One open buffer lifetime. Editing the buffer preserves
  that lifetime; closing, replacing, or restoring it creates another lifetime.
- **Session visit:** A continuous selection of one session by a mounted action
  consumer. Returning after selecting another session starts another visit.
- **Owned completion:** A completion whose originating consumer, session visit,
  and affected editor incarnation remain available.

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
  boundary. Without later edits it shall clear the buffer and panel dirty state.
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
  the replacement editor. Desktop and phone shall retain their existing
  composition, navigation, scrolling and touch behavior.

## Out of scope

- Cancellation or rollback of a filesystem write/delete already accepted by
  the server, and background persistence of inactive editor buffers.
- New mutation ordering, deduplication, retry or conflict-resolution policy
  within an unchanged editor lifetime.
- General workspace resync/read races, transport/authorization redesign,
  backend APIs, new settings, and responsive presentation changes.

## System design

- [File editor mutation ownership](../system-design/file-editor-mutation-ownership.md)

## Implementation plans

- [File editor mutation ownership](../../../plans/file-editor-mutation-ownership/plan.md)
