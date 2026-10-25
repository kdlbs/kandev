---
id: "03"
title: "Executor route guards"
status: todo
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-003
  - REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-004
acceptance_criteria:
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.1
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.2
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.3
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.4
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.5
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.6
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.7
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-004.1
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-004.2
system_design:
  - ../../specs/executors/system-design/session-control-plane-isolation.md
---

# Task 03: Executor route guards

## Summary

Scope and interlock on executor mutations; remove WS mutations; record bind decision.

## Scope

- Route guards implementing the authorization matrix, route-enumeration test, WS registration removal.
- Profile routes: parent taken from `profile.ExecutorID`, a shared parent passes for every caller; a context without a snapshot skips the service comparison (settings-operations path unchanged).
- Owner snapshot carried from the middleware to the service write; mismatch answers not found and changes nothing (including owner-to-shared races).
- Web: move executor create, update and delete from the WebSocket request to the existing HTTP functions, with web unit tests.

## Exclusions

Bind default change.

## Acceptance

1. Mutations are refused without scope or interlock and accepted with both.
2. Settings pages still work, using HTTP for executor create, update and delete.

## Verification

`(cd apps/backend && go test ./internal/task/handlers/... -count=1)`
`cd apps/web && pnpm run typecheck && pnpm run i18n:check`

## Likely files

internal/task/handlers/executor_handlers.go, executor_profile_handlers.go, route_registration_test.go.
