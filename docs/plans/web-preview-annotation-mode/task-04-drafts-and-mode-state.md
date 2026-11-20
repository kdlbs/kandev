---
id: "04-drafts-and-mode-state"
title: "Draft list and annotation mode state"
status: done
wave: 2
depends_on:
  - "03-bridge-protocol-3"
plan: "plan.md"
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-005
acceptance_criteria:
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-005.3
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-005.4
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-005.5
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback-03.md
---

# Task 04: Draft list and annotation mode state

## Summary

Replace the single capture draft in `use-preview-capture` with an ordered
draft list, and add `usePreviewAnnotationMode` with the tool state and the
Escape reducer, wired to protocol 3.

## In scope

- Draft list: append and open on capture, dismiss keeps, resume, discard
  releases PNG and upload, save removes, one open at most, screenshot limit
  (unsaved plus saved at most 10).
- `usePreviewAnnotationMode`: `{ active, tool }`, last tool remembered,
  bridge mode mapping, `set-annotation-active`, re-sent on `inspector-ready`.
- `nextEscapeStep` pure reducer used by the bridge `escape` event and Kandev
  keydown.
- Selection state from `selection-changed` and `captureSelection()`.
- Region draft recapture API: `recaptureRegion(id, rect)` with rollback on
  failure, for drafts and, through task 01's update, saved items.

## Out of scope

- Rendering (task 05).

## Acceptance conditions

1. Hook tests cover the draft list rules, the screenshot limit, and that a
   capture keeps the tool active.
2. The Escape reducer follows the order in the design for every state.
3. A failed recapture restores the previous rectangle and image.

## Verification

```bash
cd apps/web
pnpm exec vitest run hooks/use-preview-capture.test.ts hooks/use-preview-annotation-mode.test.ts
pnpm run typecheck && pnpm run lint
```

## Likely files

- `apps/web/hooks/use-preview-capture.ts`, `.test.ts`
- New `apps/web/hooks/use-preview-annotation-mode.ts`, `.test.ts`

## Risks

- `use-preview-capture.ts` is near the 600-line limit. Split the draft list
  into `use-preview-drafts.ts`.
