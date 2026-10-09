---
created: 2026-09-18
status: done
requirements:
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003
system_design:
  - ../../specs/tasks/system-design/workflow-explicit-completion-signal.md
legacy_specs: []
---

# Implementation plan: completion recovery after a step change

## Overview

[Issue #3772](https://github.com/kdlbs/kandev/issues/3772) reports repeated completion
rejections after a reviewer moves the task during a worker turn.
This package makes that rejection actionable while preserving stale-reviewer protection.
One sequential work order delivers diagnostics, instructions, and regression evidence.

## Evidence and root cause

Investigation base: `81b59416ab`.
`handleStepComplete` compares the latest turn's launch stamp with the current task step.
A mismatch always returns the same opaque validation error before claiming a signal.
Repeating the call or changing session state does not change that stamp.

The existing `TestHandleStepComplete_RejectsSignalFromMovedTurn` reproduces this
condition with a real SQLite repository. It passed during investigation.
Its purpose is to stop a stale reviewer from completing the rejected Work step.
Removing the comparison would regress that protection.

The issue's production timeline is reported evidence. The precise upstream
interleaving that applies a deferred move during worker execution was not reproduced.
The current deferred-move path has additional queue and session-identity guards.
This package does not claim to repair or fully explain that separate race.

The missing behavior is recovery guidance, covered by the proposed requirement
addition. The immutable turn stamp and safe rejection remain correct.
The current source does not contain the issue's exact "CALL THIS EXACTLY ONCE"
wording in the built-in completion sections. User-authored prompts are not rewritten.

## Scope

### In scope

- Step IDs and fresh-turn recovery instructions in the mismatch error.
- Consistent tool, task-mode, and Office instructions.
- Tests for rejection, fresh-turn recovery, and MCP error forwarding.
- Public recovery instructions for the implemented behavior.

### Out of scope

- Automatic restart, synthetic completion, or a default direct-move fallback.
- Restamping old turns or accepting stale signals from idle sessions.
- Deferred-move queue changes, issue #3716, and issue #3712.
- UI layout, migrations, new flags, or rewriting saved user prompts.

## Technical approach

The [system design](../../specs/tasks/system-design/workflow-explicit-completion-signal.md)
owns the response and recovery contract.
Update only the mismatch message in `internal/mcp/handlers/handlers.go`.
Keep its validation code and existing prefix. Include recovery text in the message
that `stepCompleteHandler` forwards through MCP.

Update `registerStepCompleteTool`, `stepCompleteSection`, and
`officeStepCompleteInstruction`. Preserve description budgets and conditional injection.
Do not change claims, subscriber behavior, or the task move API.

Public docs updates belong in `docs/public/tasks-and-workflows.md` and
`docs/public/automation-and-mcp.md`. These reference pages need a short recovery
note, not a new tutorial. Update them with implementation, not during this design turn.

## Tests

| Acceptance | Regression evidence |
| --- | --- |
| 003.1, 003.2 | `step_complete_recovery_test.go:TestHandleStepComplete_StaleTurnRecoveryGuidance`, table-driven RUNNING and WAITING_FOR_INPUT cases |
| 003.3 | `step_complete_recovery_test.go:TestHandleStepComplete_FreshTurnAfterStepChange` |
| 003.4 | `step_complete_recovery_test.go:TestStepCompleteHandler_ForwardsRecoveryGuidance` in the MCP server package, plus sysprompt recovery tests |
| 003.5 | Public docs validators and review against the proven fresh-turn sequence |

All short acceptance suffixes refer to
`AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003.*`.
The first new handler test must fail on the existing opaque response before implementation.
Retain the existing moved-turn, duplicate, concurrent-claim, and prompt-size tests.

## End-to-end evidence

The changed surface is MCP text. The server test sends a completion request through
`stepCompleteHandler` and proves that backend recovery text reaches the MCP error.
The SQLite handler test proves rejection followed by fresh-turn acceptance.
No browser test is required because this package changes no rendered UI behavior.
Automated text tests do not prove that every model obeys recovery instructions.

## Work orders

- [x] [Task 01: expose safe completion recovery](task-01-completion-recovery.md)

## Verification results

Investigation passed:

```bash
(cd apps/backend && go test ./internal/mcp/handlers -run TestHandleStepComplete_RejectsSignalFromMovedTurn -count=1)
```

Package validation passed:

- `python3 scripts/list-docs.py validate`: 288 decisions and 1003 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specifications passed.
- `git diff --check -- docs/specs docs/plans/step-completion-stale-turn-recovery`: passed.
- `git status --short -- docs/plans/step-completion-stale-turn-recovery`: both package files present.

Issue #3772 was assigned to `carlosflorencio` during investigation.

Implementation completed:

- Added launch-step and current-step IDs plus fresh-turn recovery guidance to
  stale completion errors while preserving validation and signal-claim guards.
- Updated MCP tool metadata, signal-gated task and Office prompts, and public
  task/MCP reference pages with the same recovery sequence.
- Added SQLite handler, MCP forwarding, prompt, and metadata regressions.
- `go test ./internal/mcp/handlers -run 'TestHandleStepComplete_' -count=1`
  passed.
- `go test ./internal/mcp/server -count=1` and `go test ./internal/sysprompt
  -count=1` passed.
- `go test -race ./internal/mcp/handlers -run 'TestHandleStepComplete_' -count=1`
  passed.
- Public-doc validators, specification catalog/lint checks, `git diff --check`,
  and `make -C apps/backend build` passed.

## Risks

- Recovery still needs a fresh user-started turn or an operator's manual move.
- A task can change steps again after recovery. The mismatch must remain safe.
- Saved prompts can contradict new built-in guidance. This package does not overwrite them.
- Broadening the engine fix requires separate evidence for the deferred-move race.
