---
created: 2026-10-21
status: implemented
requirements:
  - REQ-UI-CHAT-TASK-MENTION-001
system_design:
  - ../../specs/ui/system-design/chat-task-mentions.md
legacy_specs: []
---

# Implementation Plan: Chat Task Mentions

## Overview

Deliver one vertical slice: parse existing persisted reference context, render
task chips inside sent user bubbles, and verify direct task navigation on
desktop and phone. Implementation and targeted verification are complete.

## Scope

In scope: normal user bubbles, existing historical reference context,
Markdown composition, canonical navigation, accessibility, mobile wrapping,
and focused regression coverage.

Out of scope: new persistence fields, mention search, composer redesign,
assistant messages, previews, and task lifecycle changes.

## Technical approach

- Add bounded `lib/tasks/task-mention-references.ts` extraction and matching.
- Add `components/task/chat/messages/task-mention-components.tsx` using
  `TaskLink`; integrate in `chat-message.tsx` and the existing prompt/entity
  Markdown composition. Preserve `user-message-body.tsx` preview/raw behavior.
- Protect recorded task aliases with render-only tokens after source
  range checks; preserve authored code and links through the CommonMark parser.
- Use original message context for identity, never the currently loaded task
  list. Skip conflicting title mappings and invalid records.
- Include translated accessibility copy only if needed. During implementation
  use `/docs-maintainer` to add a short how-to section to the appropriate chat
  documentation (or the smallest existing public workflow page); no public
  behavior documentation changes in this design-only package.

## ASCII UI preview

### UI-01: Sent task reference, normal chat bubble

Entry: send a message after selecting a task in the existing `@` menu.

```text
Before (desktop and phone):
  check if @Investigate terminal retry is fixing it?

After (desktop):
  check if [task-icon @Investigate terminal retry] is fixing it?

After (phone, long title):
  check if
  [task-icon @Investigate terminal ]
  [          turn capture retry   ]
  is fixing it?

Unrecognized or ambiguous:
  check if @something is fixing it?
```

Brackets denote one tinted task link, not literal UI text. Required structure:
icon plus title, distinct styling, visible focus, direct task destination,
44px phone/coarse-pointer active height, and wrapping within the bubble.
Phone diagram shows one wrapping chip, not two actions. Exact color and
spacing are illustrative. Existing chat scroll ownership remains unchanged.
Raw view remains original text. Covers AC-UI-CHAT-TASK-MENTION-001.1 through .4.

## Tests

- `lib/tasks/task-mention-references.test.ts`: valid historical/current blocks,
  duplicate IDs/titles, malformed delimiters, punctuation, Unicode, matching
  boundaries, and source/reference limits (AC .1, .2, .3).
- `components/task/chat/messages/task-mention-components.test.tsx`: correct
  task identity/href, saved-prompt name collisions, nested Markdown, link/code
  exclusions, raw view, bounded previews, and retained entity rendering
  (AC .1, .2, .3). Exercise the actual composed message renderer.

## E2E tests

- `e2e/tests/chat/chat-task-mentions.spec.ts`, `chromium`: select a real task
  in the composer, send, assert chip/link, refresh, activate with keyboard,
  and verify exact destination; include a renamed target and plain `@text`
  control (AC .1, .2, .3).
- `e2e/tests/chat/mobile-chat-task-mentions.spec.ts`, `mobile-chrome`: send
  long/adjacent mentions, measure active target and bubble containment, assert
  no horizontal overflow, tap to the exact task, and return to the conversation
  (AC .1, .2, .4). Include a narrow fine-pointer check of phone target sizing.
- Inspect desktop light/dark and phone screenshots against UI-01 as part of
  these focused runs. Do not introduce backend mocks to prove navigation.

## Work orders

- [x] [Task 01: Render and navigate task mentions](task-01-render-task-mentions.md)

Single sequential work order; no implementation delegation is authorized.

## Verification results

All checks passed. See the [work-order results](task-01-render-task-mentions.md#results)
for commands and evidence. Desktop light/dark and phone captures were inspected
against UI-01.

## Risks

- Hidden reference records are a legacy text format; malformed or ambiguous
  records must degrade to plain text without guessing task identities.
- Markdown decoration can interfere with prompt/entity links unless tested
  through the composed renderer.
- Phone targets must fit the existing narrow bubble without overlapping.
