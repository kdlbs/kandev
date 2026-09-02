---
id: "01-hybrid-markdown-adapter"
title: "Add the Source-Preserving Markdown Adapter"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-MARKDOWN-FILE-EDITING-001
  - REQ-UI-MARKDOWN-FILE-EDITING-002
acceptance_criteria:
  - AC-UI-MARKDOWN-FILE-EDITING-001.2
  - AC-UI-MARKDOWN-FILE-EDITING-001.3
  - AC-UI-MARKDOWN-FILE-EDITING-001.6
  - AC-UI-MARKDOWN-FILE-EDITING-002.1
  - AC-UI-MARKDOWN-FILE-EDITING-002.6
  - AC-UI-MARKDOWN-FILE-EDITING-002.7
  - AC-UI-MARKDOWN-FILE-EDITING-002.8
system_design:
  - ../../specs/ui/system-design/markdown-file-editing.md
---

# Task 01: Add the Source-Preserving Markdown Adapter

## Summary

Add the experimental browser editor behind one Kandev-owned adapter. Prove
source fidelity, lifecycle safety, and host callback boundaries before any file
surface depends on it.

## In scope

- Exact dependency pin and lockfile update.
- Model, view, controller, history, selection, and disposal lifecycle.
- Source edit and authoritative replacement translation.
- Link, comment, baseline, gutter, and failure callback contracts.
- Exact-string source corpus, security cases, and adapter component tests.

## Out of scope

- File toolbar or open-tab state changes.
- Mobile layout changes.
- Backend file operations.

## Acceptance

- One adapter contains every upstream package import and preserves untouched
  bytes across render-only lifecycle events.
- Supported edits update Kandev's canonical string. Unsupported syntax remains
  editable source without unsafe HTML or URL activation.
- Initialization and update failures preserve content, dispose failed state,
  and emit the Source fallback callback.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
pnpm --filter @kandev/web test -- --run \
  components/editors/markdown/hybrid-markdown-editor.test.tsx \
  components/editors/markdown/markdown-source-preservation.test.ts
pnpm --filter @kandev/web typecheck
pnpm --filter @kandev/web lint
```

## Files likely touched

- `apps/web/package.json`
- `apps/pnpm-lock.yaml`
- `apps/web/components/editors/markdown/hybrid-markdown-editor.tsx`
- `apps/web/components/editors/markdown/hybrid-markdown-editor.test.tsx`
- `apps/web/components/editors/markdown/markdown-source-preservation.test.ts`
- `apps/web/components/editors/markdown/fixtures/`

## Dependencies

None.

## Risks

- Upstream pre-1.0 types and behavior can change between releases.
- Renderer lifecycle callbacks can create an edit echo or change whitespace.
- Upstream DOM rendering can bypass Kandev security if host callbacks are loose.

## Parallelism

`sequential`

## Inputs

- Requirement sections `REQ-UI-MARKDOWN-FILE-EDITING-001` and `002`.
- System design sections `Hybrid editor adapter`, `Data and contracts`, and
  `Security`.
- `docs/decisions/2026-08-27-source-preserving-hybrid-markdown-engine.md`.
- Existing `MarkdownPreviewContent` safety and comment tests.

## Results

- Added the exact `@vscode/markdown-editor@0.0.2-84` dependency and lockfile entry.
- Added the source-preserving replacement helper and the Kandev-owned upstream adapter.
- Covered source corpus fidelity, lifecycle disposal, link/baseline/gutter/comment wiring,
  host replacement, and Source fallback behavior with focused tests.
- Focused tests, web typecheck, and adapter lint pass.
