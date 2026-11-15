---
id: "01-settle-continuation-and-own-restore"
title: "Settle continued sessions and keep workspace recovery session-owned"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-003
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-005
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.7
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.9
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.24
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-003.1
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-005.1
system_design:
  - ../../specs/platform/system-design/interrupted-prompt-recovery.md
  - ../../specs/agents/system-design/harness-session-continuity.md
---

# Task 01: Settle continued sessions and keep workspace recovery session-owned

## Outcome

A committed context continuation retires its recovery notice and settles its
ancestry and attempts, and workspace restore uses only the restored session's
own execution.

## In scope

- SQLite context-continuation recovery retires the recovery notice and settles
  ancestry and attempts once the continuation commits; native-state fences are
  cleared before verification passes, and other creation reasons keep their
  state.
- `launchRestoreWorkspace` requires the restored session's own prepared
  execution; a shared environment no longer lends a sibling's execution.
- A launch carrying a held workspace-recovery claim coalesces by the claim's
  operation ID in `manager_execution.go`; callers without a claim keep the plain
  session key.
- Superseded predecessor attempts are settled, and a manual route-action
  recovery claim stays fenced while retrying.
- Cold native recovery preserves continued history with inventory
  verification, and legacy sessions get an explicit history-recovery route.
- Web restore and panel actions follow the selected session.

## Exclusions

- Execution-less stop settlement is unchanged.
- No interrupted instruction is resent.

## Acceptance conditions

1. Workspace-only restoration uses an execution owned by the restored session
   (`AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.7`,
   `AC-AGENTS-HARNESS-SESSION-CONTINUITY-003.1`).
2. Native recovery of a continued session keeps its history without resending
   the interrupted instruction (`AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.9`).
3. Settled continuation state does not trigger another recovery attempt or
   stale notice (`AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.24`), and the persisted
   recovery outcome matches the committed continuation
   (`AC-AGENTS-HARNESS-SESSION-CONTINUITY-005.1`).

## Verification

- `go build ./...` and `go vet` on changed packages.
- Web vitest for the touched areas.
- Same-test comparison against `main` for `task/repository/sqlite`,
  `orchestrator/executor`, and `orchestrator`; targeted lifecycle selection.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/context_continuation_recovery.go`
- `apps/backend/internal/task/repository/sqlite/context_continuation_attempt.go`
- `apps/backend/internal/task/repository/sqlite/context_continuation_ancestry.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_execution.go`
- `apps/backend/internal/agent/runtime/lifecycle/native_resume_history.go`
- `apps/backend/internal/orchestrator/context_continuation_route.go`
- `apps/backend/internal/orchestrator/session_launch.go`
- `apps/web/lib/services/session-launch-service.ts`

## Dependencies

None.
