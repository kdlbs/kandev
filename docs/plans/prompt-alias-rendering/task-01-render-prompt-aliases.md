---
id: task-01-render-prompt-aliases
title: Render saved prompt aliases consistently
status: pending
wave: 1
depends_on: []
plan: docs/plans/prompt-alias-rendering/plan.md
requirements:
  - REQ-UI-PROMPT-ALIAS-001
acceptance_criteria:
  - AC-UI-PROMPT-ALIAS-001.1
  - AC-UI-PROMPT-ALIAS-001.2
  - AC-UI-PROMPT-ALIAS-001.3
  - AC-UI-PROMPT-ALIAS-001.4
  - AC-UI-PROMPT-ALIAS-001.5
system_design: docs/specs/ui/system-design/prompt-alias-rendering.md
---

# Render saved prompt aliases consistently

## Summary

Reuse the transcript's saved-prompt chip presentation in the anchored last-
prompt bar and Prompt history, with no change to persisted message content or
agent delivery.

## Scope

- Extract the prompt-name subscription, prompt mention component factory, and
  chip presentation from `apps/web/components/task/chat/messages/chat-message.tsx`
  into a reusable module.
- Keep `lib/prompts/prompt-mention-segments` as the only matching implementation.
- Pass shared Markdown components to
  `apps/web/components/task/chat/anchored-last-prompt-bar.tsx`.
- Render recognized aliases through the shared text renderer in
  `apps/web/components/task/prompt-history-panel-content.tsx` while preserving
  its collapsed overflow measurement, expansion cap, navigation, and mobile
  hit area.
- Add focused Vitest assertions for recognized and unrecognized aliases in the
  pinned and history surfaces; retain existing transcript assertions.
- Extend the relevant existing Playwright coverage if the fixture can seed a
  saved prompt without introducing a separate test-only data path.

## Exclusions

- Prompt parser, backend expansion, prompt persistence, APIs, WebSocket payloads,
  entity-reference behavior, prompt numbers, and raw-message rendering.
- General Markdown redesign or a second Prompt history layout.

## Likely files

- `apps/web/components/task/chat/messages/chat-message.tsx`
- `apps/web/components/task/chat/messages/prompt-mention-components.tsx`
- `apps/web/components/task/chat/messages/chat-message.test.tsx`
- `apps/web/components/task/chat/anchored-last-prompt-bar.tsx`
- `apps/web/components/task/chat/anchored-last-prompt-bar.test.tsx`
- `apps/web/components/task/prompt-history-panel-content.tsx`
- `apps/web/components/task/prompt-history-panel-content.test.tsx`
- `apps/web/e2e/tests/chat/last-prompt-scroll.spec.ts`
- `apps/web/e2e/tests/task/prompt-history-panel.spec.ts`

## Targeted verification

```text
cd apps/web && pnpm exec vitest run \
  components/task/chat/messages/chat-message.test.tsx \
  components/task/chat/anchored-last-prompt-bar.test.tsx \
  components/task/prompt-history-panel-content.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm exec eslint \
  components/task/chat/messages/chat-message.tsx \
  components/task/chat/messages/prompt-mention-components.tsx \
  components/task/chat/anchored-last-prompt-bar.tsx \
  components/task/prompt-history-panel-content.tsx
```

Run the focused Playwright spec only when the local E2E backend/profile is
available:

```text
cd apps/web && pnpm exec playwright test \
  e2e/tests/chat/last-prompt-scroll.spec.ts \
  e2e/tests/task/prompt-history-panel.spec.ts
```

## Risks

The history row is intentionally not converted to block Markdown: its one-line
collapsed projection depends on measuring a text span. The shared text renderer
must therefore produce the same chip nodes without changing row geometry.
