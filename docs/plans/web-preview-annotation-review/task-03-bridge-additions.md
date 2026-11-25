---
id: "03-bridge-additions"
title: "Bridge additions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-006
acceptance_criteria:
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-006.2
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-006.4
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback-04.md
---

# Task 03: Bridge additions

## Summary

Add the additive protocol 3 messages from part 4: clickable marker badges in
the mode, page clicks as outside clicks for an open comment, parent-assigned
numbers, and a highlighted marker.

## In scope

- Inspector: badges `pointer-events: auto` with a pointer cursor only while
  annotation is active; click sends `marker-activated {id}` and stops the
  event.
- Inspector: `set-annotation-active` accepts `surface_open`; while true, a
  page `pointerdown` outside inspector UI sends `page-pointerdown {}`.
- Inspector: markers and regions render `number` from the payload, falling
  back to list position; `highlighted_id` scrolls its marker into view and
  draws a thicker ring.
- Parent: validators for the two new events, `sendSetAnnotationActive` with
  `surface_open`, and `sendProjectPreviewMarkers` with numbers and
  `highlighted_id`.

## Out of scope

- Acting on the events (task 04).

## Acceptance conditions

1. Node-executed inspector tests cover badge clickability in and out of the
   mode, `page-pointerdown` gating, and rendered numbers.
2. Validators accept the new events and reject malformed payloads.

## Verification

```bash
cd apps/web
pnpm exec vitest run lib/preview-inspector-script.test.ts lib/preview-inspect-bridge.test.ts
cd ../backend && go test ./internal/agentctl/server/api/...
```

## Likely files

- `apps/backend/internal/agentctl/server/api/scripts/inspector.js`
- `apps/web/lib/preview-inspect-bridge.ts`, `.test.ts`
- `apps/web/lib/preview-inspector-script.test.ts`

## Risks

- The page can stop propagation of `pointerdown`; listen in the capture
  phase, as the existing listeners do.
