---
id: "04-draft-lifecycle"
title: "Close reducer, numbering, markers, pausing"
status: done
wave: 2
depends_on:
  - "01-capture-reliability"
  - "03-bridge-additions"
plan: "plan.md"
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-005
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-006
acceptance_criteria:
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-005.4
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-005.5
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-006.2
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-006.4
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-006.6
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback-04.md
---

# Task 04: Close reducer, numbering, markers, pausing

## Summary

Wire the preview state to part 4: one close reducer for X, Escape, and outside
clicks; one number per capture shared by markers, outlines, and the list;
markers that open their comments; and a paused tool while a text or element
comment is open.

## In scope

- Close reducer: empty new capture discards (PNG, upload, marker), typed new
  capture stays unsaved, saved item closes. Escape step 1 and
  `page-pointerdown` use it.
- Numbering helper per route (saved items in collection order, then drafts),
  sent through `project-markers` and exposed for the list.
- `marker-activated` and the `RegionOverlay` badge `onSelect` open the item or
  draft.
- Pausing: text and element comments send mode `null` while annotation stays
  active, project a highlighted pending marker, and restore the tool on close.
- `surface_open` sent while a floating comment is open.
- Pass `onCancel` to the floating preview surfaces.

## Out of scope

- The panel (task 05).

## Acceptance conditions

1. Reducer tests cover empty, typed, and saved closes from each trigger.
2. Numbers match between projected markers, regions, and the list helper.
3. A text or element comment pauses the tool and restores it on close.

## Verification

```bash
cd apps/web
pnpm exec vitest run hooks/use-preview-capture.test.ts hooks/use-preview-drafts.test.ts hooks/use-preview-annotation-mode.test.ts lib/preview-annotation.test.ts components/task/inspector
pnpm run typecheck && pnpm run lint
```

## Likely files

- `apps/web/hooks/use-preview-capture.ts`, `use-preview-drafts.ts`,
  `use-preview-annotation-mode.ts`, `use-preview-inspector-events.ts`
- `apps/web/lib/preview-annotation.ts` (reducer, numbering)
- `apps/web/components/task/inspector/preview-annotation-surfaces.tsx`,
  `preview-annotation-layer.tsx`

## Risks

- `use-preview-capture.ts` is close to the file limit; put the reducer and
  numbering in `lib/preview-annotation.ts`.
