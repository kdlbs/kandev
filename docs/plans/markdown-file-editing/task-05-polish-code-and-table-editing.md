---
id: "05-polish-code-and-table-editing"
title: "Polish Code and Table Editing"
status: completed
wave: 5
depends_on: ["04-markdown-editor-e2e"]
plan: "plan.md"
requirements:
  - REQ-UI-MARKDOWN-FILE-EDITING-001
  - REQ-UI-MARKDOWN-FILE-EDITING-002
  - REQ-UI-MARKDOWN-FILE-EDITING-003
acceptance_criteria:
  - AC-UI-MARKDOWN-FILE-EDITING-001.9
  - AC-UI-MARKDOWN-FILE-EDITING-002.9
  - AC-UI-MARKDOWN-FILE-EDITING-003.7
system_design:
  - ../../specs/ui/system-design/markdown-file-editing.md
---

# Task 05: Polish Code and Table Editing

## Summary

Connect the hybrid editor's bundled syntax-highlighting seam and add
source-preserving table row and column controls with consistent Preview and
Edit borders.

## In scope

- Theme-aware syntax highlighting for supported fenced code blocks.
- A two-pixel active-block radius.
- Visible table borders in Preview and Edit.
- Undoable append-row and append-column actions for active Edit-mode tables.
- Compact fine-pointer and touch-sized coarse-pointer controls.
- Focused unit, component, desktop, and mobile verification.

## Out of scope

- Cell merging, row or column deletion, reordering, or rich spreadsheet
  behavior.
- Persisted table widths or non-Markdown table formats.
- Replacing the source-preserving hybrid engine.

## Verification

```bash
cd apps
pnpm --filter @kandev/web test -- --run components/editors/markdown
pnpm --filter @kandev/web typecheck
pnpm --dir web run i18n:check
pnpm --dir web e2e:run tests/task/markdown-file-editing.spec.ts
pnpm --dir web e2e:run -- --project=mobile-chrome tests/task/mobile-markdown-file-editing.spec.ts
cd ..
python3 scripts/lint-spec-files.py --all
```

## Risks

- Monaco internal tokenizer imports require an explicit compatibility boundary.
- Table source edits must preserve line endings and existing cell bytes.
- Table-local controls must survive upstream DOM rerenders without leaking a
  React portal or adding an overflow owner.

## Parallelism

`sequential`

## Results

- Connected the upstream incremental Monaco Monarch highlighter for TypeScript,
  JavaScript, CSS, HTML, Python, Rust, shell, and YAML code fences.
- Added undoable, source-preserving append-row and append-column controls with
  compact fine-pointer styling and 44-pixel coarse-pointer targets.
- Applied the two-pixel active-block radius and visible themed table borders in
  Preview and Edit.
- Passed 14 focused adapter tests, web typecheck, focused ESLint, i18n checks,
  specification lint, and the Vite production build.
- Passed production-build Playwright coverage: desktop Chromium 4/4 and
  mobile-chrome 1/1.
