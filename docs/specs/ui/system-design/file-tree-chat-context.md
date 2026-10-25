---
status: current
system: ui
requirements:
  - REQ-UI-FILE-TREE-CHAT-CONTEXT-001
---

# File Tree Chat Context System Design

## Purpose and boundaries

The existing UI capability owns Files-to-composer selection and its browser-tab
retention contract as well as the reusable file-tree row/action surface. This
design extends that existing owner rather than creating a second incident or
task-lifecycle specification. Tasks retains durable messages, queue admission,
and agent execution. The shared task-chat completion boundary consumes local
submitted selections; it does not redefine delivery. Existing responsive row
geometry remains part of this design.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `AC-UI-FILE-TREE-CHAT-CONTEXT-001.7` | [Responsive row geometry](#responsive-row-geometry), [Interaction preservation](#interaction-preservation) |
| `AC-UI-FILE-TREE-CHAT-CONTEXT-001.9` | [Responsive row geometry](#responsive-row-geometry) |
| `AC-UI-FILE-TREE-CHAT-CONTEXT-001.5`, `.11`-`.14` | [Submitted selection boundary](#submitted-selection-boundary), [Persistence and failure](#persistence-and-failure), [Verification](#verification) |
| `AC-UI-FILE-TREE-CHAT-CONTEXT-001.6` | [Payload compatibility](#payload-compatibility) |

The backend message and queue contracts retain admission and delivery authority.
Selection consumption is local to the composer and follows their existing result.

## Submitted selection boundary

`FileBrowser.handleAddToChatContext`, through `useFileBrowserHandlers`, writes
un-pinned `ContextFile` entries into `useContextFilesStore`. The touch
`file-tree-touch-add-to-chat` action and desktop `FileContextMenu` share this
callback and have no send-busy guard. `useContextFiles` in
`chat/use-chat-panel-state.ts` subscribes to the same canonical entries.
`TaskChatPanel` uses `useSubmitHandler` in `chat/chat-input-area.tsx`.

Before the first admission await, `submitChatPayload` captures a shallow array
of the unpinned canonical entries from its captured `panelState.contextFiles`,
plus the captured `resolvedSessionId`. This is the same selection supplied to
the default `useMessageHandler` closure. It must not read a newer store snapshot
after admission or combine it with a newer rendered panel state.

Add a separate store action, `consumeSubmittedEphemeral(sessionId, submitted)`.
Expose it through `useContextFiles` and call it from `completeChatSubmission`
with the captured submission snapshot. Within one Zustand update, remove a
current entry only when it is still unpinned and is the identical canonical
object included in the submitted snapshot. Preserve current order and metadata
for every survivor. Never reinsert entries absent from the current store.

Store entries have immutable selection identity for this in-process boundary.
`addFile` shall shallow-copy a newly inserted descriptor, so even remove/re-add
with the very same caller object produces a new canonical identity. No-op
duplicate additions retain identity. Existing pin upgrades and `unpinFile`
already replace the entry object; unrelated collection updates retain surviving
objects. Snapshot only entries unpinned at submission: a submitted pinned entry
that becomes unpinned must not be consumed by the older acceptance. No revision
field, counter, identifier, persistence migration, or framework change is needed.

The existing unconditional `clearEphemeral` action remains available with its
current semantics. Its separate passthrough consumer and explicit session/file
clear/removal actions are not migrated by this repair. After snapshot consumption,
the existing plan-mode `plan:context` re-add remains unchanged and deduplicated.

## Payload compatibility

`ContextFile` retains `path`, `name`, optional `isDirectory`, and optional
`pinned`. File/directory entries, `prompt:` descriptors, and `plan:context` share
the local identity mechanism; their existing inclusion and filtering rules remain
unchanged. Inline mentions come from `ChatSubmitPayload`, not the context store,
and are not consumed as stored selections. `ContextItem` variants for comments,
feedback, plans, images, and uploads have independent cleanup owners.

No fields are added to `ChatSubmitPayload`, `ChatSubmitResult`, `context_files`,
or any transport API. Default `useMessageHandler` still composes captured context
plus inline mentions before delivery, filters prompt/plan sentinels from file
metadata, and awaits direct `message.add` or real queue admission. The callback
`onSend` branch retains its existing payload and `false`/throw/void/true result
meaning; it does not gain implicit context composition. Clarification routing,
outgoing transforms, admission recovery, successful text/upload cleanup, and
other completion side effects keep their existing behavior.

## Persistence and failure

The new action persists the survivor list through existing `persistFiles` in the
same store update. The storage format and `kandev.contextFiles.<sessionId>` key
remain unchanged. Object identity stays in memory and never enters serialization.
Hydration retains legacy `{ path, name }` entries; no pending admission survives
a page reload under this design. Current session deletion and browser-storage
failure behavior remain unchanged.

Consumption runs only after existing successful admission. A `false` result or
exception does not consume anything or roll back deliberate user removals.
Already pinned entries, entries pinned in flight, later unpinned selections,
new paths, and same-path replacements survive. Repeating consumption is harmless.
No new logs, settings, metrics, feature flags, or security boundary are required.

## Desktop and mobile scope

This correction changes state consumption inside the existing shared submit
boundary. It changes no layout, touch target, scroll owner, navigation, viewport
branch, or rendered control. The mobile-parity data-only exception applies:
targeted real-provider component tests exercise the existing touch selection
action; no new preview or mobile Playwright run is needed. Desktop and phone
receive identical survivor semantics from the shared store.

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

Permanent tests are authored independently from the protected ROOT reproduction.
Store tests prove snapshot consumption and persisted survivors, including legacy
hydration, directories, pinned/unpinned transitions, duplicate no-ops, same-path
remove/re-add with the same descriptor object, other-session isolation, and
unchanged intentional clearing. A real-provider regression mounts
`StateProvider`, `ToastProvider`, `FileBrowser`, and a shared submit harness using
real `useContextFiles`, `useSubmitHandler`, default `useMessageHandler`, and
storage. Mock only external HTTP/WebSocket transport. Defer admission, select a
new path through the actual row menu, then settle admission and assert sent
context, live survivors, persisted survivors, and hydration independently.
Direct accepted/rejected and queued accepted controls exercise real callers;
callback admission gets its own compatibility controls.

These tests establish default client composition/admission coverage, not real
browser, backend, or agent-delivery evidence. Existing text/attachment admission
tests protect successful composer cleanup. Exact cases and resource-capped serial
commands are in the [repair work order](../../../plans/preserve-unsent-chat-context/task-01-consume-submitted-context.md).

The component regression test shall verify that a touch-enabled row keeps the
non-wrapping row geometry and that a fine-pointer row does not receive the
responsive action layout. The existing desktop and mobile file-tree chat
context Playwright flows remain the integration proof that the trigger is
reachable and the action still adds the selected file or directory to chat.
