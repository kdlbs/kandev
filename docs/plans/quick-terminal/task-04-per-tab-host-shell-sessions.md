---
id: "04-per-tab-host-shell-sessions"
title: "Add per-tab host-shell sessions"
status: done
wave: 4
depends_on: ["03-fix-quick-terminal-startup-race"]
plan: "plan.md"
spec: "../../specs/ui/requirements/quick-terminal.md"
---

# Task 04: Add per-tab host-shell sessions

## Acceptance

- `POST /api/v1/host-shell/start` accepts an optional UUID `client_id`: the same value is
  idempotent, different values create independent running PTYs, an omitted value retains the legacy
  singleton behavior, and malformed values return `400 Bad Request`.
- The manager separates its internal uniqueness key from the public `AgentID`, preserving one
  login PTY per agent, `agent_id: "_host_shell"` responses, exit callbacks, and independent
  stop/cleanup for each host terminal.
- Focused manager and handler tests cover same/different/missing/invalid client IDs, concurrent
  command sessions, and stopping one session without affecting its sibling.

## Verification

From the repository worktree:

```bash
cd apps/backend && go test ./internal/agent/loginpty
```

## Files likely touched

- `apps/backend/internal/agent/loginpty/handlers.go`
- `apps/backend/internal/agent/loginpty/handlers_test.go`
- `apps/backend/internal/agent/loginpty/manager.go`
- `apps/backend/internal/agent/loginpty/manager_test.go`
- `docs/plans/quick-terminal/plan.md` (status/results only)
- `docs/plans/quick-terminal/task-04-per-tab-host-shell-sessions.md` (status/results only)

## Dependencies

- Task 03 is the shipped PTY lifecycle baseline. Preserve its output-drain, StrictMode-facing
  idempotency, timeout, and cleanup behavior.

## Parallelism

Sequential. Task 05 consumes the exact request and ownership contract established here.

## Inputs

- Spec: `API surface`, `State machine`, `Failure modes`, and `Persistence guarantees`.
- Plan: `Per-tab host-shell idempotency`, `Manager lifecycle`, backend tests, and backend risks.
- Existing patterns:
  `apps/backend/internal/agent/loginpty/handlers.go:httpStartHostShell`,
  `Manager.Start`, `Manager.supervise`, and the existing manager tests.

## Implementation notes

- Follow TDD: first add failing tests for two client IDs and the unchanged omitted-ID path.
- Keep client IDs as bounded opaque map keys. They must never alter the shell command, environment,
  filesystem path, public agent identity, or discovery-cache callback identity.
- Avoid weakening the one-session-per-agent invariant used by login/auth terminals.
- Ensure every test stops its PTYs and leaves no goroutine or process behind.

## Risks

- Deleting manager state by public `AgentID` would remove the wrong host terminal once several
  sessions share `_host_shell`; cleanup must use the internal key.
- A handler-only idempotency map can drift from manager cleanup. Keep uniqueness ownership in the
  manager so timeout, natural exit, and explicit stop follow one removal path.

## Output contract

Report the contract implemented, files changed, exact test count/result, process cleanup evidence,
blockers, residual risks, and synchronized task/plan status. Set this task to `in_progress` before
production or test changes and replace `## Results` before marking it `done`.

## Results

- Implemented manager uniqueness keys separate from the stable public `AgentID`, including
  per-key cleanup and preserved `Manager.Start`/agent-login semantics.
- Added UUID validation and per-client host-shell keys while retaining the omitted-client singleton.
- Added manager and `httptest` handler coverage for idempotency, sibling isolation, public identity,
  invalid IDs, and legacy behavior.
- `cd apps/backend && go test ./internal/agent/loginpty -count=1` — 8 passed.
- `cd apps/backend && go test -race ./internal/agent/loginpty -count=1` — 8 passed.
- Every PTY started by the tests is stopped through cleanup; the manager cleanup assertions wait for
  the first session to leave the manager while confirming its sibling remains running.
