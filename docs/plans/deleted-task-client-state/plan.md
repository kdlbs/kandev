---
created: 2026-10-07
status: implemented
requirements:
  - REQ-UI-DELETED-TASK-CLIENT-STATE-001
system_design:
  - ../../specs/ui/system-design/deleted-task-client-state.md
legacy_specs: []
---

# Implementation Plan: Deleted Task Client State

## Overview

Release the store state of a deleted task's sessions when `task.deleted`
arrives. One work order makes the handler change and adds a regression test.
Context: [kdlbs/kandev#4100](https://github.com/kdlbs/kandev/issues/4100).

## Work orders

- [Task 01: Remove deleted task sessions](task-01-remove-deleted-task-sessions.md)

## Risks

- A later handler step that reads the removed session maps would see them gone.
  The current steps after the loop read task identity only.

## Verification

`pnpm --filter @kandev/web exec vitest run lib/ws/handlers/
lib/state/slices/session/remove-task-session.test.ts` from `apps/`.
