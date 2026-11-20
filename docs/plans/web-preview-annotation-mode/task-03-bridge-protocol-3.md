---
id: "03-bridge-protocol-3"
title: "Inspector bridge protocol 3"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-005
acceptance_criteria:
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-005.1
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-005.3
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-005.5
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-005.7
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback-03.md
---

# Task 03: Inspector bridge protocol 3

## Summary

Update the injected inspector and the parent validators to protocol 3:
persistent modes, routed Escape, selection reporting and capture, region
anchors with scroll-container tracking, and region layout events.

## In scope

- `inspector.js`: modes persist after a capture; same-document route changes
  keep the mode; `set-annotation-active`; Escape sends `escape` when Kandev
  owns it; `selectionchange` coalesced per frame into `selection-changed`;
  `capture-selection`; scroll-container detection on region drag start;
  region anchors in `project-markers`; `region-layout` per frame when changed; `pending`
  markers drawn dashed.
- `preview-inspect-bridge.ts`: version 3, validators for every new message,
  `isCaptureRect` accepting the three container fields with bounds.
- `PreviewCaptureRect` type gains the optional fields.

## Out of scope

- Parent UI and hook changes (task 04 and 05).

## Acceptance conditions

1. Validators accept every well-formed protocol 3 message and reject wrong
   versions, unknown types, and oversized payloads.
2. In a DOM test, the script keeps its mode after a capture, routes Escape by
   annotation state, reports selection changes, and reports a region's
   rectangle moving with a scrolled inner container.

## Verification

```bash
cd apps/backend
go test -tags fts5 ./internal/agentctl/server/api/ -run 'Inspector|HTML' -count=1
cd ../web
pnpm exec vitest run lib/preview-inspect-bridge.test.ts lib/preview-inspector-script.test.ts
pnpm run typecheck && pnpm run lint
```

## Likely files

- `apps/backend/internal/agentctl/server/api/scripts/inspector.js`, `html_injector_test.go`
- `apps/web/lib/preview-inspect-bridge.ts`, `.test.ts`
- New `apps/web/lib/preview-inspector-script.test.ts` (jsdom, loads `inspector.js`)
- `apps/web/lib/types/http-agents.ts`

## Risks

- jsdom has no layout. Stub `getBoundingClientRect` and scroll offsets in the
  script test, and leave real geometry to E2E in task 05.
