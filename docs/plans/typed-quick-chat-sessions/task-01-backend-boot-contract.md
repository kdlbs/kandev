---
id: "01-backend-boot-contract"
title: "Backend boot contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/quick-chat-expiration.md"
---

# Task 01: Backend Boot Contract

## Acceptance

- Boot restores eligible ordinary and config sessions with metadata-derived `chat`/`config` kinds,
  hydrated primary sessions, and newest-activity-first order.
- Automation-run, workflow-bound, archived, missing-primary, and foreign-workspace tasks are absent.
- The modal hydrates closed with no active session and no schema change.

## Verification

`cd apps/backend && go test ./internal/backendapp -run 'TestBootPayloadRestoresQuickChatSessions'`

## Files Likely Touched

- `apps/backend/internal/backendapp/boot_state.go`
- `apps/backend/internal/backendapp/helpers_test.go`

## Inputs

- Spec: Data model, API surface, persistence scenarios.
- ADR-2026-07-14-typed-utility-chat-sessions metadata-derived typing and workspace isolation.
- Existing `quickChatSessions`, `listQuickChatTasks`, and boot harness patterns.

## Dependencies

None.

## Output Contract

Report behavior changed, files touched, red/green test commands, blockers, residual risks, and mark
this task `done` in this file and `plan.md`.
