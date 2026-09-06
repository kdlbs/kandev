---
id: "02-responsive-html-preview"
title: "Add responsive HTML preview surfaces"
status: cancelled
wave: 2
depends_on:
  - "01-preview-state-and-sandbox"
plan: "plan.md"
requirements:
  - REQ-UI-NATIVE-HTML-PREVIEW-001
acceptance_criteria:
  - AC-UI-NATIVE-HTML-PREVIEW-001.1
  - AC-UI-NATIVE-HTML-PREVIEW-001.2
  - AC-UI-NATIVE-HTML-PREVIEW-001.3
  - AC-UI-NATIVE-HTML-PREVIEW-001.4
  - AC-UI-NATIVE-HTML-PREVIEW-001.5
  - AC-UI-NATIVE-HTML-PREVIEW-001.6
  - AC-UI-NATIVE-HTML-PREVIEW-001.8
  - AC-UI-NATIVE-HTML-PREVIEW-001.10
system_design:
  - ../../specs/ui/system-design/native-html-preview.md
---
# Task 02: Add responsive HTML preview surfaces

## Summary

This work order wired the HTML preview to a static `srcDoc` iframe. The owner
selected script-capable execution, so the implementation is superseded by the
runtime-backed renderer work in Tasks 06 and 07.

## In scope

- Existing toolbar, file-state, localization, desktop, and mobile wiring may be
  reused after the runtime-backed renderer contract is implemented.

## Out of scope

- Shipping `HtmlPreviewContent` with `sandbox=""` and inert scripts as the
  final behavior.
- Treating component tests for iframe attributes as proof of script isolation.

## Acceptance

- Cancelled. The original acceptance conditions describe the static-only
  renderer and do not satisfy inline JavaScript execution.

## Verification

Not applicable. Use [Task 06](task-06-preview-state-and-renderer.md) and
[Task 07](task-07-responsive-preview-surfaces.md) for the replacement work.

## Files likely touched

- `apps/web/components/task/html-preview-content.tsx`
- `apps/web/components/task/file-editor-content.tsx`
- `apps/web/components/task/mobile/mobile-file-viewer-panel.tsx`

## Dependencies

Replaced by Tasks 06 and 07.

## Risks

Keeping the old iframe contract in a passing component test can hide a missing
runtime boundary.

## Parallelism

`sequential`

## Inputs

- The superseded static prototype and existing Markdown preview surfaces.
- The revised runtime and renderer system design.

## Results

Cancelled after review. Existing UI changes remain non-ready until the
script-capable renderer is implemented.
