---
id: "02-region-anchors"
title: "Region anchors and drift in the bridge"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-006
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-007
acceptance_criteria:
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-006.2
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-007.1
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-007.2
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-007.3
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback-05.md
---

# Task 02: Region anchors and drift in the bridge

## Summary

Record the content under each region at capture time, draw outlines from it
when the layout changes, report drift, and report the highlighted marker's
position for anchoring the floating editor.

## In scope

- Inspector: choose an anchor on drag end (smallest containing element from
  the region's center, no `html`, `body`, or inspector UI, unique selector)
  and add `anchor_selector`, `anchor_offset_x`, `anchor_offset_y`,
  `anchor_width`, and `anchor_height` to the capture rectangle.
- Inspector: `regionLayout` places anchored outlines from the anchor's
  bounding rectangle plus offset, subtracting container scroll only when the
  container is the anchor or its ancestor, and reports `drifted` by the rules
  in part 5.
- Inspector: `ResizeObserver` on `documentElement`, `body`, and resolved
  anchors, and a `MutationObserver` on the body subtree, coalesced into one
  layout per frame, active only while the current route has regions.
- Inspector: `anchor-region {request_id, rect}` answered by
  `region-anchored {request_id, anchor}`.
- Inspector: `region-layout` gains `highlighted: {id, rect} | null`.
- Parent: types and validators for the anchor fields, `drifted`,
  `highlighted`, and `region-anchored`; `requestRegionAnchor(iframe, rect)`
  resolving to the anchor or `null` after 500 ms.
- Parent: recapture requests the anchor while rasterizing and saves it in the
  new capture rectangle; `PreviewRegionView` gains `drifted`.

## Out of scope

- Drawing drift or the eye button (task 05).
- Using `highlighted` for the editor position (task 03).

## Acceptance conditions

1. Node-executed inspector tests cover anchor choice, following a moved
   anchor, ancestor-container scroll, every drift rule, `anchor-region`
   replies, relayout after resize and mutation, and `highlighted`.
2. Validators accept the new fields and messages and reject oversized
   selectors; a missing `drifted` reads as false.
3. A recapture saves the anchor it received, and saves no anchor when the
   request times out.

## Verification

```bash
cd apps/web
pnpm exec vitest run lib/preview-inspector-script.test.ts lib/preview-inspect-bridge.test.ts lib/preview-annotation.test.ts hooks/use-preview-screenshots.test.ts hooks/use-preview-capture.test.ts
cd ../backend && go test -trimpath ./internal/agentctl/server/api/...
```

## Likely files

- `apps/backend/internal/agentctl/server/api/scripts/inspector.js`
- `apps/web/lib/preview-inspect-bridge.ts`, `.test.ts`
- `apps/web/lib/preview-inspector-script.test.ts`
- `apps/web/lib/preview-annotation.ts`, `.test.ts`
- `apps/web/lib/types/http-agents.ts`
- `apps/web/hooks/use-preview-screenshots.ts`, `.test.ts`
- `apps/web/hooks/use-preview-inspector-events.ts`
- `apps/web/hooks/use-preview-capture.ts`

## Risks

- `inspector.js` is near its function-size limits; extract helpers rather than
  growing `regionLayout`.
- Observers must disconnect when regions leave the route or the bridge
  reloads, or a long session accumulates them.
