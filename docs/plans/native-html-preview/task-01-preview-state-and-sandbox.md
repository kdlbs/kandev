---
id: "01-preview-state-and-sandbox"
title: "Establish preview state and sandbox contract"
status: cancelled
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-NATIVE-HTML-PREVIEW-001
acceptance_criteria:
  - AC-UI-NATIVE-HTML-PREVIEW-001.3
  - AC-UI-NATIVE-HTML-PREVIEW-001.4
  - AC-UI-NATIVE-HTML-PREVIEW-001.5
  - AC-UI-NATIVE-HTML-PREVIEW-001.6
  - AC-UI-NATIVE-HTML-PREVIEW-001.7
  - AC-UI-NATIVE-HTML-PREVIEW-001.10
system_design:
  - ../../specs/ui/system-design/native-html-preview.md
---
# Task 01: Establish preview state and sandbox contract

## Summary

This work order described the first static-only prototype. Its empty iframe
and inert-script contract was superseded when the owner reaffirmed the
inline-JavaScript requirement. It is retained as history and is not evidence
for the current plan.

## In scope

- The previously implemented format-neutral preview state and legacy Markdown
  restoration are reusable inputs for the revised work.
- The previous static document builder and navigation normalizer are reusable
  only as defense in depth.

## Out of scope

- Treating an empty sandbox or `script-src 'none'` as the final preview
  execution boundary.
- Shipping the static-only HTML preview contract.

## Acceptance

- Cancelled. The original acceptance conditions required scripts to remain
  inert and therefore conflict with the approved script-capable contract.

## Verification

Not applicable. Use [Task 05](task-05-script-capable-preview-runtime.md) and
[Task 06](task-06-preview-state-and-renderer.md) for the replacement work.

## Files likely touched

- `apps/web/lib/utils/file-types.ts`
- `apps/web/lib/html-preview/`
- `apps/web/hooks/use-file-editors.ts`

## Dependencies

Replaced by Tasks 05 and 06.

## Risks

Treating the old static tests as final evidence can conceal the missing inline
JavaScript capability.

## Parallelism

`sequential`

## Inputs

- The superseded static prototype commit and its test results.
- The revised requirements and system design.

## Results

Cancelled after review. No production contract is accepted from this work
order.
