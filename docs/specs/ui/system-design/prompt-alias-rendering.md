---
status: draft
system: ui
requirements:
  - REQ-UI-PROMPT-ALIAS-001
created: 2026-09-02
owners:
  - kandev
---

# Prompt Alias Rendering System Design

## Purpose and boundaries

The UI system owns the presentation contract for saved prompt aliases across
multiple task transcript surfaces. Prompt data and alias matching remain owned
by the existing prompt store and `lib/prompts/prompt-mention-segments` helpers.
Agent-facing prompt expansion and persistence are unchanged.

## Requirement mapping

| Requirement               | Design section                                                                                     |
| ------------------------- | -------------------------------------------------------------------------------------------------- |
| `REQ-UI-PROMPT-ALIAS-001` | [Components and responsibilities](#components-and-responsibilities), [Control flow](#control-flow) |

## Components and responsibilities

- `lib/prompts/prompt-mention-segments` remains the single matcher. It builds
  normalized prompt names and splits text into ordinary and recognized prompt
  segments.
- A shared prompt-mention presentation module under
  `apps/web/components/task/chat/messages/` owns the prompt chip, prompt lookup,
  and Markdown component factory currently embedded in `chat-message.tsx`.
  It exposes both the Markdown components used by rich transcript content and a
  text-segment renderer for single-line Prompt history rows.
- `ChatMessage` consumes the shared factory for the existing transcript user
  bubble. Its entity-reference component composition remains transcript-owned.
- `AnchoredLastPromptBar` passes the shared prompt components to
  `MemoizedMarkdown` after stripping system tags, preserving the existing
  Markdown renderer and height/overflow behavior.
- `PromptHistoryPanelContent` uses the shared text-segment renderer in both
  the collapsed and expanded row content. The row keeps its current text span,
  truncation measurement, expansion cap, navigation, and touch sizing. Content-
  bearing chips remain keyboard-focusable and intercept activation so keyboard
  and touch preview access does not navigate the row. Chips rendered inside
  Markdown links are visual-only, avoiding nested interactive semantics while
  preserving link activation.
- `MemoizedMarkdown` remains the common Markdown renderer and continues to
  normalize content through its existing cache. The change does not add raw HTML
  or alter the Markdown safety policy.

## Data and contracts

No backend, HTTP, WebSocket, or persistence contract changes. The shared UI
renderer reads `state.prompts.items`, matching the existing transcript path.
Recognized aliases are represented by the existing `custom-prompt-mention`
test ID and `data-prompt-name` attribute. Unknown aliases remain text.

The shared Markdown factory accepts optional entity-reference components so the
transcript can preserve its current entity chip behavior, while pinned content
(which has no message metadata) and history rows use the empty entity-reference
set.

## Control flow

1. Each mounted surface derives prompt names from the current prompt store.
2. The shared matcher identifies only recognized aliases using the existing
   boundary and name rules, excluding code spans and link destinations.
3. Rich Markdown surfaces pass the shared component map to `MemoizedMarkdown`,
   which injects prompt chips into supported Markdown block children.
4. Prompt history passes each plain row text through the shared segment renderer,
   preserving the row's single-line CSS measurement and expanded layout.
5. A chip looks up its current saved prompt by name. Existing non-empty prompts
   receive the hover preview; empty or missing content receives the existing
   title-only chip.
6. Store updates invalidate the derived names/components through React store
   subscriptions, so mounted views reconcile without changing message data.

## Failure and recovery

If the prompt store is not loaded or a name is absent, the alias is rendered as
ordinary text or a title-only chip according to the existing transcript behavior.
The renderer never modifies or expands persisted message content. If a surface
has no prompt text, its current empty state remains unchanged.

## Security

The change only reuses the existing prompt chip and `MemoizedMarkdown` safety
path. Prompt content shown in hover previews remains the same content already
available to the authenticated prompt store. No new HTML, URL, or navigation
handling is introduced.

## Observability

No new runtime metrics or logs are required. Unit coverage will assert shared
recognized/unknown behavior and surface-specific rendering; existing anchored
bar, ChatMessage, and Prompt history E2E coverage remains the behavioral smoke
signal.
