---
id: "01-render-thinking-message-previews"
title: "Render thinking-message previews"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-THINKING-MESSAGE-PREVIEW-001
acceptance_criteria:
  - AC-UI-THINKING-MESSAGE-PREVIEW-001.1
  - AC-UI-THINKING-MESSAGE-PREVIEW-001.2
  - AC-UI-THINKING-MESSAGE-PREVIEW-001.3
  - AC-UI-THINKING-MESSAGE-PREVIEW-001.4
  - AC-UI-THINKING-MESSAGE-PREVIEW-001.5
  - AC-UI-THINKING-MESSAGE-PREVIEW-001.6
  - AC-UI-THINKING-MESSAGE-PREVIEW-001.7
system_design:
  - ../../specs/ui/system-design/thinking-message-preview.md
---

# Task 01: Render Thinking-Message Previews

## Summary

Show the first meaningful reasoning line in expandable thinking-message
headers. Preserve complete Markdown content, compact single-line behavior, and
provider-neutral message contracts while containing the preview on mobile.

## In scope

- Write the focused component regression first.
- Derive a plain-text preview from the first meaningful source line.
- Render and truncate the preview in expandable thinking-message headers.
- Preserve label-only, compact-inline, streaming, and expanded-content paths.
- Add focused mobile browser evidence for visibility and containment.

## Out of scope

- Agent adapter, lifecycle, task-message service, persistence, and WebSocket
  changes.
- Generated summaries, provider branches, new translations, or settings.
- Changes to `ExpandableRow`, the Markdown renderer, or shared transcript
  navigation.

## Acceptance

- A Codex-shaped multiline thought shows its first non-empty Markdown-stripped
  line beside Thinking before expansion, while later appended lines do not
  replace it.
- Empty or decoration-only thoughts retain the label-only fallback, and compact
  single-line thoughts retain their current full inline behavior while wrapping
  when the row is narrow.
- The expandable preview stays on one truncated line without widening the
  mobile chat or document, and expansion reveals the complete source.

## Verification

Bootstrap workspace dependencies once when `apps/node_modules` is absent:

```bash
cd apps && pnpm install --frozen-lockfile
```

Run the focused TDD and validation commands:

```bash
cd apps && pnpm --filter @kandev/web test -- components/task/chat/messages/thinking-message.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm exec eslint components/task/chat/messages/thinking-message.tsx components/task/chat/messages/thinking-message.test.tsx e2e/tests/chat/mobile-markdown-wrap.spec.ts
cd apps/web && pnpm exec prettier --check components/task/chat/messages/thinking-message.tsx components/task/chat/messages/thinking-message.test.tsx e2e/tests/chat/mobile-markdown-wrap.spec.ts
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans apps/web/components/task/chat/messages apps/web/e2e/tests/chat/mobile-markdown-wrap.spec.ts
cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/chat/mobile-markdown-wrap.spec.ts -- --grep "thinking preview" --retries=0
```

## Files likely touched

- `apps/web/components/task/chat/messages/thinking-message.tsx`
- `apps/web/components/task/chat/messages/thinking-message.test.tsx`
- `apps/web/e2e/tests/chat/mobile-markdown-wrap.spec.ts`
- `docs/specs/ui/requirements/thinking-message-preview.md`
- `docs/specs/ui/system-design/thinking-message-preview.md`
- `docs/specs/ui/README.md`
- `docs/plans/thinking-message-preview/plan.md`
- `docs/plans/thinking-message-preview/task-01-render-thinking-message-previews.md`

## Dependencies

None.

## Risks

- A Markdown structural line can be a technically valid but less descriptive
  preview. Do not add semantic ranking beyond the specified first visible line.
- Flex truncation fails if a header ancestor cannot shrink. Cover the final DOM
  geometry in the mobile browser scenario.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-THINKING-MESSAGE-PREVIEW-001` and all acceptance criteria.
- Preview derivation, rendering, streaming, and responsive sections in the
  thinking-message-preview system design.
- Existing `ThinkingMessage`, `ExpandableRow`, message-renderer tests, and
  mobile Markdown containment scenario.

## Results

Implemented a model-agnostic first-meaningful-line helper and rendered its
plain-text result beside the localized Thinking label for expandable rows.
Compact single-line messages remain complete, non-expandable, and readable when
their inline text wraps; expanded Markdown content keeps its existing behavior.
The header uses shrink-safe flex regions and truncates only the expandable
preview; structural Markdown-only lines are skipped and identifier punctuation
is preserved in previews.

Verification:

- `cd apps && pnpm --filter @kandev/web test -- components/task/chat/messages/thinking-message.test.tsx` — 7 passed.
- `cd apps/web && pnpm run typecheck` — passed.
- Targeted ESLint and Prettier checks — passed.
- `python3 scripts/lint-spec-files.test.py` and `python3 scripts/lint-spec-files.py --all` — passed.
- `cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/chat/mobile-markdown-wrap.spec.ts -- --grep "thinking preview" --retries=0` — 2 passed, including complete compact text wrapping.
- Disposable PR capture E2E — 1 passed; desktop and 393px mobile screenshots were inspected and compressed.
