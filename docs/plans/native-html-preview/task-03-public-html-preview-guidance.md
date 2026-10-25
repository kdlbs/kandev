---
id: "03-public-html-preview-guidance"
title: "Publish HTML preview guidance"
status: cancelled
wave: 3
depends_on:
  - "02-responsive-html-preview"
plan: "plan.md"
requirements:
  - REQ-UI-NATIVE-HTML-PREVIEW-001
acceptance_criteria:
  - AC-UI-NATIVE-HTML-PREVIEW-001.1
  - AC-UI-NATIVE-HTML-PREVIEW-001.2
  - AC-UI-NATIVE-HTML-PREVIEW-001.3
  - AC-UI-NATIVE-HTML-PREVIEW-001.4
  - AC-UI-NATIVE-HTML-PREVIEW-001.5
system_design:
  - ../../specs/ui/system-design/native-html-preview.md
---
# Task 03: Publish HTML preview guidance

## Summary

This work order documented the static-only preview prototype. Its public text
must not be treated as the final contract because the approved behavior now
includes isolated inline JavaScript.

## In scope

- The existing Files and editor guidance is an input to Task 08.

## Out of scope

- Publishing claims that scripts are inert as the final product behavior.
- Describing the current `srcDoc` iframe as a complete security boundary.

## Acceptance

- Cancelled. Public guidance will be rewritten after the runtime-backed
  behavior and its browser evidence are complete.

## Verification

Not applicable. Use [Task 08](task-08-script-capable-preview-guidance.md).

## Files likely touched

- `docs/public/developer-tools.md`

## Dependencies

Replaced by Task 08, after the runtime and user-facing surfaces are complete.

## Risks

Premature public guidance can promise either unsupported script execution or a
static-only behavior that the owner rejected.

## Parallelism

`sequential`

## Inputs

- The revised requirements and system design.
- The existing Files and editor integrations guide.

## Results

Cancelled after review. No public documentation result from this work order is
accepted for the revised contract.
