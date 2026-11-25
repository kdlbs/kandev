---
id: "01-backend-terminal-owner"
title: "Resolve persisted terminal ownership"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-TERMINALS-002
acceptance_criteria:
  - AC-TASKS-TASK-TERMINALS-002.1
  - AC-TASKS-TASK-TERMINALS-002.6
system_design:
  - ../../specs/tasks/system-design/task-terminal-persistence.md
---

# Task 01: Resolve persisted terminal ownership

## Summary

Use persisted terminal ownership when a shared environment's execution belongs
to another task. Keep existing authorization and command-claim boundaries.
Restrict environment-qualified lists to matching ordinary rows.

## In scope

- Add an internal service lookup for the owner of an environment/terminal pair.
  Reject missing metadata, an empty owner, and a different environment.
- Update `resolveOrdinaryTerminalLaunch` to authorize that pair through
  `CheckTaskEnvironmentAccess` before `ClaimShellLaunchInfo` consumes a command.
  Keep the environment execution check and shared local/remote entry point.
- Filter `wsUserShellList` and environment-qualified `httpListTaskTerminals`.
  Preserve task-only HTTP listing and unmanaged union items.
- Add regressions before production changes. Use the real repository and service.

## Out of scope

- Browser state, panel reconciliation, transport URL changes, and new permissions.
- Changes to `requireOwnership`, atomic command claims, or repository schema.

## Acceptance

1. `TestOrdinaryTerminalLaunchUsesPersistedOwnerInSharedEnvironment` fails on
   the baseline gateway. It then passes for A and B with either task's
   execution serving the environment, for local and fake remote attachment.
2. Denied task/environment access, a foreign environment, and missing metadata
   expose no launch command and start no PTY. Denial leaves a one-shot command
   unclaimed. Existing explicit cross-task mutation tests remain green.
3. Mixed task/environment list fixtures return only matching ordinary rows.
   Task-only HTTP lists, fixed/script items, ordering, and parked filters retain
   their current contracts.

## Verification

Run from the repository root. New filenames below are owned by this work order.

```bash
(cd apps/backend && go test -tags fts5 -trimpath -race ./internal/terminal/service ./internal/gateway/websocket ./internal/agent/handlers)
bash scripts/lint-go-changed apps/backend/internal/terminal/service/launch_info.go apps/backend/internal/terminal/service/launch_info_test.go apps/backend/internal/gateway/websocket/terminal_handler.go apps/backend/internal/gateway/websocket/terminal_ownership_test.go apps/backend/internal/agent/handlers/shell_handlers.go apps/backend/internal/agent/handlers/shell_scope_test.go
git diff --check
```

## Files likely touched

- `apps/backend/internal/terminal/service/launch_info.go` (new owner lookup).
- `apps/backend/internal/terminal/service/launch_info_test.go` (new).
- `apps/backend/internal/gateway/websocket/terminal_handler.go`.
- `apps/backend/internal/gateway/websocket/terminal_ownership_test.go` (new).
- `apps/backend/internal/agent/handlers/shell_handlers.go`.
- `apps/backend/internal/agent/handlers/shell_scope_test.go` (new).

## Dependencies

None. Task 03 consumes this corrected gateway behavior.

## Risks

The new lookup must not claim an authentication command before authorization.
Tests must distinguish an authorized shared task from an unrelated task or
environment. Do not replace pair authorization with two independent checks.

## Parallelism

`sequential`

## Inputs

- [Ownership and list design](../../specs/tasks/system-design/task-terminal-persistence.md#terminal-ownership).
- [Owning requirements](../../specs/tasks/requirements/task-terminal-persistence.md).
- Existing `TestShellLaunchInfoReadsPersistedCommandAndChecksEnvironment` and
  `TestStartUserShellProcessExecutesPersistedTerminalCommand` fixtures.
- `terminal_handler_remote_test.go` fake agentctl and
  `service_access.go::AuthorizeTaskEnvironmentAccess` shared-task authorization.
- `apps/backend/AGENTS.md` and the backend TDD test reference.

## Results

Completed. `OwnerForTerminal` reads the persisted row and validates the
environment without consuming a command. `resolveOrdinaryTerminalLaunch`
authorizes the resolved owner through `CheckTaskEnvironmentAccess` before
`ClaimShellLaunchInfo`, and both list handlers filter ordinary rows by the
requested environment.

Verification executed from the repository root:

```bash
(cd apps/backend && go build -tags fts5 -trimpath ./...)
(cd apps/backend && go test -tags fts5 -trimpath -race ./internal/terminal/service ./internal/gateway/websocket ./internal/agent/handlers)
bash scripts/lint-go-changed apps/backend/internal/terminal/service/launch_info.go apps/backend/internal/terminal/service/launch_info_test.go apps/backend/internal/terminal/service/service.go apps/backend/internal/gateway/websocket/terminal_handler.go apps/backend/internal/gateway/websocket/terminal_ownership_test.go apps/backend/internal/agent/handlers/shell_handlers.go apps/backend/internal/agent/handlers/shell_scope_test.go
git diff --check
```

The three package test runs passed, the changed-file Go lint reported
`0 issues`, and `git diff --check` passed.
