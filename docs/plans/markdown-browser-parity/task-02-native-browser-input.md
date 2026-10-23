---
id: "02-native-browser-input"
title: "Enable scoped browser input"
status: done
wave: 2
depends_on: ["01-input-seam"]
plan: "plan.md"
requirements:
  - REQ-UI-MARKDOWN-FILE-EDITING-001
  - REQ-UI-MARKDOWN-FILE-EDITING-002
  - REQ-UI-MARKDOWN-FILE-EDITING-003
acceptance_criteria:
  - AC-UI-MARKDOWN-FILE-EDITING-001.2
  - AC-UI-MARKDOWN-FILE-EDITING-001.3
  - AC-UI-MARKDOWN-FILE-EDITING-001.5
  - AC-UI-MARKDOWN-FILE-EDITING-001.6
  - AC-UI-MARKDOWN-FILE-EDITING-001.10
  - AC-UI-MARKDOWN-FILE-EDITING-001.11
  - AC-UI-MARKDOWN-FILE-EDITING-002.6
  - AC-UI-MARKDOWN-FILE-EDITING-002.9
  - AC-UI-MARKDOWN-FILE-EDITING-002.10
  - AC-UI-MARKDOWN-FILE-EDITING-002.11
  - AC-UI-MARKDOWN-FILE-EDITING-002.12
  - AC-UI-MARKDOWN-FILE-EDITING-002.13
  - AC-UI-MARKDOWN-FILE-EDITING-003.11
system_design:
  - ../../specs/ui/system-design/markdown-file-editing.md
---

# Task 02: Enable scoped browser input

## Summary

Keep rendered Edit available without EditContext. Implement stable scoped
textarea input, exact source-range translation, and single-owner commands.
Prove paragraph and rendered table-cell editing in Firefox and WebKit.

## In scope

- Shadow-root input transport, normalized native projection, bidirectional
  offsets, context synchronization, keyboard/clipboard dispatch, and cleanup.
  Native selection uses a separate callback into model selection with user
  provenance and current instance/source revision. Reject stale and echoed events.
- Preserve mixed CRLF/LF, BOM, surrogate pairs, whitespace, and exact engine
  undo/redo. Never publish the complete normalized textarea value. Test selection
  without keyboard events, including backward and read-only selection. Comments,
  copy, and replacement must use the mapped range without dirty/history changes.
- Replace the missing-EditContext unavailable branch in desktop and phone
  capability/mode helpers. Keep actual failure/retry recovery.
- Add narrow `markdown-firefox` and `markdown-webkit` projects. Reuse current
  Markdown test files and backend fixtures. Do not broaden unrelated suites.

## Out of scope

- Global polyfill installation, DOM-based source selection, or another history.
- Replacing active table cells with source-only tables or new rich documents.

## Acceptance

- First reproduce `browser parity: rendered Edit works without EditContext` as
  a failure. It then passes in real Firefox and WebKit through the production
  adapter, including active table-cell edits, Save, and reload.
- Selection replacement, clipboard, undo/redo, mode transfer, comments, table
  insertion/resizing, and the exact-byte corpus pass. Native command defaults
  cannot produce duplicate edits or bypass read-only state.
- Two hybrid instances plus a Source editor do not share input, focus, or
  listeners. Browser globals remain unchanged. Existing Chromium behavior passes.

## UI preview

UI-01: Desktop Files retains file identity, Preview/Edit/Source, existing
actions, and Save. Firefox/Zen and Safari enter the same rendered document.
Active table cells retain borders and editing, with no delimiter row or pipe
glue. UI-02: Phone retains Back, identity, mode menu, actions, and visible Save.
The textarea is input plumbing, not another visible source surface. See the
[combined preview](plan.md#ui-preview). Criteria: `001.2`, `.10`, `002.9` through
`.12`, and `003.11`.

## Verification

The new projects and test files below are deliverables of this work order.
Commands start at the repository root and run sequentially.

```bash
pnpm --dir apps/web exec playwright install chromium firefox webkit
pnpm --dir apps/web exec vitest run components/editors/markdown/input/markdown-input-context.test.ts components/editors/markdown/input/markdown-textarea-input.test.tsx components/editors/markdown/hybrid-markdown-editor.test.tsx components/task/markdown-file-mode.test.ts components/task/markdown-file-editor.test.tsx components/task/mobile/mobile-file-viewer-panel.test.tsx
pnpm --dir apps/web run typecheck
pnpm --dir apps/web run i18n:check
pnpm --dir apps/web e2e:run --host --shards 1 --project markdown-firefox tests/task/markdown-file-editing.spec.ts -- --grep "browser parity:"
pnpm --dir apps/web e2e:run --host --shards 1 --project markdown-webkit tests/task/markdown-file-editing.spec.ts -- --grep "browser parity:"
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/task/markdown-file-editing.spec.ts
git diff --check
```

Retain missing-browser/runtime-library errors as blockers with their exact
command. Do not substitute Chromium simulation for the native-engine runs.

## Files likely touched

- `apps/web/components/editors/markdown/input/markdown-input-context.ts` and tests (new).
- `apps/web/components/editors/markdown/input/markdown-textarea-input.ts` and tests (new).
- `apps/web/components/editors/markdown/hybrid-markdown-editor.tsx` and its tests.
- `apps/web/components/task/markdown-file-mode.ts`, `markdown-file-editor.tsx`, and tests.
- `apps/web/components/task/mobile/mobile-file-viewer-panel.tsx` and tests.
- `apps/web/e2e/playwright.config.ts`, existing Markdown E2E files.
- Required locale keys if recovery copy changes. Remove browser exclusion copy
  consistently across catalogs, without removing actual runtime recovery copy.

## Dependencies

Task 01. Adapter, coordinator, and E2E files are shared, so execute sequentially.

## Risks

Textarea normalization changes source offsets. Shadow events may differ by
engine. Test real native clipboard events and read-only behavior. Handle
composition in the initial transport, then complete its platform interaction
coverage and corrections in Task 03 before shipping.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/markdown-file-editing.md).
- [Browser transport design](../../specs/ui/system-design/markdown-file-editing.md#browser-input-transport).
- [Input decision](../../decisions/2026-10-21-scoped-markdown-browser-input.md).
- Existing table source helpers, comment bridge, and source-position ports.

## Results

Implemented and verified through the production adapter. Missing or throwing
EditContext selects the scoped textarea without disabling rendered Edit.
TDD covers canonical normalization/range mapping, backward/read-only selection,
selection echo suppression, command ownership, cleanup, and constructor fallback.
The initial absent-API browser regression failed in Chromium and actual Firefox
before integration. It now passes in actual Firefox and WebKit, including Save.

The integrated targeted run passed 104 tests across 11 files. TypeScript,
translations, and owned-file ESLint passed. Native Firefox and WebKit passed
rendered tables, comments, clipboard/undo, LF/CRLF/mixed exact-byte preservation,
and synthetic composition. Task 03 owns final full-matrix results and native
mobile/assistive-technology evidence. Existing Source tests now use actual
keyboard typing rather than direct protocol insertion, which duplicated or
bypassed Monaco updates on Firefox/WebKit.
