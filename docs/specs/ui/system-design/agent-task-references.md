---
status: current
system: ui
created: 2026-11-16
requirements:
  - REQ-UI-AGENT-TASK-REFERENCE-001
---

# Agent task reference system design

## Purpose and boundaries

Extend agent-message Markdown with the task-chip presentation already used by
[user mentions](chat-task-mentions.md). This is a reusable UI reference contract.
Task data and access remain with Tasks. Use the existing
[task navigation contract](task-navigation.md).

The user selected direct navigation on 2026-11-16. A preview-first interaction
would add a confirmation to every reference. Desktop disclosure provides
optional context while preserving a single activation on all viewports.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-UI-AGENT-TASK-REFERENCE-001 | Authored format; Renderer integration; Presentation; Compatibility and safety; Agent guidance |

## Authored format

Agents emit ordinary Markdown: `[Task title](/t/task-id)`.
The title is the known task title, escaped for Markdown. The ID is the exact
task ID supplied by Kandev. No new URI scheme, tool, message field, or migration
is required. Normal Markdown remains readable outside the enhanced renderer.

Add a small pure recognizer under `apps/web/lib/tasks/agent-task-reference.ts`.
Accept only the exact root-relative `/t/` prefix followed by one ID containing
1 to 256 ASCII letters, digits, underscores, or hyphens. Do not decode or
normalize into eligibility. Reject query strings, fragments, encoded separators,
extra segments, whitespace, protocol-relative URLs, absolute URLs, and aliases.
These links retain existing Markdown behavior, without chip decoration.

Recognize parsed Markdown anchor nodes, not matches in source text. Require a
nonempty plain-text label, including escaped punctuation. Rich labels or image
children remain normal links. Renderer-owned text-motion spans retain plain-label
eligibility, including whitespace-only animation runs. Inline and reference-style Markdown links that
resolve to the same eligible anchor receive identical handling. Code nodes are
never visited as anchors. A parser-recognized anchor is the eligibility boundary
during streaming. No incremental regex or persistent token replacement is used.

## Renderer integration

Create a stable component map for agent task links in
`components/task/chat/messages/agent-task-reference-components.tsx`.
Compose its anchor renderer over `markdownComponents.a`. Recognized task links
must be checked before the file-link renderer, since `/t/id` resembles an
absolute file path. Delegate every other anchor with its original props.

Pass this map to `MemoizedMarkdown` in `AgentMessageContent` and the shared
`PlanMarkdownBody` used by `AgentPlanMessage`. Cover collapsed, expanded, and
dialog plan bodies. Do not alter the shared default renderer or user-mention
parser. Retain Markdown file context, text motion, and raw-view behavior.
The existing custom-component path bypasses the context-free element cache.
Keep the map stable to avoid recreating it for every streamed update.

Reuse `TaskMentionChip` with an optional presentation prop for the authored
agent label. Existing user callers retain `@title` and their test contract.
Agent references show the title without an added `@`, because the task icon
already identifies the object. Both variants use the same shape and colors.
The actual anchor remains `TaskLink`, which owns the canonical destination via
`linkToTask`. Never build a separate navigation handler or nest a button/link
inside the anchor.

## Presentation

Fine-pointer desktop keeps the existing compact 24px minimum chip height,
task icon, rounded corners, restrained tint, and visible focus outline.
Read the referenced task's personal color through `useTaskColor`, which owns
the persisted user-settings projection. Apply matching text, border, and
background tokens, with readable light and dark variants. Use the primary
accent when the personal color is absent or cleared. Both agent references
and user mentions follow changes in the shared chip without reloads.
Use existing theme tokens and Tabler icons. Permit title wrapping rather than
hiding identity behind an ellipsis. No pulse, entrance sequence, or animated
status dot is needed. Hover feedback changes only background color, at no more
than 150ms, behind the hover-capable media condition.

