---
id: "01-completion-recovery"
title: "Expose safe completion recovery"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003
acceptance_criteria:
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003.1
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003.2
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003.3
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003.4
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003.5
system_design:
  - ../../specs/tasks/system-design/workflow-explicit-completion-signal.md
---

# Task 01: expose safe completion recovery

## Summary

Make stale-turn completion errors explain the cause and the fresh-turn recovery path.
Preserve the rejection and document the distinction from a duplicate accepted signal.

## In scope

- Add the plan's handler regressions before changing the error.
- Use `seedStepCompleteTarget` and `CreateTurnWithStepStamp` for Review to Work scenarios.
- Assert both IDs and recovery text in repeated rejected responses.
- Assert no event or pending signal after stale calls, in running and idle states.
- Create a later Work turn and assert ordinary completion acceptance.
- Exercise MCP forwarding with the existing `testBackend` server fixture.
- Cover task and Office prompt guidance, gating, final-action rules, and size limits.
- Update the two public reference pages with the tested recovery sequence.

## Out of scope

Queue or orchestrator changes, automatic task moves, turn restamping, UI, and migrations.

## Acceptance

1. The new guidance test fails on the old response, then passes with the correction.
2. Stale calls remain harmless, while a fresh current-step turn can signal normally.
3. The MCP response, built-in prompts, and public docs describe the same recovery behavior.

## Verification

Run the new handler test first and record its expected failure. Then implement
and run these commands from the repository root:

```bash
(cd apps/backend && go test ./internal/mcp/handlers -run 'TestHandleStepComplete_' -count=1)
(cd apps/backend && go test ./internal/mcp/server ./internal/sysprompt -count=1)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/step_complete_recovery_test.go` (new)
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/step_complete_recovery_test.go` (new)
- `apps/backend/internal/sysprompt/sysprompt.go`
- `apps/backend/internal/sysprompt/step_complete_recovery_test.go` (new)
- `docs/public/tasks-and-workflows.md`
- `docs/public/automation-and-mcp.md`

## Dependencies

None. Implementation completed in this work order after the explicit implementation request.

## Risks

Keep the tool description within the existing metadata budget.
Do not equate `already_signaled` with a stale rejection.
Do not add a backend side effect to make the text recovery test pass.

## Parallelism

`sequential`

## Inputs

- [Requirement 003](../../specs/tasks/requirements/workflow-explicit-completion-signal.md)
- [System design](../../specs/tasks/system-design/workflow-explicit-completion-signal.md)
- `apps/backend/internal/mcp/handlers/step_complete_test.go`
- `apps/backend/internal/mcp/server/step_complete_description_test.go`
- `apps/backend/internal/sysprompt/mcp_discovery_test.go`
- [ADR 0015](../../decisions/0015-explicit-completion-signal-for-auto-advance.md)

## Results

Completed. The stale-turn rejection now includes both step IDs and directs the
agent to end the stale turn and have the user resume the session. A fresh turn
stamped with the current step can signal normally. MCP forwarding, tool
metadata, task and Office prompts, public docs, and the existing stale-turn,
duplicate, concurrency, and prompt-size coverage remain aligned.
