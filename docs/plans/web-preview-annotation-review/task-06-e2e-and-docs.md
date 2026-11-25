---
id: "06-e2e-and-docs"
title: "E2E coverage and public docs"
status: done
wave: 4
depends_on:
  - "05-comment-panel"
plan: "plan.md"
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-005
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-006
acceptance_criteria:
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-006.1
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-006.2
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-006.3
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-006.4
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-006.5
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-006.6
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback-04.md
---

# Task 06: E2E coverage and public docs

## Summary

Cover the part 4 flows end to end on desktop and phone, update the existing
annotation specs for the panel, and update the public preview docs.

## In scope

- Desktop: busy outline then image; scroll during capture; X on an empty
  capture removes its outline; a typed close shows a dashed unsaved entry;
  hide the panel and check the iframe width; click a marker to edit; the
  toggle opens and closes; dragging an element comment keeps the highlight.
- Phone: the sheet starts hidden, opens from the toggle, and a region capture
  opens the drawer composer with Close.
- Update `preview-annotation-mode.spec.ts` and helpers for the panel.
- `docs/public/developer-tools.md`: the panel, closing captures, and markers.

## Out of scope

- New behavior.

## Acceptance conditions

1. The new and existing preview specs pass on `chromium` and
   `mobile-chrome`.
2. The public docs validators pass.

## Verification

```bash
cd apps/web
pnpm e2e:run --host --shards 1 --project chromium tests/preview tests/chat/html-preview.spec.ts tests/chat/question-image-review.spec.ts
pnpm e2e:run --host --shards 1 --project mobile-chrome tests/preview tests/task/mobile-html-preview.spec.ts
cd ../.. && node scripts/validate-public-docs.mjs
```

## Likely files

- `apps/web/e2e/tests/preview/preview-annotation-mode.spec.ts`
- New `apps/web/e2e/tests/preview/preview-comment-panel.spec.ts`
- `apps/web/e2e/helpers/` preview helpers
- `docs/public/developer-tools.md`

## Risks

- Busy-outline assertions race a fast capture. Hold rasterization with a
  routed slow image or assert the busy state through the draft attribute
  before the image arrives.
