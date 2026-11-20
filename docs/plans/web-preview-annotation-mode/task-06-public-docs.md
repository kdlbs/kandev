---
id: "06-public-docs"
title: "Public docs"
status: done
wave: 4
depends_on:
  - "05-annotation-layer"
plan: "plan.md"
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-005
acceptance_criteria:
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-005.2
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-005.6
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback-03.md
---

# Task 06: Public docs

## Summary

Update the web preview feedback section of the developer tools page for
annotation mode, comment on selection, multiple regions, resizing, and Escape.

## In scope

- `docs/public/developer-tools.md`: replace the menu-based steps with the
  toolbar flow, and describe drafts, the screenshot limit, and scroll
  tracking limits (canvas scrolling).

## Out of scope

- Screenshots or videos.

## Acceptance conditions

1. The page describes the shipped behavior and passes the public-doc
   validators.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Likely files

- `docs/public/developer-tools.md`

## Risks

- None beyond keeping the page scan-friendly.
