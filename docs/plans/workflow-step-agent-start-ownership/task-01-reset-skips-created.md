---
id: "01-reset-skips-created"
title: "Context reset skips never-prompted sessions"
status: done
wave: 1
parallelism: sequential
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/workflow-step-agent-start-ownership.md"
---

# Task 01: Context reset skips never-prompted sessions

## Acceptance

- `resetAgentContext` returns success without restarting the subprocess when
  `session.State` is `CREATED`, so `auto_start_agent` remains the only path that
  starts the agent on that step entry.
- The skip happens before the agent-execution lookup, so a `CREATED` session
  with a prepared workspace-only execution never reaches `ResetAgentContext`.
- Sessions in every other state keep today's behavior: subprocess restarted and
  `acp_session_id` cleared.
- Passthrough `CREATED` sessions are skipped on the same rule.
- The session-scoped public `Service.ResetAgentContext` entry point is not
  changed by this task.

## Regression test

Add to `TestProcessOnEnterResetAgentContext` in
`apps/backend/internal/orchestrator/event_handlers_workflow_triggers_test.go`:

- **Red first.** A subtest seeding a session with `State = CREATED` and a
  prepared execution, entering a step whose `on_enter` is
  `reset_agent_context` + `auto_start_agent`, asserting
  `len(agentMgr.restartProcessCalls) == 0`. Before the fix this reads 1.
- Assert `acp_session_id` is still empty afterward (it is empty for a
  never-prompted session; the test must not assume the clear ran).
- Keep the existing `RUNNING` subtests untouched — `seedSession`
  (`event_handlers_test.go:637`) seeds `RUNNING`, so they continue to pin the
  restart path.

## Verification

```bash
(cd apps/backend && go test ./internal/orchestrator/ -race -run 'TestProcessOnEnterResetAgentContext|TestAutoStart')
```

```bash
(cd apps/backend && go test ./internal/orchestrator/... -race)
```

## Files Likely Touched

- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow_triggers_test.go`

## Dependencies

None.

## Inputs

- Spec scenarios 1–4.
- `resetAgentContext` at `event_handlers_workflow.go:2137` and its call site at
  `:1123`.
- `markIdleAfterReset` at `:2117`, whose comment already documents the intended
  invariant.

## Output Contract

`resetAgentContext` is a no-op returning `true` for `CREATED` sessions; all
other states unchanged. No signature change.
