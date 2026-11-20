---
id: "05-public-docs"
title: "Public docs for preview feedback changes"
status: done
wave: 3
depends_on:
  - "02-canvas-capture-readability"
  - "04-preview-comment-surface"
plan: "plan.md"
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-002
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-004
acceptance_criteria:
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-002.8
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-002.9
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-004.6
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-004.7
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback.md
  - ../../specs/tasks/system-design/web-preview-feedback-02.md
---

# Task 05: Public docs for preview feedback changes

## Summary

Update `docs/public/developer-tools.md` (the web preview feedback section)
using `/docs-maintainer`: the comment appears next to the capture, dismissed
drafts can be resumed, WebGL capture is supported, and the uniform-canvas
warning is explained. Add a Flutter tip: element feedback needs
`SemanticsBinding.instance.ensureSemantics()` to anchor on `flt-semantics`
nodes. CanvasKit and `--wasm` builds both capture in previews. Worker-rendered
canvases on cross-origin-isolated pages are not supported.

## In scope

- Prose and, if present, screenshot references in `docs/public/developer-tools.md`.

## Out of scope

- New pages and changes to the specs.

## Implementation acceptance conditions

1. The docs describe the new flow and its limits, with no em dashes, and
   follow `docs/public/README.md`.

## Verification

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Likely files

- `docs/public/developer-tools.md`

## Results

Done. `docs/public/developer-tools.md` (how-to section) describes the shared
comment form, draft resume, WebGL support and its limits, the uniform-canvas
warning, and the Flutter semantics tip. `node scripts/validate-public-docs.mjs`,
`python3 scripts/list-docs.py validate`, and
`python3 scripts/lint-spec-files.py --all` pass.
