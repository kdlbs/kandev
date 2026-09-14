---
status: draft
system: ui
requirements:
  - REQ-UI-THINKING-MESSAGE-PREVIEW-001
---

# Thinking Message Preview System Design

## Purpose and boundaries

This design derives a stable collapsed preview from the thinking content that
the web client already receives. It changes only the task-chat presentation.
The ACP adapter, lifecycle coalescer, task-message persistence, boot payload,
and WebSocket message contract remain unchanged.

An ACP probe against Codex 1.6.0 emitted four thought chunks under one stable
message ID: blank lines, a bold summary, blank lines, and a second bold
summary. Kandev persisted their ordered concatenation. The current
`ThinkingMessage` classifies any text containing a newline as expandable but
only renders inline text for non-expandable messages. The useful first summary
therefore disappears from the collapsed row.

## Requirement mapping

| Requirement                           | Design section                                                                                                                                                                                              |
| ------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `REQ-UI-THINKING-MESSAGE-PREVIEW-001` | [Preview derivation](#preview-derivation), [Rendering contract](#rendering-contract), [Streaming behavior](#streaming-behavior), [Responsive behavior](#responsive-behavior), [Verification](#verification) |

## Components and responsibilities

- `ThinkingMessage` reads the existing `metadata.thinking` string, with
  `comment.content` as its current fallback, and owns preview derivation and
  header layout.
- `ExpandableRow` continues to own collapse and expansion without a new prop or
  interaction mode.
- `MemoizedMarkdown` continues to render the complete source only in the
  expanded content region.
- The task-message store and WebSocket handlers continue to replace each live
  message with the latest complete public state.

## Preview derivation

`ThinkingMessage` uses a pure, model-agnostic helper:

1. Split the source on Unix or Windows line endings.
2. Process lines in source order with conservative preview-specific inline
   Markdown normalization.
3. Trim the plain-text result and select the first non-empty value.
4. Return an empty preview when no line produces visible text.

The helper does not parse provider metadata, translate agent content, generate
new copy, or persist the preview. Markdown links contribute their visible
label, balanced emphasis and code delimiters are removed, structural-only
Markdown lines are skipped, and identifier punctuation is preserved. React
renders the result as escaped text rather than Markdown or HTML.

Compact single-line detection remains unchanged. Its complete stripped text
continues to render inline, and the row remains non-expandable. The compact
text stays complete and may wrap at narrow widths, but it is not truncated or
replaced by the expandable preview. Expandable messages use the derived
preview in the same header position.

## Rendering contract

The header keeps the localized Thinking label as a non-shrinking prefix. The
preview occupies the remaining width in a `min-w-0` flex region and uses
single-line truncation. The complete source remains in the current expanded
Markdown region, so truncation never changes or discards transcript content.

The compact inline text uses a separate shrinkable, wrapping flex item. Its
complete text remains readable on narrow screens without widening the
transcript.

When preview derivation returns an empty string, the header renders only the
Thinking label. No placeholder, tooltip, or new user-facing copy is added.

## Streaming behavior

Reasoning updates append ordered content to one thinking message. The client
derives the preview from the current complete string on each render. Leading
blank chunks initially produce no preview. The preview appears when the first
meaningful line arrives. Later appended lines cannot precede that line, so the
derived preview remains stable without local state, memoized protocol IDs, or
an additional streaming lifecycle.

Animation-frame WebSocket replacement can skip intermediate renders, but each
delivered message contains the complete current thinking string. Preview
derivation therefore reaches the same result after replacement or reload.

## Responsive behavior

Desktop and mobile use the same `ThinkingMessage` component and derivation.
This is an inline content change inside the existing task-chat row. It adds no
navigation, overlay, scroll owner, control, or touch target.

The nearest mobile exemplar is the current thinking row in
`apps/web/e2e/tests/chat/mobile-markdown-wrap.spec.ts`. The localized label
keeps its current priority. An expandable preview truncates before it can
widen the chat or document, while compact text remains complete and can wrap
within the row. Expanding the row continues to use the existing full-width
Markdown region and its local overflow handling.

## Failure and security behavior

- Empty, whitespace-only, or decoration-only content produces no preview and
  keeps the existing label-only fallback.
- A malformed or unavailable thinking value follows the component's existing
  empty-message handling; this change does not broaden accepted message data.
- Preview text is rendered as a React text node. Markdown cannot create links,
  raw HTML, executable content, or additional interactive elements in the
  header.
- Expanded content continues through the existing Markdown safety boundary.

## Verification

Focused component tests cover leading blank lines, conservative Markdown
stripping, structural-only lines, identifier punctuation, the label-only
fallback, compact single-line preservation and wrapping classes, later
appended content, expansion, and the truncating header classes.

A `mobile-chrome` Playwright scenario seeds Codex-shaped multiline and compact
thinking messages, waits for their persisted content, verifies the meaningful
preview and expansion behavior, checks complete compact text with allowed
wrapping, and checks that the preview, chat, and document remain horizontally
contained.

No production telemetry is added. This is a deterministic presentation rule
with component and browser evidence.

## Related decisions

- [Isolate Replaceable Session Stream Traffic](../../../decisions/2026-08-02-isolate-replaceable-session-stream-traffic.md)
