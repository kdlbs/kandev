---
id: "06-positional-table-edits"
title: "Add Positional Markdown Table Edits"
status: completed
wave: 6
depends_on: ["05-polish-code-and-table-editing"]
plan: "plan.md"
requirements:
  - REQ-UI-MARKDOWN-FILE-EDITING-002
acceptance_criteria:
  - AC-UI-MARKDOWN-FILE-EDITING-002.10
  - AC-UI-MARKDOWN-FILE-EDITING-002.11
system_design:
  - ../../specs/ui/system-design/markdown-file-editing.md
---

# Task 06: Add Positional Markdown Table Edits

## Summary

Replace append-only table helpers with source-preserving insertion after any
visible row or column. Keep the Markdown delimiter row canonical but exclude it
from the visible row model.

## In scope

- Parse table rows without splitting escaped pipe characters.
- Insert a body row below the selected visible row, including directly below
  the header while leaving the delimiter in its required source position.
- Insert a column to the right of the selected column in the header, delimiter,
  and every body row.
- Preserve existing bytes, line endings, alignment markers, and outer-pipe
  style.

## Out of scope

- Rendering controls or resize handles.
- Deleting rows or columns, merging cells, or normalizing malformed tables.

## Acceptance

- Every visible row and column index produces one deterministic insertion.
- Existing cell bytes and line endings remain unchanged.
- The returned source selection targets the new cell and the edit remains
  suitable for one local-history record.

## Verification

```bash
cd apps/web
pnpm exec vitest run components/editors/markdown/markdown-table-edit.test.ts
pnpm run typecheck
```

## Files likely touched

- `apps/web/components/editors/markdown/markdown-table-edit.ts`
- `apps/web/components/editors/markdown/markdown-table-edit.test.ts`

## Dependencies

- Task 05 table helpers and local-history integration.

## Risks

- Escaped pipes and omitted outer pipes can make visual and source column
  indices diverge.
- Header-adjacent insertion must not separate the header from its delimiter.

## Parallelism

`sequential`

## Inputs

- `AC-UI-MARKDOWN-FILE-EDITING-002.10` and `.11`.
- Hybrid adapter table AST and current helper tests.

## Results

Verified on 2026-10-20 in the remediation worktree based on
`b03dce19114c4a63b8a6716894906dbc7f351c95`. Focused table and adapter tests,
TypeScript, and translation checks passed. Real desktop and phone browser
flows passed for row and column insertion, delimiter preservation, contained
table layout, and column resizing with exact saved source assertions.

[Remediation Task 06](../markdown-editing-remediation/task-06-engine-evidence.md)
records the complete commands and environment limitations. These results
replace the prior pending acceptance status. No push or merge was performed.
