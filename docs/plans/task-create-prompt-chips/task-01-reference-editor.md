---
id: 01-reference-editor
title: Build the reference editor
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-UI-PROMPT-ALIAS-002
acceptance_criteria:
  - AC-UI-PROMPT-ALIAS-002.1
  - AC-UI-PROMPT-ALIAS-002.2
  - AC-UI-PROMPT-ALIAS-002.3
  - AC-UI-PROMPT-ALIAS-002.4
  - AC-UI-PROMPT-ALIAS-002.5
  - AC-UI-PROMPT-ALIAS-002.6
system_design:
  - ../../specs/ui/system-design/prompt-alias-rendering.md
---

# Task 01: Build the reference editor

## Summary

Build a scoped editor from installed Tiptap primitives. Its external contract
remains plain text, with recognized references as removable inline chips.

## In scope

- Text/document conversion, offset mapping, chip recognition, and store reconciliation.
- Shared chip appearance, desktop/touch preview, occurrence-specific removal, and undo.
- Prompt suggestion selection without full-definition insertion or implicit submission.
- IME-safe typing, clipboard text, external value changes, and stable selection.

## Out of scope

- Production task-form integration, backend delivery, and chat/session editor changes.

## Acceptance

- Round trips and offset tests preserve exact draft text across all mapped criteria.
- Editor tests prove selection, removal, previews, undo, and delayed prompt data.
- Existing inline/context insertion tests and shared chip tests remain green.

## ASCII UI preview

UI-01/02 excerpt from the [full preview](plan.md#ascii-ui-preview), criteria `.1` to `.6`:

```text
Editable goal text
[@create-canvas] [x]
Desktop activation -> prompt preview
Phone tap          -> inset preview drawer
```

Removal is separate from preview activation. Touch controls have 44px hit areas.
The editor keeps bounded scrolling and text wrapping. UI-03 fallback remains ordinary text.

## Verification

Run from the repository root. Use TDD and record the failing regression before implementation.
New file paths below are deliverables of this work order.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/prompts/task-prompt-document.test.ts components/task-prompt-reference-editor.test.tsx hooks/use-inline-mention.test.ts components/task/chat/messages/prompt-mention-components.round2.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint lib/prompts/task-prompt-document.ts components/task-prompt-reference-editor.tsx hooks/use-inline-mention.ts)
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- New `apps/web/lib/prompts/task-prompt-document.ts` and its test.
- New `apps/web/components/task-prompt-reference-editor.tsx` and its test.
- Extracted editor hooks/extensions and adjacent tests as needed for size limits.
- `apps/web/components/task/chat/messages/prompt-mention-components.tsx`.
- `apps/web/hooks/use-inline-mention.ts` and its test.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/task.json` for new action labels.

## Dependencies

None. Reuse the shared renderer already present on this branch.

## Risks

Do not rebuild the entire editor document on each React render or store update.
Do not share session draft keys or import chat history behavior.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/ui/requirements/prompt-alias-rendering.md), requirement 002.
- [Design](../../specs/ui/system-design/prompt-alias-rendering.md#task-creation-editor).
- `use-tiptap-editor.ts`, `tiptap-mention-extension.tsx`, and `PromptMentionChip` as source precedents.

## Results

Implemented the plain-text-backed Tiptap editor, reference document conversion,
occurrence-specific removal, touch-sized controls, and create-mode autocomplete.
Task 02 owns the rendered desktop and phone verification.

Verification on 2026-09-12:

- `(cd apps && pnpm install --frozen-lockfile)`: passed.
- `cd apps/web && pnpm exec vitest run lib/prompts/task-prompt-document.test.ts components/task-prompt-reference-editor.test.tsx hooks/use-inline-mention.test.ts components/task/chat/messages/prompt-mention-components.round2.test.tsx`: 4 files and 37 tests passed.
- `cd apps/web && pnpm run typecheck`: passed.
- `cd apps/web && pnpm exec eslint lib/prompts/task-prompt-document.ts components/task-prompt-reference-editor.tsx hooks/use-inline-mention.ts`: passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
