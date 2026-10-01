---
id: "02-step-complete-advances-move-action"
title: "Report advances only when a signal-gated step has a move action"
status: done
wave: 2
depends_on:
  - "01-assignment-wake-step-eligibility"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001.2
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001.3
system_design:
  - ../../specs/tasks/system-design/workflow-explicit-completion-signal.md
---

# Task 02: Report advances only when a signal-gated step has a move action

## Summary

Task 01 derived `advances` from `auto_advance_requires_signal` alone. A
signal-gated step whose `on_turn_complete` has no move action the engine runs
therefore answered `advances:true`, although the recorded signal can never
produce a transition. Report `advances:false` with an explanatory `note` for
that shape, keep `accepted:true`, and keep recording the signal.

## In scope

- Add `WorkflowStep.AdvancesOnTurnComplete` in `internal/workflow/models`,
  matching the engine: `move_to_next`, `move_to_previous`, and `move_to_step`
  with a non-empty target different from the current step count;
  `requires_approval` moves and `disable_plan_mode` do not. Preserve action
  ordering: an unguarded self-target blocks later moves, while a guarded
  self-target can fall through when its guard is not satisfied.
- Use it in `resolveStepCompletionAdvances` and return a distinct `note` for a
  signal-gated step without a move.
- Handler regressions for no actions, `disable_plan_mode` only, `move_to_step`
  without `step_id`, a self-target alone and before a later move, and a
  `requires_approval` move, asserting that the signal is still recorded and
  published.
- Model coverage for self-target ordering and an engine parity test that runs
  the compiled actions through actual transition evaluation.

## Out of scope

- Rejecting the call or skipping the signal write.
- Prompt injection, which still keys on `auto_advance_requires_signal`.
- The workflow settings editor, which can keep the signal flag set after the
  `on_turn_complete` transition is changed to "Do nothing".
- Orchestrator gating, the stuck-signal watchdog, and signal clearing.

## Acceptance

1. The new handler test fails on the previous behavior with `advances:true`
   for every signal-gated step without a runnable move.
2. After the change, those steps return `accepted:true`, `advances:false`, and
   a `note` stating that the step has no `on_turn_complete` move that runs
   automatically; the pending signal and bus event are unchanged.
3. Existing gated-with-move, non-gated, and lookup-failure responses keep
   their fields and `note` text.
4. An unguarded self-target, including one before a later valid move, returns
   `advances:false`; a guarded self-target can fall through to a later move
   when the guard is not satisfied.

## Verification

```bash
(cd apps/backend && go test -tags fts5 ./internal/mcp/handlers -run "StepComplete" -count=1)
(cd apps/backend && go test -tags fts5 ./internal/workflow/models -count=1)
(cd apps/backend && go test -tags fts5 ./internal/workflow/engine -run "TestCompileStep_OnTurnCompleteMovesMatchAdvancesOnTurnComplete" -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/workflow/models/models.go`
- `apps/backend/internal/workflow/models/models_test.go`
- `apps/backend/internal/workflow/engine/types_test.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/step_complete_advances_test.go`
- `docs/specs/tasks/requirements/workflow-explicit-completion-signal.md`
- `docs/specs/tasks/system-design/workflow-explicit-completion-signal.md`

## Parallelism

`sequential`

## Inputs

- [Requirement 001](../../specs/tasks/requirements/workflow-explicit-completion-signal.md)
- [System design](../../specs/tasks/system-design/workflow-explicit-completion-signal.md)
- [ADR 0015](../../decisions/0015-explicit-completion-signal-for-auto-advance.md)
- `apps/backend/internal/workflow/engine/types.go` (`compileOnTurnComplete`)
- `apps/backend/internal/workflow/engine/engine.go` (`evaluateActions`)

## Results

Completed in the contributor PR. The handler cases failed before that change
with `expected: false, actual: true` for `advances` and an empty `note`, and
passed after it. The existing step-completion handler tests, model test, and
engine comparison test passed.

### Follow-up correction

An unguarded `move_to_step` targeting the current step is selected by the
engine. It blocks later transition actions, then does not commit a transition.
`AdvancesOnTurnComplete` now reports this case as false. A guarded self-target
can fall through when its guard is not satisfied, so a later valid move still
counts. Regression coverage includes self-only, self-before-valid, and
invalid-guard self-target cases in the model and engine parity tests, plus
self-only and self-before-valid cases in the handler.

Verification passed:

- `go test -tags fts5 ./internal/workflow/models ./internal/workflow/engine ./internal/mcp/handlers`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- The commit hooks passed, including Go lint for the changed packages.

The correction was committed as `a79821d5fd` and pushed to the contributor's
PR branch over SSH.
