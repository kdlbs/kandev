---
id: "05-region-overlay"
title: "Drift warning and saved-capture view on regions"
status: done
wave: 2
depends_on: ["02-region-anchors"]
plan: "plan.md"
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-007
acceptance_criteria:
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-007.3
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-007.4
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback-05.md
---

# Task 05: Drift warning and saved-capture view on regions

## Summary

Draw drifted regions as a warning with an explanatory tooltip, and let the
user show a region's saved capture over the live page.

## In scope

- `RegionOverlay`: optional `driftedIndexes`, per-item `imageUrl`, and
  `labels` (drift tooltip, Show capture, Hide capture) from the host.
- Drifted outline: dashed amber, a focusable warning badge with a tooltip on
  fine pointers and a tap-opened popover on coarse pointers.
- Eye button with `aria-pressed`: hover or focus reveals it on fine pointers
  (hold to show, click to toggle); always visible at 44 pixels on coarse
  pointers (tap toggles). The image fills the outline with `object-contain`
  and hides when a move or resize starts.
- The preview layer passes drifted indexes, image URLs (attachment URL or
  draft object URL), and translated labels. Locale keys in seven locales and
  pseudo.

## Out of scope

- Computing drift (task 02).

## Acceptance conditions

1. A drifted region renders the warning style and badge with the tooltip
   text; a non-drifted region does not.
2. The eye button toggles the image and its pressed state, and appears only
   for regions with an image.
3. Question-review annotations render unchanged without the new props.

## ASCII UI preview

Plan UI-03 (drift), UI-04 (saved capture), and UI-05 (phone).

## Verification

```bash
cd apps/web
pnpm exec vitest run components/shared/region-annotation components/task/inspector/preview-annotation-layer.test.tsx components/task/chat/question-review
pnpm run i18n:check
```

## Likely files

- `apps/web/components/shared/region-annotation/region-overlay.tsx`, `.test.tsx`
- `apps/web/components/task/inspector/preview-annotation-layer.tsx`
- `apps/web/hooks/use-preview-capture.ts`
- `apps/web/src/locales/*/task.json`

## Risks

- `region-overlay.tsx` is shared and near the component size guideline;
  split the badge and eye button into their own components.
