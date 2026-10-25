---
status: active
system: ui
created: 2026-08-04
owners:
  - kandev
---
# File Tree Chat Context Requirements

## Overview

Users inspecting a task's Files tab must currently return to the chat composer and search for a path again before they can discuss it with the agent. This interrupts file-tree exploration and is especially awkward for directories, which are easiest to identify in the tree hierarchy.

## Requirements

### REQ-UI-FILE-TREE-CHAT-CONTEXT-001: File Tree Chat Context

**Intent:** Users inspecting a task's Files tab must currently return to the chat composer and search for a path again before they can discuss it with the agent. This interrupts file-tree exploration and is especially awkward for directories, which are easiest to identify in the tree hierarchy.

#### Acceptance criteria

- **AC-UI-FILE-TREE-CHAT-CONTEXT-001.1:** Right-clicking a single file or directory in a task session's Files tab exposes an **Add to chat context** action alongside the existing node actions.
- **AC-UI-FILE-TREE-CHAT-CONTEXT-001.2:** Choosing the action adds that task-root-relative path to the chat composer for the same session without opening the file, changing the active panel, or reading file contents eagerly.
- **AC-UI-FILE-TREE-CHAT-CONTEXT-001.3:** A context path appears at most once per session. Choosing the action again for the same path leaves one context item.
- **AC-UI-FILE-TREE-CHAT-CONTEXT-001.4:** File and directory context items are visually distinguishable before send. Directory items do not attempt to open a file preview when selected.
- **AC-UI-FILE-TREE-CHAT-CONTEXT-001.5:** File-tree context items are ephemeral: they survive a reload and a failed send. An accepted shared task-chat submission shall consume its submitted, unchanged, unpinned items while preserving pinned items. Successful send includes accepted queue admission; it does not require the agent to finish responding.
- **AC-UI-FILE-TREE-CHAT-CONTEXT-001.6:** Sending the message, directly or after a busy-session send is queued, includes each selected path in the existing hidden context block and `context_files` message metadata so the agent is instructed to inspect the path and the sent user message records it. Metadata retains optional directory identity while remaining compatible with older `{ path, name }` entries.
- **AC-UI-FILE-TREE-CHAT-CONTEXT-001.7:** On phone, tablet, and coarse-pointer desktop-workbench compositions, each eligible file-tree row exposes a visible, accessible action trigger. The trigger opens the existing responsive menu treatment with a touch target at least 44px high; long press or right-click is not required.
- **AC-UI-FILE-TREE-CHAT-CONTEXT-001.8:** All new visible labels and feedback use the existing localization catalogs.
- **AC-UI-FILE-TREE-CHAT-CONTEXT-001.9:** File-tree rows shall retain compact single-line spacing in every responsive composition. A visible coarse-pointer action, including in the desktop workbench, shall not cause wrapping; long names shall truncate instead.
- **AC-UI-FILE-TREE-CHAT-CONTEXT-001.10:** After the Files panel reopens or changes size, every row within its scroll viewport shall remain visible and ordered without blank gaps. Recovery shall not require a manual scroll, file operation, or page reload. This applies to desktop and touch layouts.
- **AC-UI-FILE-TREE-CHAT-CONTEXT-001.11:** When a user adds a file or directory for their next message while an older shared task-chat submission awaits acceptance, accepting the older submission shall preserve the new selection in the composer and through reload in the same browser tab. The older message shall retain only its own submitted context. Losing that selection would silently omit context from the user's next message.
- **AC-UI-FILE-TREE-CHAT-CONTEXT-001.12:** When a user removes a submitted context item and adds another selection at the same path before the older submission is accepted, the replacement shall survive acceptance, including when its displayed values are identical. Repeating Add without removing or changing the existing item remains deduplicated and does not create a replacement.
- **AC-UI-FILE-TREE-CHAT-CONTEXT-001.13:** When admission is rejected, reports unsuccessful, or throws, shared task-chat submission shall preserve all current context selections, including submitted items, later additions, replacements, and pinned items. It shall not restore items the user intentionally removed.
- **AC-UI-FILE-TREE-CHAT-CONTEXT-001.14:** When a user pins an item during admission, it shall survive the older acceptance. An item pinned at submission and unpinned during admission shall also survive for the next message. Acceptance shall preserve the latest selection metadata and leave other sessions' selections untouched.

## Migrated source detail

## Why

Users inspecting a task's Files tab must currently return to the chat composer and search for a path again before they can discuss it with the agent. This interrupts file-tree exploration and is especially awkward for directories, which are easiest to identify in the tree hierarchy.

