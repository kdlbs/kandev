---
status: current
system: ui
created: 2026-10-21
requirements:
  - REQ-UI-CHAT-TASK-MENTION-001
---

# Chat Task Mention System Design

## Purpose and boundaries

UI owns rendering persisted task references within user-message Markdown.
Reuse the current message representation and canonical task navigation. No
backend, database, HTTP, or WebSocket contract change is required.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-CHAT-TASK-MENTION-001` | Reference extraction; Markdown composition; Navigation and responsive behavior; Compatibility and failure behavior |

## Reference extraction

`buildTaskMentionsContext` in `apps/web/hooks/use-message-handler.ts` appends a
`REFERENCED TASKS` block inside `kandev-system`, with title, task ID, workflow ID,
step, and optional state. Both standard and passthrough composers use it.
The backend's user-message projection removes hidden system content from
`content` and exposes the original in `raw_content` (covered by
`TestMessageToAPI_StripsOnlySystemContent`).

Add a pure helper under `apps/web/lib/tasks/` to extract title/ID pairs from
those blocks in `raw_content`, falling back to `content` when raw content is
absent. Accept only complete records inside a correctly delimited system block
with the exact referenced-tasks header. Parse the record suffix from the end
so punctuation in a title does not become a guessed ID. Validate IDs as opaque
nonempty route identifiers, excluding separators/control characters and bounding
their length. Do not extract arbitrary Markdown or executable URLs.

Bound source scanning and scanned reference records using explicit constants (initial
limits: 256 KiB of source, 200 references, 512 code units per title, 256 per ID).
When source exceeds the limit, skip task decoration rather than truncate into
a partial record. Ignore malformed records. Deduplicate identical pairs; discard
every mapping for a title associated with conflicting IDs in the message.
No live task lookup or per-bubble network request is needed. Existing messages
with usable context can render immediately; absent context stays plain text.

## Markdown composition

`useUserMentionComponents` in `chat-message.tsx`, consumed by
`UserMessageContent`, derives message-local task references and
passes them into the Markdown mention composition currently implemented by
`usePromptMentionMarkdownComponents`. Extract a small task renderer module
rather than grow the existing large prompt module.

Match literal `@` plus the recorded title, longest title first, using the
existing whitespace-start and Unicode name-boundary conventions. Use submitted
titles, preserving links after renames without querying current task titles.
Only matching plain text nodes in supported Markdown blocks become task chips.
Do not rewrite Markdown source or use string replacement to inject links.
Never decorate code, link labels/destinations, or raw content. When a saved
prompt shares a task title, a valid message-local task mapping takes priority
for that token. Preserve the current prompt and entity-reference components
for remaining content. Ensure component memoization includes task references.

`renderUserMessageBody` and its bounded previews continue controlling normal
and raw presentation. Extract references once from the full original message,
then decorate only visible preview content; previews do not expand hidden
context or bypass existing size limits.

## Navigation and responsive behavior

Render the chip with `components/routing/task-link.tsx` (`TaskLink`), preserving
normal anchor behavior and route hydration. Use a decorative task icon, a
restrained border/tint distinct from emerald prompt chips, and focus-visible
styling using existing tokens. Explicit chip color and decoration utilities
override the global Markdown anchor style, including on hover and in dark
mode. The visible label retains `@title`. Any added
accessible helper copy uses translations in all supported catalogs.

Desktop and phone share inline presentation and direct navigation. Phone and
coarse-pointer chips use a real minimum 44px active height scoped to those
conditions; fine-pointer desktop retains compact inline sizing. Long titles
wrap within the bubble, and adjacent chips flow onto subsequent lines with
nonoverlapping targets. No overlay, extra scroller, or saved layout change.
Use `useResponsiveBreakpoint` when a React branch is needed.

The nearest mobile exemplar is `kanban-with-preview.tsx`: its phone task
selection directly routes to the task. Existing `PromptMentionChip` supplies
inline styling/touch-sizing precedent, while its preview Drawer is unnecessary
for this explicit navigation action. The existing task layout supplies the
destination phone surface and conversation scroll ownership.

## Compatibility and failure behavior

No persisted format is added. Existing hidden task context supplies display
identity, not permission authority. `TaskLink` and the destination route retain
access handling; a removed or inaccessible task does not redirect to another
task with the same title. Arbitrary user text cannot supply an external URL.
Legacy messages with missing, sanitized, or ambiguous labels may remain text.
The legacy format has no occurrence IDs: equally named tasks cannot be safely
distinguished and deliberately receive no link. Raw/copy and agent delivery
remain byte-preserving under their existing behavior.

## Delivery and validation

The [implementation plan](../../../plans/chat-task-mentions/plan.md) owns unit,
component, and desktop/phone Playwright coverage. No new telemetry or ADR is
needed for this bounded presentation change using existing storage/navigation.
