---
status: current
system: ui
requirements:
  - REQ-UI-FILE-TREE-CHAT-CONTEXT-001
---

# File Tree Chat Context System Design

## Purpose and boundaries

The UI system owns the reusable file-tree row presentation and responsive action
surface used by task Files panels. This repair preserves the existing
file-tree interaction contract while ensuring the coarse-pointer action trigger
does not turn compact rows into multi-line rows. File data, context-file state,
and message delivery remain owned by their existing task and chat contracts.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `AC-UI-FILE-TREE-CHAT-CONTEXT-001.7` | [Responsive row geometry](#responsive-row-geometry), [Interaction preservation](#interaction-preservation) |
| `AC-UI-FILE-TREE-CHAT-CONTEXT-001.9` | [Responsive row geometry](#responsive-row-geometry) |

The existing task and chat contracts own the other acceptance criteria for file
context state and message delivery. This design only records the responsive row
and action-surface repair.

## Components and responsibilities

`FileBrowser` in `apps/web/components/task/file-browser.tsx` determines whether
touch actions are required from the responsive breakpoint and pointer model.
`FileBrowserContentArea` and `TreeNodeItem` in
`apps/web/components/task/file-browser-parts.tsx` pass that presentation state
through the file-tree render path. `FileTreeNodeTouchActions` owns the visible
coarse-pointer overflow trigger and its responsive menu; its trigger remains a
secondary action and does not own row navigation.

## Responsive row geometry

A file-tree row remains a compact single flex line when its name and controls
fit within the panel. Responsive rows that render a 44px touch trigger establish
an exclusive 44px vertical interaction slot. The absolutely positioned trigger
stays within that slot while the filename reserves its action space, shrinks,
and truncates. This prevents adjacent action targets from overlapping without
allowing row wrapping.

The responsive action remains rendered on phone and coarse-pointer layouts,
including when the desktop task composition is selected on a phone. Fine-pointer
desktop rows retain the existing compact geometry and context-menu interaction.

## Interaction preservation

The row's primary click continues to open files or expand directories. The touch
action trigger stops row click, selection, keyboard, and pointer propagation
before opening the existing dropdown menu. The menu continues to provide the
same context action and touch-sized menu items. Search-result rows use the same
trigger geometry and action component.

## Verification

The component regression test shall verify that a touch-enabled row keeps the
non-wrapping row geometry and that a fine-pointer row does not receive the
responsive action layout. The existing desktop and mobile file-tree chat
context Playwright flows remain the integration proof that the trigger is
reachable and the action still adds the selected file or directory to chat.
