---
id: "06-e2e-and-docs"
title: "E2E coverage and public docs"
status: done
wave: 3
depends_on: ["01-page-identity", "04-toolbar", "05-region-overlay"]
plan: "plan.md"
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-002
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-005
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-006
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-007
acceptance_criteria:
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-002.5
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-005.2
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-005.3
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-006.3
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-007.1
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-007.3
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-007.4
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback-05.md
---

# Task 06: E2E coverage and public docs

## Summary

Cover the round's flows in Playwright, update older specs that relied on the
inline panel form, and update the public developer-tools page.

## In scope

- Desktop Chromium: reopen an HTML preview and see the outline and number
  return, then a new capture numbered 2; open the panel and see a region stay
  on its content; reflow the fixture at a narrower width and see the drift
  badge, then move the region and see it cleared; show the saved capture;
  comment with the panel open in the floating box; switch from Text to Region
  with a comment open and draw a region at once; drag the toolbar and find it
  in place after a reload.
- Mobile Chrome: the eye button toggles on tap; the bottom bar is unchanged.
- Update `preview-comment-panel.spec.ts` and other specs that typed into the
  panel's inline form.
- `docs/public/developer-tools.md`: floating comments, moving the toolbar,
  switching tools, drift warning, and the eye button.
- Mark part 5 `current`, the plan `implemented`, and record deviations.

## Out of scope

- New fixture servers beyond extending the existing preview fixture with a
  width-dependent layout.

## Acceptance conditions

1. The new and updated preview specs pass on `chromium` and `mobile-chrome`.
2. Public docs validators pass.

## ASCII UI preview

Plan UI-01 to UI-05.

## Verification

```bash
cd apps/web
pnpm e2e:run --host --shards 1 --project chromium tests/preview tests/chat/html-preview.spec.ts tests/chat/question-image-review.spec.ts
pnpm e2e:run --host --shards 1 --project mobile-chrome tests/preview tests/task/mobile-html-preview.spec.ts
cd ../.. && node scripts/validate-public-docs.mjs && python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all
```

## Likely files

- `apps/web/e2e/tests/preview/preview-comment-panel.spec.ts`
- `apps/web/e2e/tests/preview/preview-annotation-alignment.spec.ts` (new)
- `apps/web/e2e/tests/preview/preview-feedback-helpers.ts`
- `apps/web/e2e/tests/chat/html-preview.spec.ts`
- `docs/public/developer-tools.md`

## Risks

- The E2E preview is narrow, so the panel overlays it; flows that click under
  the panel keep using `startWithCommentPanelHidden`.
