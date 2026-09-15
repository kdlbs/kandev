---
id: "03-capture-text-and-elements"
title: "Capture text and elements across previews"
status: pending
wave: 3
depends_on:
  - "02-deliver-feedback-through-chat"
plan: "plan.md"
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-002
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-004
acceptance_criteria:
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-002.1
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-002.2
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-002.4
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-002.5
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-002.6
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-002.7
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-004.1
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-004.2
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-004.3
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-004.4
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-004.5
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback.md
---

# Task 03: Capture Text and Elements Across Previews

## Summary

Turn the injected inspector into a validated selection bridge and add one shared
desktop and phone review UI to the Browser panel and native HTML-file preview.
Users can save commented text and element captures, navigate across pages, and
restore markers without tying feedback to a mounted iframe.

## In scope

- Versioned parent/iframe commands and events with source, type, byte, route,
  and selector validation.
- Text selection and DOM element capture by mouse, touch, and keyboard,
  including frozen DOM range endpoints, containing-element HTML, selection
  rectangles, scroll position, and viewport metrics.
- A preselection outline, translucent fill, and concise tag/ID/class label that
  follows pointer hover, keyboard focus, or an active touch press.
- Bounded accessible metadata and escaped `outerHTML` snapshots.
- Browser localhost proxy eligibility plus already-proxied HTML-preview
  eligibility.
- Task-backed marker projection for a matching source and route, including
  unavailable-anchor state without mutation.
- Shared pending list, desktop menu/Popover/panel, phone Drawer, edit/delete,
  clear confirmation, mutation progress, and failure recovery.
- Localized copy in all five locales and minimum touch/overflow behavior.
- Removal of Browser-panel-local annotation ownership, Copy/Clear-only state,
  and obsolete local hook assumptions.

## Out of scope

- Screenshot rasterization and image upload.
- Source-line mapping or execution-state persistence.
- Sending directly from preview chrome.

## Acceptance

- Browser and native HTML preview create the same task-owned text and element
  payloads and retain prior-route items while navigation continues normally.
- A script-generated text selection remains fully represented by its saved
  rendered element, DOM range, and visual position after the live page changes.
- Element mode identifies the candidate before activation and removes every
  hint overlay when capture ends or the document changes.
- Parent code ignores events from any other window or invalid schema, and page
  HTML is always rendered as text in Kandev chrome.
- Desktop and phone offer equivalent capture, edit, delete, and review outcomes
  with viewport-native controls and recoverable drafts.

## ASCII UI preview

Implement [UI-01](plan.md#ui-01-desktop-preview-annotation) and
[UI-02](plan.md#ui-02-phone-preview-annotation). The phone action opens a Drawer
instead of shrinking the desktop pending panel.

```text
Desktop: [Refresh] [Annotate (3) ▾]  -> Text | Element | Screenshot | Review
Phone:   [Show code] [Annotate 3]     -> full-width bottom Drawer

Pending feedback
/products · Text “Choose a plan” · “Increase contrast”              [Edit]
/products · <button.checkout> · “Align with price”                  [Edit]

Element mode:  ┌──── button#save.primary ────┐
               │            Save             │  updates before selection
               └─────────────────────────────┘
```

## TDD sequence

1. Add failing Go injection and TypeScript bridge tests for the protocol,
   source validation, generated-text DOM ranges and coordinates, element
   metadata, candidate hint lifecycle, route changes, and marker projection;
   run both suites RED.
2. Implement the minimum bridge and shared hook until protocol tests pass.
3. Add failing rendered tests for Browser and HTML-preview entry, desktop
   Popover/panel, phone Drawer, draft preservation, keyboard focus, 44-pixel
   controls, and no overflow; run RED.
4. Implement the responsive components and localized copy, rerun GREEN, then
   remove obsolete local annotation state and refactor shared surface adapters.

## Verification

```bash
cd apps/backend
go test -race ./internal/agentctl/server/api
cd ../../apps
pnpm --filter @kandev/web test -- --run lib/preview-inspect-bridge.test.ts hooks/use-preview-capture.test.ts components/task/browser-panel.test.tsx components/task/html-preview-content.test.tsx components/task/inspector components/task/mobile/mobile-preview-feedback.test.tsx
cd web
pnpm run typecheck
pnpm run i18n:check
pnpm run i18n:ratchet
pnpm exec eslint lib/preview-inspect-bridge.ts hooks/use-preview-capture.ts components/task/browser-panel.tsx components/task/html-preview-content.tsx components/task/inspector components/task/mobile/mobile-preview-feedback.tsx
```

## Files likely touched

- `apps/backend/internal/agentctl/server/api/scripts/inspector.js`
- `apps/backend/internal/agentctl/server/api/html_injector_test.go`
- `apps/web/lib/preview-inspect-bridge*.ts`
- `apps/web/lib/preview-url-detector*.ts`
- `apps/web/hooks/use-preview-capture*.ts`
- `apps/web/components/task/browser-panel*.tsx`
- `apps/web/components/task/html-preview-content*.tsx`
- `apps/web/components/task/inspector/`
- `apps/web/components/task/mobile/mobile-preview-feedback*.tsx`
- `apps/web/components/task/mobile/mobile-file-viewer-panel*.tsx`
- `apps/web/src/locales/*/task.json`

## Dependencies

- Task 01 supplies synchronized CRUD and screenshot-neutral pending rows.
- Task 02 ensures data created by this UI can be delivered through normal Send.

## Risks

- SPA navigation and iframe reloads can leave bridge listeners or markers bound
  to the previous document unless teardown is identity-scoped.
- Element events inside interactive controls must capture only in explicit mode
  and restore normal click and touch behavior immediately afterward.
- Native HTML preview must not pass an already-proxied URL through proxy
  rewriting again.

## Parallelism

`sequential`

## Inputs

- Capture bridge, capture payloads, marker projection, responsive interaction,
  accessibility, and security sections in the system design.
- Existing Browser inspector submission and native HTML preview tests.

## Results

Pending implementation.
