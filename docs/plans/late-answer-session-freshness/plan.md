---
created: 2026-11-03
status: in_progress
requirements:
  - REQ-TASKS-CLARIFICATION-LIFECYCLE-001
system_design:
  - ../../specs/tasks/system-design/clarification-response-reliability.md
---

# Implementation Plan: Late-answer session freshness

## Overview

Preserve current source-session state while a late-answer action refreshes the
session before ordinary message admission. A delayed HTTP response must not
replace newer session state or live activity. A refresh failure keeps the
answer available for retry.

## Scope

- Refresh the source session before deriving direct-send or queue admission.
- Preserve newer timestamped session state and equal-timestamp live activity.
- Keep the existing send, queue, error, and retry behavior.
- Add deterministic coverage for session changes during and before refresh.

No endpoint, database, message format, or presentation contract changes.

## Work order

- [ ] [Task 01: Preserve source-session state during refresh](task-01-preserve-source-session-state.md)

## Verification strategy

- Run the focused late-clarification hook tests.
- Run targeted ESLint and Prettier checks on the changed web files.
- Require the current PR head's frontend, backend, E2E, architecture, and
  documentation coverage checks to pass.

This work changes admission correctness only. It has no rendered UI change, so
it needs no UI preview.
