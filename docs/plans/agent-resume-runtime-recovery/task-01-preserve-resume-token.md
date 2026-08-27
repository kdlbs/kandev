---
id: "01-preserve-resume-token"
title: "Preserve resume tokens on startup failure"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/agent-resume-runtime-recovery.md"
---

# Task 01: Preserve resume tokens on startup failure

## Acceptance

- Process startup, ACP initialize, and pre-session transport failures do not
  clear `executors_running.resume_token`.
- Those failures use the existing recoverable-failure state and action path.
- Explicit `session.recover` with `fresh_start` still clears the token before
  launching fresh.

## Verification

- `cd apps/backend && go test ./internal/orchestrator -run 'TestHandleAgentFailed|TestRecoverSession'`

The new token-retention regression must fail for the expected blank-token
assertion before production code changes and pass afterward.

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/event_handlers_test.go`
- `apps/backend/internal/orchestrator/session_launch_test.go`

## Dependencies

None.

## Parallelism

Sequential. It defines the recovery semantics consumed by Task 02.

## Inputs

- Repair spec sections `Expected behavior`, `Persistence and security
  constraints`, and the first two regression scenarios
- Plan section `Preserve resume identity on startup failure`
- Existing `handleRecoverableFailure`, `clearResumeToken`, and
  `RecoverSession` patterns

## Output contract

Report the failing-test evidence, changed recovery branches, retained explicit
fresh-start behavior, exact test result, files changed, blockers, and risk
notes. Mark this task `done` and update its plan checkbox in the same
conversation.
