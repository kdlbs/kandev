---
id: "01-backend-restart"
title: "Backend Configuration Chat replacement operation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-CONFIG-CHAT-RESTART-001
acceptance_criteria:
  - AC-TASKS-CONFIG-CHAT-RESTART-001.3
  - AC-TASKS-CONFIG-CHAT-RESTART-001.4
  - AC-TASKS-CONFIG-CHAT-RESTART-001.5
  - AC-TASKS-CONFIG-CHAT-RESTART-001.6
  - AC-TASKS-CONFIG-CHAT-RESTART-001.7
system_design:
  - ../../specs/tasks/system-design/configuration-chat-restart.md
---

# Task 01: Backend replacement operation

## Summary

Expose a bounded Configuration Chat restart operation that authorizes and
retires the exact old conversation before creating and starting a blank one.
This API is an independently verifiable prerequisite for the header action.

## In scope

- TDD for validation, preview tickets, scoped runtime access, synchronous stop,
  canonical deletion, launch selections, and stable partial-failure responses.
- A narrow orchestrator retirement interface separate from
  `OrchestratorStarter`; keep large handler/operation files limited to wiring.
- Exclude duplicate requests for the same old task; coordinate with existing
  session lifecycle admission and cancel transient retries before retirement.
- Project transient restart admission in the authorized quick-chat list and
  reject competing config-chat creation until the operation settles. Cover
  list reads spanning operation transitions and missing/lost POST responses.
- Explicit blank ACP/passthrough start and normal cleanup of failed preparation.

## Out of scope

Frontend, new persistence tables, provider-specific lifecycle changes, and
changes to ordinary task deletion semantics.

## Acceptance

1. Deferred fakes prove stop completes before deletion and deletion before
   replacement creation. Cover running, starting, waiting, terminal with a live
   runtime, and confirmed-absent runtimes; stop/lookup failure prevents creation.
2. Authorization/eligibility/ticket tests prove no effects for foreign or
   mismatched IDs, wrong chat kind, non-primary or extra sessions, invalid
   profile/executor, stale target, and duplicate concurrent requests. A blocked
   resume cannot reopen the old session during retirement; no lock deadlock.
3. Fault injection at delete/create/prepare/start proves truthful stage and
   identity responses, no unreachable replacement, no prompt replay, and
   exactly one passthrough launch. Accepted work retains caller scope after
   request cancellation and respects bounded timeouts.

## Verification

Run from the repository root, using `/tdd` and backend test guidance:

```bash
(cd apps/backend && go test -race ./internal/orchestrator ./internal/task/handlers -run 'Test(ConfigChatRestart|HTTPRestartConfigChat)' -count=1)
(cd apps/backend && go test ./internal/task/handlers -run 'Test(.*ConfigChat|HTTPListQuickChatSessions|QuickChatRoutesAreRegistered)' -count=1)
(cd apps/backend && go test ./internal/orchestrator -run 'Test(StartCreatedSession_EmptyPromptSkipsWrap|StartCreatedSession_ConfigMode|WorkflowAsyncStartFailure_.*Input)' -count=1)
(cd apps/backend && golangci-lint run --allow-serial-runners --new-from-rev=HEAD ./internal/task/handlers/... ./internal/task/service/... ./internal/orchestrator/...)
git diff --check
```

Name new tests with the listed prefixes so every new case runs. Record RED and
GREEN evidence; confirm nonzero discovered test counts.

## Files likely touched

- `apps/backend/internal/task/handlers/config_chat_restart.go` (new)
- `apps/backend/internal/task/handlers/config_chat_restart_test.go` (new)
- `apps/backend/internal/task/handlers/task_handlers.go`
- `apps/backend/internal/task/handlers/task_http_handlers.go` (config creation and list projection)
- `apps/backend/internal/task/handlers/quick_chat_list_handlers_test.go`
- `apps/backend/internal/task/handlers/quick_chat_list_authz_test.go`
- `apps/backend/internal/orchestrator/config_chat_restart.go` (new)
- `apps/backend/internal/orchestrator/config_chat_restart_test.go` (new)
- `apps/backend/internal/orchestrator/session_launch.go` (internal blank-start option)
- `apps/backend/internal/task/service/config_chat_restart.go` (new validation boundary,
  covered by the handler tests above)
- Existing task-service access/launch validation helpers if a narrow reusable
  entry point is needed; add adjacent tests and its exact command if modified.

## Dependencies

None.

## Risks

Internal synchronous stop skips authorization; guard the exact pair first.
Deletion callbacks may take lifecycle locks: preserve the documented order and
test bounded waits. Do not treat generic not-found as proof a remote agent is gone.

## Parallelism

`sequential`

## Inputs

- [Design: Backend operation and Failure recovery](../../specs/tasks/system-design/configuration-chat-restart.md#backend-operation)
- `apps/backend/AGENTS.md`
- `httpStartConfigChat`, `WithTaskDeleteConfirmation`, `DeleteTaskWithLifecycle`,
  `StopSessionSynchronously`, and existing config-chat launch tests.

## Results

Completed on 2026-10-01 in the primary session.

- RED: the restart route and retirement boundary were missing; later fault
  cases exposed implicit-local executor selection and partial preparation
  recovery. A real startup test caught configuration instructions being sent
  as an unsolicited first turn.
- GREEN: all 16 discovered restart test functions passed with the race
  detector, including their validation, runtime, concurrency and failure
  subtests. The configured handler regression suite and existing empty-prompt,
  config-context and asynchronous startup regression suites passed.
- The replacement explicitly starts with no recorded prompt and no initial
  configuration-only turn. Normal configuration message dispatch retains its
  existing context behavior. Partially prepared sessions remain reachable.
- Scoped `golangci-lint run --allow-serial-runners --timeout=5m
  --new-from-rev=HEAD` passed with zero issues. No migrations or public launch
  request fields were added.
