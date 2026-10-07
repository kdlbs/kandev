---
created: 2026-10-07
status: implemented
requirements:
  - REQ-UI-HIDDEN-RUNNING-BACKFILL-001
system_design:
  - ../../specs/ui/system-design/hidden-running-backfill.md
legacy_specs: []
---

# Implementation Plan: Hidden Running Backfill

## Overview

Skip the periodic running-session message read while the document is hidden.
One work order adds the gate and hook tests for both visibility states.
Context: [kdlbs/kandev#4100](https://github.com/kdlbs/kandev/issues/4100).

## Work orders

- [Task 01: Gate running backfill on visibility](task-01-gate-running-backfill.md)

## Risks

- Desktop WebKit does not report a hidden document when the window is only
  unfocused or covered, so the gate applies when the window is minimized or on
  another Space.

## Verification

`pnpm --filter @kandev/web exec vitest run hooks/domains/session/` from `apps/`,
and the `tests/chat/hidden-running-backfill.spec.ts` Playwright spec, which
counts `message.list` frames for a running session while the page is visible,
hidden, and visible again.
