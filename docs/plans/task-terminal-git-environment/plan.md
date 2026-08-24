---
spec: docs/specs/integrations/requirements/github-authentication.md
created: 2026-07-28
status: completed
---

# Implementation Plan: Managed Task Terminal Git Environment

## Overview

Extend the existing managed GitHub credential contract from agent subprocesses to every authorised
task execution surface: local/worktree terminals, remote terminal shells, passthrough-agent PTYs,
legacy agentctl shells, and task-scoped command processes. Reuse the current per-execution
environment and ownership guards; do not persist, log, or expose the broker contract to the
browser.

## Backend

### Capture and provide the effective execution environment

Files:

- `apps/backend/internal/agent/runtime/lifecycle/types.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_execution.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_passthrough.go`
- `apps/backend/internal/gateway/websocket/terminal_handler.go`

Changes:

- Keep the already-resolved per-execution environment in memory on `AgentExecution`, using a
  defensive copy and clearing it when the execution is removed.
- Provide the effective environment only to lifecycle-owned, already-authorized terminal/process
  launch paths; preserve explicit terminal/process overrides as the highest precedence.
- Use it for local/worktree user-shell terminals and passthrough-agent PTYs, including their
  managed `GIT_CONFIG_*` block and shim-first `PATH`.
- Preserve executor inheritance: environments without a broker contract retain their current
  host/executor behavior and receive no Kandev shim.

### Make agentctl shell and command launch surfaces inherit instance environment

Files:

- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/process/runner.go`
- `apps/backend/internal/agentctl/server/process/interactive_shells.go`

Changes:

- Merge the instance `AgentEnv` into the legacy `/shell/start` shell, per-terminal agentctl
  shells, and `StartProcess` command launches before their explicit call-site environment.
- Retain the existing indexed-Git configuration exactly as one ordered block; do not reconstruct
  or log its individual credential values.
- Make process start fail before spawning when effective-environment preparation cannot satisfy
  the managed contract, without ambient Git/`gh` fallback.

### Public documentation

Files:

- `docs/public/executors.md`
- `docs/public/developer-tools.md`

Changes:

- Document that a terminal opened in a managed task receives the same task-scoped Git/`gh`
  routing as its agent, while executor-inheritance mode leaves credentials to the executor.
- State that an already-open terminal keeps its launch environment; open a new terminal after a
  session resume or credential-policy change.

## Frontend

No UI or browser protocol changes are required. Existing terminal ownership checks and terminal
routes remain the boundary; the backend supplies the process environment after authorization.

## Tests

- **What:** a local/worktree terminal and a passthrough-agent PTY inherit a managed broker
  contract, Git helper configuration, and shim-first `PATH`; executor-inheritance terminals do
  not gain that shim.
  **Files:** `apps/backend/internal/agent/runtime/lifecycle/manager_launch_test.go`,
  `apps/backend/internal/agent/runtime/lifecycle/manager_passthrough_test.go`, and
  `apps/backend/internal/gateway/websocket/terminal_handler_test.go`.
  **How:** focused Go tests with a synthetic non-secret broker contract and captured PTY start
  environment; assert authorization fails before environment retrieval.
- **What:** legacy agentctl shells, per-terminal agentctl shells, and task-scoped command starts
  inherit `AgentEnv`, while explicit call-site values override inherited ones.
  **Files:** `apps/backend/internal/agentctl/server/process/runner_shells_test.go` and
  `apps/backend/internal/agentctl/server/process/runner_test.go`.
  **How:** table-driven environment merge tests and a real short-lived child process that reports
  only non-secret marker variables.
- **What:** public documentation remains valid.
  **Files:** `docs/public/executors.md`, `docs/public/developer-tools.md`.
  **How:** `node --test scripts/validate-public-docs.test.mjs` and
  `node scripts/validate-public-docs.mjs`.

## E2E Tests

No new Playwright test is planned. The terminal UI and WebSocket contract are unchanged; focused
backend tests exercise the authorization boundary and the actual process environments without
putting a broker contract into an E2E fixture or browser-visible output.

## Implementation Waves

Wave 1:

- [x] [Task 01: Runtime terminal environment propagation](task-01-runtime-terminal-environment.md)

Wave 2:

- [x] [Task 02: Agentctl shell and process inheritance](task-02-agentctl-shell-process-environment.md)

Wave 3:

- [x] [Task 03: Document and verify terminal Git routing](task-03-document-terminal-git-routing.md)

Execute sequentially in the primary conversation. No task is delegated unless the user explicitly
authorizes delegation after choosing the implementation model.

## Risks and non-goals

- Broker leases and indexed Git configuration are sensitive runtime data. They must remain
  in-memory and must not enter terminal metadata, logs, errors, frontend payloads, or command
  arguments.
- A terminal already owns host-level execution authority for local/worktree tasks; this change
  aligns its credential behavior with the task agent and is not a sandbox boundary.
- This plan does not grant terminal access across task-environment ownership boundaries, change
  credential policy selection, persist terminal environments, or support arbitrary host shells.

## Open Questions

None.