Use the shared Tooltip primitive with `TaskLink` as its trigger for a short,
noninteractive disclosure. Show the full authored title first. If a loaded
record has a different title, label its current title separately. Below it,
show status and session-count badges using existing domain presentation and
localized plural labels. Zero is a valid count. Omit unknown counts.
Use labeled rows for creation date, workflow, and parent. Validate the creation
timestamp with the shared RFC3339 parser, then format its date and time for the
active locale. Omit invalid or missing dates. Resolve the parent title from the
canonical overview. Omit the parent row until its title is available. Keep the parent row
noninteractive so the tooltip retains its disclosure-only contract.
Omit the task description from the disclosure. Keep the card focused on metadata.
Display the task identifier when loaded. Use a 22rem-wide card constrained to
the viewport, with wrapping badge and metadata rows.
Read `taskOverview.byId` and loaded workflow state through a small domain hook.
The canonical overview includes sidebar-owned tasks even when no board is open.
Resolve a known workflow ID directly against loaded workflows or its keyed
snapshot. Only legacy records without a workflow ID scan snapshot tasks. Memoize
the lookup over its source slices so unrelated store updates do not repeat it.
Do not limit the lookup to legacy board arrays. Shared mention chips use
`primary` and `ring` theme tokens for color and focus treatment.
Do not fetch, mount `useTask`, subscribe to task rooms, or poll for previews.
The snapshot is optional context, not an assertion that metadata is fresh.
If metadata is absent, show localized guidance to open the task for details.

The existing `TaskTitleHoverCard` demonstrates the timing and typography but
its button trigger and subtask expansion are unsuitable for a direct link.
Do not reuse that trigger or load subtask data. Tooltip content stays within the
viewport and has no controls or separate scroll owner. Enter retains anchor
activation. Escape dismisses the tooltip and focus stays on the anchor.

On phones or coarse pointers, omit the tooltip and use direct navigation.
The nearest shipped exemplar is `TaskMentionChip`, verified by
`mobile-chat-task-mentions.spec.ts`. Agent references use a 24px minimum height
and snug line height on all viewports so inline text is not stretched. Existing
user mentions keep their touch sizing. Long and adjacent references wrap
with nonoverlapping hit areas. Keep the existing conversation scroller and
destination safe-area behavior. No new fixed surface or persistent preference.

All added interface labels use `t()` and all seven language catalogs. Task
titles and workflow names remain data. Generate the Traditional Chinese pair
through `pnpm run i18n:zh-hant`. Accessible names contain the authored title.

## Agent guidance

Add concise guidance to `apps/backend/config/prompts/kandev-context.md` and
`office-context.md`, preserving their existing identity and authorization rules:

> In chat, write `[Task title](/t/task-id)` using Kandev's exact task ID and known title. Escape Markdown punctuation.
> If the ID is unknown, use plain text. Do not guess an ID.
> A task reference does not authorize creating or changing a task.

Test the formatted prompts through `internal/sysprompt`, including explicit
built-in package bindings. The Markdown format does not depend on an MCP tool
being enabled. This changes agent guidance, not protocol adapters or model
capabilities. The prompt-size regression test adds a 300-byte allowance for
this guidance. Existing authorization instructions remain unchanged. No compliance
guarantee can be made for arbitrary model output.

## Compatibility and safety

The authored label stays stable after a task rename. Each link keeps its own
ID, including equal titles for different tasks. No title search or global title
map is used. The destination route remains responsible for missing tasks and
access failures. Metadata comes only from currently loaded application state.

Use React text escaping and the existing Markdown URL sanitization. Do not add
raw HTML support or broaden allowed protocols. Raw view, copy, downloaded
Markdown, message comments, and persistence retain original source content.
Historical eligible links gain decoration when reopened. Other content keeps
its prior behavior. No new backend metrics or storage are needed.

## Validation and related contracts

Unit tests prove strict route recognition, composed Markdown rendering,
streaming, raw/copy invariants, and optional cached metadata. Browser tests
prove navigation without a document reload, focus, hover, mobile geometry,
Back, refresh, and duplicate-title destinations.

[ADR 0046](../../../decisions/0046-settings-route-save-coordinator.md) retains
navigation guard authority. This feature follows existing navigation and
Markdown contracts, so a separate ADR is unnecessary.

## Implementation plans

- [Agent task references](../../../plans/agent-task-references/plan.md)
