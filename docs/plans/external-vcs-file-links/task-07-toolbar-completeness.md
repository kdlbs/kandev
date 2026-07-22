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
