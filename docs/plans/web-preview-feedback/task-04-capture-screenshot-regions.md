---
id: "04-capture-screenshot-regions"
title: "Capture screenshot regions as attachments"
status: pending
wave: 4
depends_on:
  - "03-capture-text-and-elements"
plan: "plan.md"
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-002
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-003
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-004
acceptance_criteria:
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-002.3
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-002.4
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-003.4
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-003.5
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-004.3
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-004.4
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback.md
---

# Task 04: Capture Screenshot Regions as Attachments

## Summary

Add bounded screenshot-region rasterization, confirmation, upload, task claim,
preview, and message transfer to the shared capture workflow.

## In scope

- The maintained `html2canvas` dependency, lockfile, and generated license data.
- CSS-pixel rectangle normalization, iframe scroll coordinates, scale/downsize,
  16-megapixel and 10-MiB limits, PNG encoding, and capture cleanup.
- Desktop pointer and phone touch-region overlays that own gestures only while
  screenshot mode is active.
- Thumbnail, dimensions, accessible description, required comment, retry,
  cancel, upload progress, and delete behavior.
- Staged upload followed by task-feedback claim, PNG signature and metadata
  validation, orphan cleanup, and atomic message/queue claim transfer.
- Unit and integration tests for renderer, attachment lifecycle, all failure
  boundaries, and mixed ordinary/screenshot attachment limits.

## Out of scope

- Full-page stitching, video, system-window capture, or editing image pixels.
- Hiding known raster limitations from the user.
- A second screenshot storage path.

## Acceptance

- A completed region produces a reviewable PNG with the selected bounds before
  any durable feedback is created.
- Raster, encode, upload, create, delete, or Send failure never leaves an
  incomplete feedback row or inaccessible attachment and preserves retryable
  user input where applicable.
- Accepted direct and queued messages expose the screenshot as a normal image
  attachment and consume the exact pending row once.

## ASCII UI preview

Implement [UI-04 in the plan](plan.md#ui-04-screenshot-draft-and-recoverable-failure).
The thumbnail confirmation and visible failure state are required.

```text
┌ Screenshot feedback ────────────────────────┐
│ [ captured region thumbnail              ] │
│ 820 × 360 · PNG                             │
│ Comment *  [ Form clips here...           ] │
│ Could not upload the image. Try again.      │
│                       [Cancel] [Retry/Add]   │
└─────────────────────────────────────────────┘
```

## TDD sequence

1. Add failing pure raster-helper tests for rectangle normalization, scroll
   coordinates, scaling, pixel/byte bounds, and empty or failed canvases; run
   them RED.
2. Add failing attachment-service/repository tests for PNG validation, task
   claims, deletion, retry, transfer, limits, and cleanup; run them RED.
3. Implement the smallest renderer and attachment lifecycle that make those
   tests GREEN.
4. Add failing desktop and phone draft-component tests for gesture ownership,
   thumbnail confirmation, progress, retry, cancel, and focus; implement and
   rerun GREEN, then refactor with the complete focused set still passing.

## Verification

```bash
cd apps/backend
go test -race ./internal/task/repository/sqlite ./internal/task/service ./internal/task/handlers ./internal/orchestrator/messagequeue
cd ../../apps
pnpm --filter @kandev/web test -- --run lib/preview-screenshot.test.ts hooks/use-preview-capture.test.ts components/task/inspector/preview-feedback-draft.test.tsx components/task/mobile/mobile-preview-feedback.test.tsx hooks/use-message-handler.test.ts
cd web
pnpm run typecheck
pnpm run i18n:check
pnpm run i18n:ratchet
pnpm run licenses:gen
pnpm exec eslint lib/preview-screenshot.ts hooks/use-preview-capture.ts components/task/inspector components/task/mobile/mobile-preview-feedback.tsx
```

## Files likely touched

- `apps/package.json`
- `apps/pnpm-lock.yaml`
- `apps/web/package.json`
- `apps/web/generated/licenses.json`
- `apps/web/lib/preview-screenshot*.ts`
- `apps/web/hooks/use-preview-capture*.ts`
- `apps/web/components/task/inspector/preview-feedback-draft*.tsx`
- `apps/web/components/task/mobile/mobile-preview-feedback*.tsx`
- `apps/backend/internal/task/service/attachment_service*.go`
- `apps/backend/internal/task/repository/sqlite/attachment*.go`
- Preview-feedback repository and admission tests from Tasks 01 and 02.

## Dependencies

- Task 03 supplies region selection, shared capture UI, and Browser/HTML iframe
  adapters.
- Tasks 01 and 02 supply task claims and atomic direct/queue transfer.

## Risks

- Cross-origin and unsupported rendered content can produce an incomplete DOM
  raster. The user must review the actual thumbnail before Add.
- High-DPI pages can allocate oversized canvases before encoding. Pixel bounds
  and scaling must be computed before rendering.
- Cancelling an upload or closing a draft must distinguish staged bytes from an
  already committed task claim.

## Parallelism

`sequential`

## Inputs

- Screenshot regions, limits, atomic acceptance, lifecycle, failure, and
  security sections in the system design.
- Existing file-backed attachment service and composer preview components.

## Results

Pending implementation.
