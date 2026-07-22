---
id: "07-toolbar-completeness"
title: "Toolbar completeness"
status: done
wave: remediation
depends_on: ["02-toolbar-wiring", "06-resolver-correctness"]
plan: "plan.md"
spec: "../../specs/ui/external-vcs-file-links.md"
---

# Task 07: Toolbar completeness

## Acceptance

- CodeMirror editor, Monaco diff, and desktop image/binary viewer toolbars expose the shared action when context is valid.
- Existing Pierre, Monaco editor, Review, and mobile coverage remains intact.
- Focused tests cover each newly wired provider family and omission behavior.
- Changed TypeScript files remain within the repository's 600-line file limit; extract the Review file header/context cohesively.

## Output contract

Report RED/GREEN tests, changed files, typecheck/lint results, and any surface intentionally excluded by the spec.

## Completion report

- **Summary:** Completed the missing CodeMirror editor, Monaco diff, and desktop image/binary toolbar integrations, while extracting Review header/context to keep changed TypeScript within the repository file-size limit.
- **Changed scope:** CodeMirror and Monaco editor/diff toolbar components and tests; desktop file viewer header/image/binary integrations and tests; Review header/context extraction and focused coverage.
- **Focused verification:** `(cd apps && pnpm --filter @kandev/web test -- components/editors/codemirror/codemirror-code-editor.external-link.test.tsx components/editors/monaco/diff-viewer-toolbar.test.tsx components/task/file-editor-panel.image.test.tsx components/task/file-tab-content.external-link.test.tsx components/review/review-diff-header.test.tsx)` passed; `(cd apps/web && pnpm run typecheck)` and targeted ESLint passed.
- **Intentional exclusion:** Mobile nested desktop-style file headers do not render a duplicate action. `MobileFileViewerPanel` remains the single touch-sized mobile entry point, preserving its 44px control and avoiding competing nested headers.
