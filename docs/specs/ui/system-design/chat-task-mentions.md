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
limits: 256 KiB of source, 200 references, 4,096 code units per record line,
512 per title, 256 per ID). Reject oversized lines before applying the record
regex, whose ID capture has the same finite safe-character limit. Find system
blocks with a forward `indexOf` scan and stop when no closing tag remains;
unmatched opening tags must not restart a scan over the remaining source.
When source exceeds the limit, skip task decoration rather than truncate into
a partial record. Ignore malformed records. Deduplicate identical pairs; discard
every mapping for a title associated with conflicting IDs in the message.
Also reject a record when its parsed title contains another complete reference
prefix with an eligible task ID: that prefix can be an earlier task record or
record-shaped step text, so the legacy string cannot identify one task safely.
Incomplete prefixes and ineligible embedded IDs in that parsed title do not
introduce a second candidate.
No live task lookup or per-bubble network request is needed. Existing messages
with usable context can render immediately; absent context stays plain text.

## Markdown composition

`useUserMentionComponents` in `chat-message.tsx`, consumed by
`UserMessageContent`, derives message-local task references and
passes them into the Markdown mention composition currently implemented by
`usePromptMentionMarkdownComponents`. Extract a small task renderer module
rather than grow the existing large prompt module.

Match literal `@` plus the recorded title, longest title first, using the
existing raw Markdown mention splitter and Unicode name-boundary conventions.
Use submitted titles, preserving links after renames without querying current
task titles. All verified task aliases use the same task-specific mapping,
independent of the saved-prompt name budget.

`prepareTaskMentionContent` calls
`lib/tasks/task-mention-markdown.ts` after the visible preview is bounded and
normalized with the renderer's shared `normalizeCached` helper.
The existing Unified/Remark parsing stack, now declared as direct dependencies
where used, applies the renderer's CommonMark, GFM, math, and math-compatibility
transform to identify code, authored link, image, HTML, and math source ranges.
Whitespace-delimited dollar prose retains the renderer's literal-text semantics.
Matches beginning inside those ranges remain untouched. Other
recognized aliases become render-only alphanumeric tokens, with a prefix absent
from the original source and a map back to the recorded references. Repeated
mentions reuse their task token. No HTML or URLs are inserted into Markdown.

`withTaskMentionTokens` composes block renderers over the existing prompt/entity
component map. It renders protected tokens as `TaskMentionChip` before delegating
remaining children to those existing components. Title punctuation, inline code
characters, link syntax, and emoji aliases within the task name therefore stay
literal. Authored code and links retain their existing behavior. Visible-text
boundary tracking includes the chip's original title, preventing an adjacent
prompt alias from becoming a separate mention without whitespace.

When a saved prompt shares a task title, a valid message-local task mapping
takes priority for that token. Preserve the current prompt and external entity
components for other content. Component and preparation memoization include the
task references and visible preview source.

`useUserMentionComponents` supplies both components and references to
`renderUserMessageBody`. Its normal Markdown path prepares only the visible
preview; raw view, copied/downloaded source, persisted messages, and agent input
continue using the original content. Hidden context never becomes visible and
render-only tokens never enter storage or message actions.

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
