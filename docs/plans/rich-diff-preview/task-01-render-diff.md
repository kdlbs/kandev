---
id: "01-render-diff"
title: Render unified diffs in FilePreview
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-AGENTS-RICH-DIFF-PREVIEW-001
acceptance_criteria:
  - AC-AGENTS-RICH-DIFF-PREVIEW-001.1
  - AC-AGENTS-RICH-DIFF-PREVIEW-001.2
  - AC-AGENTS-RICH-DIFF-PREVIEW-001.3
  - AC-AGENTS-RICH-DIFF-PREVIEW-001.4
  - AC-AGENTS-RICH-DIFF-PREVIEW-001.5
system_design:
  - ../../specs/agents/system-design/rich-diff-preview.md
---

# Render unified diffs in FilePreview

## Outcome and scope

Implement the [plan](plan.md) in file-preview-block.tsx and a colocated renderer for both layouts. Reuse existing diff parsing utilities if they preserve source lines. Update rich-output guidance, regenerate its embedded artifacts, update public docs and translate new controls in all locales. Patch editing and inline MCP payloads are outside this work order.

## Acceptance and tests

1. For AC-AGENTS-RICH-DIFF-PREVIEW-001.1 and .4, exercise timestamp headers, multiple files, repeated signs inside hunks, CRLF and ordinary text fallback in diff-preview-model.test.ts. Preserve plugin precedence and the existing error flow.
2. For AC-AGENTS-RICH-DIFF-PREVIEW-001.2, .3 and .5, extend rich-output.spec.ts and mobile-rich-output.spec.ts with a real workspace patch. Assert the initial side-by-side selection, old/new column order and blank cells for unmatched changes. Page to a later hunk, switch to Unified, verify source order and the current change, then switch back. Reach the final hunk and reopen the preview. Check signs, keyboard navigation, and phone viewport overflow with a long line.
3. Check that the catalog guidance shows an agent how to preview a saved patch using FilePreview.

## ASCII UI preview

UI-01. Expanded preview on desktop and phone. Side by side is selected on first open.

```text
[Side by side *] [Unified]
Before                  | After
@@ -1 +1 @@
- "eventId": 141402386  | + "eventId": 141403809
```

UI-02. The reader selects Unified in the same card.

```text
[Side by side] [Unified *]
@@ -1 +1 @@
- "eventId": 141402386
+ "eventId": 141403809
```

On phones, place the file actions below the path and the layout selector above the patch. Keep the same column order, allow horizontal scrolling inside the patch, and place paging below it. The asterisk illustrates the selected state.

## Verification commands

From apps/web:

```sh
pnpm rich-output:generate
pnpm rich-output:check
pnpm exec vitest run components/task/chat/messages/kandev/rich-output lib/rich-output/guidance.test.ts
pnpm e2e:run --project chromium tests/chat/rich-output.spec.ts -- --grep 'switches diff preview'
pnpm e2e:run --no-build --project mobile-chrome tests/chat/mobile-rich-output.spec.ts -- --grep 'switches diff preview'
```

## Results

Implemented. All 80 focused unit tests passed. Desktop Chromium and mobile Chrome each passed the 733-line workspace patch scenario. Typecheck, focused lint, locale checks and generated catalog validation passed.
