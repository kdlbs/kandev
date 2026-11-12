---
id: "01-source-claims"
title: "Bind signal claims to their source"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-COMPLETION-OUTCOMES-001
  - REQ-TASKS-COMPLETION-OUTCOMES-002
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003
  - REQ-TASKS-SIGNAL-PAYLOAD-DELIVERY-001
  - REQ-TASKS-SIGNAL-PAYLOAD-DELIVERY-003
acceptance_criteria:
  - AC-TASKS-COMPLETION-OUTCOMES-001.1
  - AC-TASKS-COMPLETION-OUTCOMES-001.2
  - AC-TASKS-COMPLETION-OUTCOMES-001.3
  - AC-TASKS-COMPLETION-OUTCOMES-001.6
  - AC-TASKS-COMPLETION-OUTCOMES-002.1
  - AC-TASKS-COMPLETION-OUTCOMES-002.5
  - AC-TASKS-COMPLETION-OUTCOMES-002.6
  - AC-TASKS-COMPLETION-OUTCOMES-002.7
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001.4
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003.6
  - AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-001.13
  - AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-003.5
system_design:
  - ../../specs/tasks/system-design/workflow-completion-outcomes.md
  - ../../specs/tasks/system-design/workflow-explicit-completion-signal.md
  - ../../specs/tasks/system-design/workflow-completion-signal-payload-delivery.md
---

# Task 01: Bind signal claims to their source

## Summary

Distinguish a duplicate completion from a later completed correction in the same entry.
Store the winning signal identity and one bounded pending receipt atomically.
Return honest source and current-route data without changing public arguments.

## In scope

- Extend `stepCompletionLaunchStep` with the persisted source-turn ID.
- Add typed signal identity and receipt models with typed/map JSON parity.
- Extend `ClaimStepCompletionSignal` with same-source deduplication and later-source replacement.
- Recheck captured source evidence at claim, including a turn created between the initial read and transaction admission.
- Keep current task-entry validation, ownership checks, and the explicit no-turn compatibility path.
- Return stored identity on duplicates. Preserve notification retry for pending signals.
- Preserve the duplicate truncation exclusion in payload-delivery AC-003.5a.
- Keep a native verification turn neutral until another completion call occurs.
- Add pending, blocked/unconfigured projection support without claiming a committed move before Task 02 wires proof.

## Out of scope

Transition proof, destination lifecycle changes, Review-cycle acceptance, tool instructions, and public documentation belong to Task 02.
This work order adds no schema, public identity argument, or permission.

## Acceptance

1. The later-native-turn regression fails before the change and passes afterward. A same-turn duplicate preserves the first payload and acceptance count.
2. Claims and receipt writes serialize with moves and stamped turn creation. Foreign, replaced, and older-entry sources gain no authority or receipt disclosure.
3. Duplicate and legacy results preserve existing fields and truncation semantics. Unknown proof remains unknown, and no receipt infers commitment from `REVIEW`.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/models ./internal/task/repository/sqlite ./internal/mcp/handlers -run 'Test(StepCompletionOutcome|PostgresStepCompletionOutcome|StepCompletionEntry|HandleStepComplete_)' -count=1)
(cd apps/backend && go run -trimpath ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/persistence/storeconformance -count=1)
bash scripts/lint-go-changed apps/backend/internal/task/models/models.go apps/backend/internal/task/models/step_completion_outcomes.go apps/backend/internal/task/models/step_completion_outcomes_test.go apps/backend/internal/task/repository/sqlite/step_completion_entry.go apps/backend/internal/task/repository/sqlite/step_completion_outcomes_test.go apps/backend/internal/task/repository/sqlite/step_completion_outcomes_postgres_test.go apps/backend/internal/mcp/handlers/handlers.go apps/backend/internal/mcp/handlers/step_completion_entry.go apps/backend/internal/mcp/handlers/step_complete_outcomes.go apps/backend/internal/mcp/handlers/step_complete_outcomes_test.go
git diff --check
```

The PostgreSQL cases use `KANDEV_TEST_POSTGRES_DSN` against a disposable database.
Record skipped PostgreSQL coverage separately if that variable is absent.
Do not record planned tests as executed evidence.

## Files likely touched

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/models/step_completion_outcomes.go` and `_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/step_completion_entry.go`
- `apps/backend/internal/task/repository/sqlite/step_completion_outcomes_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/step_completion_outcomes_postgres_test.go` (new)
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/step_completion_entry.go`
- `apps/backend/internal/mcp/handlers/step_complete_outcomes.go` and `_test.go` (new)

## Dependencies

None. Existing entry-bound turn attribution is already implemented.

## Risks

Do not order sources by wall-clock time or random IDs.
Do not acquire the session-turn advisory lock after the task lock.
The retained receipt is one slot, so unmatched historical calls must fail safely.

## Parallelism

`sequential`

## Inputs

- [Plan evidence, scope, and test targets](plan.md)
- [Outcomes requirements](../../specs/tasks/requirements/workflow-completion-outcomes.md)
- [Outcomes design: source identity, receipt, compatibility](../../specs/tasks/system-design/workflow-completion-outcomes.md)
- Existing `step_complete_delivery_turn_test.go`, `step_complete_test.go`, and `step_completion_entry_test.go` patterns.
- `.agents/skills/tdd/references/backend-tests.md`

## Results

Done.

- `PendingStepCompletionSignal` now carries a minted `signal_id` and the persisted `source_turn_id`; the launch resolver returns the source turn.
- `ClaimStepCompletionSignal` deduplicates by `(session, source turn, workflow entry)`, keeps the first payload within one source, replaces an unconsumed earlier turn from the current persisted turn, and refuses an older source after re-entry.
- The winning claim writes one pending receipt atomically with the signal. Duplicate and terminal-session retries return the retained receipt without another transition.
- Handler responses add `signal_id`, `source`, `outcome`, `transition` when proof exists, and a `current` snapshot read after publication from one repository statement covering state, route, entry, queue placement, and primary session. Existing `accepted`, `reason`, `step_id`, `signaled_at`, `advances`, and truncation semantics are preserved. Unconfigured steps record `not_configured`.
- Legacy no-turn signals remain entry-wide first-wins and explicitly `unproven`.

Verification: handler regressions passed; model, repository, and handler race runs passed; `gofmt` and `scripts/lint-go-changed` passed. PostgreSQL behavior tests were added but skipped because `KANDEV_TEST_POSTGRES_DSN` was unset.