## What

- Right-clicking a single file or directory in a task session's Files tab exposes an **Add to chat context** action alongside the existing node actions.
- Choosing the action adds that task-root-relative path to the chat composer for the same session without opening the file, changing the active panel, or reading file contents eagerly.
- A context path appears at most once per session. Choosing the action again for the same path leaves one context item.
- File and directory context items are visually distinguishable before send. Directory items do not attempt to open a file preview when selected.
- File-tree context items follow the submitted-selection consumption boundary in AC-UI-FILE-TREE-CHAT-CONTEXT-001.5 and .11-.14. Adding context while another send awaits acceptance must not lose the next message's selection.
- Sending the message, directly or after a busy-session send is queued, includes each selected path in the existing hidden context block and `context_files` message metadata so the agent is instructed to inspect the path and the sent user message records it. Metadata retains optional directory identity while remaining compatible with older `{ path, name }` entries.
- On phone, tablet, and coarse-pointer desktop-workbench compositions, each eligible file-tree row exposes a visible, accessible action trigger. The trigger opens the existing responsive menu treatment with a touch target at least 44px high; long press or right-click is not required.
- File-tree rows retain compact single-line spacing in every responsive composition; visible coarse-pointer actions do not cause wrapping, and long names truncate instead.
- All new visible labels and feedback use the existing localization catalogs.
- Existing file opening, folder expansion, multi-selection, drag/drop, editor, rename, download, and delete behavior remains unchanged.

## Failure modes

- Adding a context path is local and does not depend on a filesystem or network read. If the later message send fails, the item remains in the composer for retry.
- If a path is renamed or deleted after being attached, Kandev still sends the recorded task-root-relative path. The agent handles the missing or stale path in the same way as an existing `@file` context reference.

## Persistence guarantees

- Pending context paths are stored in the existing session-scoped browser `sessionStorage` entry and survive a reload in that browser tab.
- Pending context paths are not backend-persisted drafts and do not transfer to another browser or session.
- Existing stored entries without directory metadata remain readable and render as file context items.

## Scenarios

- **GIVEN** a file in the Files tab, **WHEN** the user right-clicks it and chooses Add to chat context, **THEN** one chip for that file appears in the same session's chat composer.
- **GIVEN** a directory in the Files tab, **WHEN** the user chooses Add to chat context, **THEN** a folder chip appears without requesting a file preview or expanding every descendant.
- **GIVEN** a path already present in chat context, **WHEN** the user adds the same node again, **THEN** the composer still contains exactly one item for that path.
- **GIVEN** a submitted file or directory context item, **WHEN** shared task-chat admission succeeds, **THEN** the sent user message records the path and only its unchanged unpinned selection is consumed.
- **GIVEN** a pending file or directory context item, **WHEN** a message send fails, **THEN** the item remains available for retry.
- **GIVEN** a pending file or directory context item while the session is busy, **WHEN** the message is queued and later drained, **THEN** the hidden context instruction and sent user-message metadata both retain the selected path.
- **GIVEN** a pending directory context item, **WHEN** the message is sent directly or after queue drain, **THEN** its sent metadata retains directory identity and the history badge uses a folder icon.
- **GIVEN** a Files-tab search result, **WHEN** the user opens its right-click or touch overflow action and chooses Add to chat context, **THEN** the same session-bound context handler adds the result without opening the file.
- **GIVEN** a phone or coarse-pointer task layout, **WHEN** the user opens the Files tab, **THEN** a visible touch action can add a file or directory to chat context without right-click or long press.
- **GIVEN** more than one file-tree node is selected, **WHEN** the shared bulk context menu opens, **THEN** its existing bulk actions remain unchanged and no ambiguous single-node Add to chat context action is shown.

## Out of scope

- Recursively expanding a directory into one context item per descendant.
- Adding all nodes in a multi-selection as one bulk context operation.
- Uploading file contents or introducing a new backend file-read API.
- Automatically switching to or focusing the Chat panel after the action.
- Persisting unsent composer context as a backend draft; queued messages are already user-message history and retain their attached metadata.
- Generic navigation/remount lifetimes, same-text composer replacement, comment/feedback concurrency, upload lifecycle changes, transport retry policy, and backend agent delivery policy.
- The independent passthrough composer send lifecycle. This repair's concurrency criteria cover the shared task-chat submit boundary and its callback admission path; no claim of passthrough race protection is made.

## Implementation plan

- [File tree chat context](../../../plans/file-tree-chat-context/plan.md)
- [Preserve unsent chat context](../../../plans/preserve-unsent-chat-context/plan.md)
