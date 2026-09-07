---
id: "04-responsive-html-preview-e2e"
title: "Prove responsive HTML preview flows"
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
  - AC-UI-NATIVE-HTML-PREVIEW-001.6
  - AC-UI-NATIVE-HTML-PREVIEW-001.7
  - AC-UI-NATIVE-HTML-PREVIEW-001.8
system_design:
  - ../../specs/ui/system-design/native-html-preview.md
---
# Task 04: Prove responsive HTML preview flows

## Summary

This work order proved the static-only browser flow and asserted that scripts
did not execute. Those assertions contradict the approved script-capable
contract and are superseded by Task 09.

## In scope

- Existing desktop and mobile fixtures may be reused after the runtime-backed
  flow is implemented.

## Out of scope

- Treating a passing static-only browser test as evidence for script execution
  or network isolation.

## Acceptance

- Cancelled. The old test proves the wrong behavior.

## Verification

Not applicable. Use [Task 09](task-09-script-capable-preview-e2e.md).

## Files likely touched

- `apps/web/e2e/tests/chat/html-preview.spec.ts`
- `apps/web/e2e/tests/task/mobile-html-preview.spec.ts`

## Dependencies

Replaced by Task 09 after Tasks 06 and 07.

## Risks

Negative-only tests can pass while the required inline script capability is
missing.

## Parallelism

`sequential`

## Inputs

- The superseded static browser scenarios.
- The runtime-backed verification strategy in the system design.

## Results

Cancelled after review. Its previous results remain historical only.
