---
id: "02-entry-bound-completion"
title: "Fence completion by workflow entry and prove fallback advancement"
status: done
wave: 2
depends_on:
  - "01-atomic-fallback-stamp"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003
acceptance_criteria:
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001.1
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001.4
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001.5
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003.1
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003.2
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003.3
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003.6
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003.7
system_design:
  - ../../specs/tasks/system-design/workflow-explicit-completion-signal.md
---

# Task 02: fence completion by entry and prove advancement

## Summary

Complete a real canonical fallback turn successfully while preserving stale-step
and stale-entry rejection. Carry its entry fence through atomic claims, duplicate
handling, delayed consumption, and the final guarded workflow move.

## In scope

- First install a real handler test for A -> B -> A that fails because the old
  turn is accepted; do not claim the existing handler already rejects entries.
- Resolve the latest turn's step and entry together; preserve missing-step
  internal errors, current no-turn compatibility, and existing stale-step
  diagnostic. A missing entry cannot be filled from an earlier turn or an
  unlocked task. Keep stale-entry recovery equally actionable.
- Add entry-aware task-conditional claiming under the existing task-row lock,
  checking task/session ownership and latest entry in that transaction. Avoid
  new inverted session/task locks. Do not weaken production by falling back to
  the older step-only capability when entry evidence exists.
- Store entry evidence with presence in the existing PendingStepCompletionSignal
  JSON. Test typed/map decode, exact numeric parsing, explicit zero versus
  missing, legacy signals/turns, unknown/corrupt entry, restart round-trip,
  duplicate first-writer payload, publish-failure retry and fresh replacement.
- Entry-match before consuming signal in engine/legacy turn-end paths, waiting
  subscriber, watchdog, handoff/blocker carry and audit. Use the existing
  `WithWorkflowStepTransitionGuard` at the actual source-task move CAS boundary.
  Delayed events never create authority. Cleanup must compare the observed
  signal before deleting it and preserve a newer valid signal.
- Real PostgreSQL multi-connection claims and moves: changed step, A -> B -> A
  between handler read/claim, and between consumption/transition. Rejection
  leaves no new signal/event/transition. Fresh entry still succeeds. Retain
  already-signaled behavior for the same entry under concurrent duplicates.
- Join public receipt/projection to real handler and configured signal-gated
  workflow transition. Prior turn closed -> text/thinking or shell wake ->
  final completion -> exactly one transition. Replay/duplicate adds none.
  Test RUNNING and WAITING_FOR_INPUT, pending clarification, stale prior entry,
  successor entry and no-step error. Include MCP server error forwarding.
- Use `/docs-maintainer`: update `automation-and-mcp.md` (reference) and
  `workflow-tips.md` (workflow guidance) with new-turn self-wake behavior and
  fresh-turn recovery for historical missing entry/stamp evidence. Reconcile
  existing stale-turn prose in `tasks-and-workflows.md` if affected. No new UI.
- Synchronize statuses/results and existing spec links. Preserve historical
  companion package validation rather than inventing new results for it.

## Out of scope

Task 01 stamping/retry ownership, a general user-reply/turn-cancellation redesign,
CLI-specific wake implementation, automatic restart, history backfill, new MCP
arguments, new UI, schema migrations or releases.

## Acceptance

1. Real projected fallback turns can signal and drive the configured transition
   once. Existing stale-step and unstamped-latest rejection remain; stale entries
   are rejected even after revisiting the same step or moving during claim.
2. An accepted signal stays bound to its originating entry through storage,
   replay, duplicate publication and transition CAS. Conditional stale cleanup
   preserves a newer valid signal. Questions remain completion barriers.
3. Real PostgreSQL races and the integrated canonical-output -> MCP -> workflow
   flow pass; public recovery guidance matches proven behavior and the plan
   records exact commands, results, skipped tests and final revision.

## Verification

Run every block from repo root. A disposable real PostgreSQL DSN is required.

```bash
(cd apps/backend && go test -race ./internal/task/models ./internal/task/repository/sqlite -run 'Test.*StepCompletionEntry|Test.*PendingStepSignal|TestCanonicalDeliveryTurn|TestSetSessionMetadataKeyIfAbsentOrDifferentStep' -count=1)
: "${KANDEV_TEST_POSTGRES_DSN:?Set a disposable real PostgreSQL DSN before acceptance}"
(cd apps/backend && go test -race -v ./internal/task/repository/sqlite -run 'TestPostgresStepCompletionEntry|TestPostgresCanonicalDeliveryTurn|TestPostgresUpdateTaskWithWorkflowStepAdmissionIfAtStep' -count=1)
(cd apps/backend && go test -race ./internal/mcp/handlers ./internal/mcp/server -run 'TestHandleStepComplete|TestStepComplete' -count=1)
(cd apps/backend && go test -race ./internal/orchestrator -run 'TestDeliveryFallbackStepCompletion|Test.*StepSignal|Test.*StepCompletion|Test.*ExplicitSignal|Test.*Handoff|Test.*StepHistory|Test.*StuckSignal|TestReplayedTerminalIntentAdvancesOnce|TestDeliveryRetentionWorkflowOutcome' -count=1)
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run 'TestDurableDeliveryFallbackProjectionRetry|Test.*Canonical.*Projection|Test.*Projection.*Fail' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Run `gofmt -l` and `bash scripts/lint-go-changed` for every changed Go path,
recording the exact expanded commands. Validate documentation coverage with
`.github/scripts/pr-docs.cjs`'s exported `validateCoverage`, using all changed
files including untracked work orders and all referenced spec contents. At PR
creation refresh live PR body/check evidence and follow the local template.
If adding tests in another changed package, add its targeted command here before
marking complete. Broad `/verify`, `/qa` or a second local review is not required.

## Files likely touched

- `apps/backend/internal/mcp/handlers/handlers.go` or a focused extracted
  completion helper; `step_complete_delivery_turn_test.go` (new), recovery tests.
- `apps/backend/internal/task/models/models.go` and focused pending-signal tests.
- `apps/backend/internal/task/repository/interface.go`, `sqlite/session.go` or
  focused completion claim helper; `step_completion_entry_test.go` and
  `step_completion_entry_postgres_test.go` (new). Related test doubles as needed.
- `apps/backend/internal/orchestrator/event_handlers_step_completion.go`,
  `event_handlers_workflow.go`, their focused entry/cleanup tests and new
  `agent_delivery_step_completion_test.go`. Extract narrowly to respect lint limits.
- `apps/backend/internal/task/repository/sqlite/task.go` only if the existing
  entry guard needs a narrowly compatible integration; reuse its move CAS.
- `apps/backend/internal/mcp/server/step_complete_recovery_test.go`.
- The two named public pages, specs, this plan and work orders.

## Dependencies

Task 01 complete. Keep it passing while adding completion consumers. Do not
modify agent/executor profiles or create persistent worker tasks/sessions.

## Risks

Comparing entry only in the handler is insufficient under delayed events and
moves before transition commit. A JSON field with omitempty on a numeric zero
loses the distinction between valid zero and legacy absence. Step-only duplicate
logic can mask a current-entry signal. Broad cleanup can delete its successor.
Do not turn no ledger evidence into invented history to satisfy tests.

## Parallelism

`sequential`

## Inputs

- Completion requirements 001 and 003; completion design Entry-bound completion,
  Compatibility and recovery; entry-bound attribution ADR.
- Existing `step_complete_test.go`, `step_complete_recovery_test.go`,
  `service_step_signal_test.go`, `agent_delivery_projection_test.go`,
  `agent_delivery_workflow_outcome_test.go`, and Task 01 fixtures.
- Existing `models/workflow_step_transition_guard.go`,
  `sqlite/task.go:taskStepTransitionGuardMatches` and source-scoped guard caller
  `orchestrator/event_handlers_github_pr_workflow.go`.
- Companion `step-completion-stale-turn-recovery` package is completed evidence;
  do not reset its status or claim this new path was previously covered.
- Backend AGENTS and `/tdd`; public docs authoring guidance.

## Results

Implemented entry-aware task-locked claims, exact JSON decoding, duplicate
identity, engine/legacy/subscriber/watchdog eligibility, carry/audit attribution,
conditional cleanup, and the existing workflow-entry CAS at transition commit.
Existing explicit user cancellation and question barriers retain their behavior.

Red/green regressions proved stale A -> B -> A completion acceptance, lost/invalid
entry decoding, stale entry advancement after eligibility, structural JSON cleanup,
empty metadata claim failures, and stale watchdog cancellation eligibility. Each
failed for its intended assertion before the corresponding correction and passed
afterwards. Existing fixture signals now include their actual originating entry;
no test grants completion authority using an unlocked current-step fallback.

Validation from `apps/backend` unless a root-relative command is stated:

- Required models/repository race command above: PASS, models 1.069 s and
  repository 11.980 s. Includes the legacy conditional metadata setter tests.
- Required PostgreSQL command above: PASS, 13.629 s, real PostgreSQL 17 Alpine,
  independent mover/observer/projector connections and per-test schemas. No
  PostgreSQL environment skips. Both claim and final transition wait behind a
  real mover and reject its A -> B -> A revisit; existing CAS concurrency/row
  lock tests also pass. Task 01 records the broader dialect suite.
- Required handler/server race command above: PASS, 17.351 s / 1.302 s. Tests
  join real receipt/projection to the actual completion handler for text,
  reasoning and shell, RUNNING/WAITING, replay and stale entry rejection.
- Expanded orchestrator race command above: PASS, 4.410 s. Overlapping public
  receipt/projection -> production claim -> configured engine/legacy/subscriber
  fixtures prove exactly one move, delayed/replayed signals, same-step revisit,
  final-move races, pending questions, carry/audit and successor cleanup.
- Required lifecycle race command including `Test.*Delivery.*Ack`: PASS, 7.091 s.
- `go test -race ./internal/orchestrator -timeout 15m -count=1`: PASS, 264.085 s.
  An earlier full run was interrupted after 239.631 s while diagnosing duration;
  it is not pass evidence. Its sole assertion failure was an older review-seat
  fixture missing its entry, corrected along with other manually seeded consumers.
- `python3 scripts/list-docs.py validate`: PASS, 398 decisions / 1,716 specs.
- `python3 scripts/lint-spec-files.test.py`: PASS, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: PASS.
- `node --test scripts/validate-public-docs.test.mjs`: PASS.
- `node scripts/validate-public-docs.mjs`: PASS, 54 published pages.
  Node 24.21.0 was selected from the configured mise installation.
- Documentation coverage through `.github/scripts/pr-docs.cjs:validateCoverage`:
  PASS, actual source/docs/untracked-work-order set is `covered`, both work orders.
- `git diff --check` and `gofmt -l` on every changed Go file: PASS, no output.
- Scoped Go lint: PASS, 0 issues. Initial lint identified helper
  complexity, unchecked rollback results, one conditional style and handler file
  length; addressed by focused extraction, checked defers and a test switch.

Exact scoped lint command (repo root; formatting uses `gofmt -l` with the same
paths):

```bash
bash scripts/lint-go-changed \
  apps/backend/internal/agent/runtime/lifecycle/durable_delivery_fallback_test.go \
  apps/backend/internal/mcp/handlers/step_complete_delivery_turn_test.go \
  apps/backend/internal/mcp/handlers/step_completion_entry.go \
  apps/backend/internal/orchestrator/agent_delivery_step_completion_test.go \
  apps/backend/internal/orchestrator/step_completion_entry.go \
  apps/backend/internal/task/models/step_completion_entry.go \
  apps/backend/internal/task/models/step_completion_entry_test.go \
  apps/backend/internal/task/repository/sqlite/agent_delivery_turn_step_stamp_postgres_test.go \
  apps/backend/internal/task/repository/sqlite/agent_delivery_turn_step_stamp_test.go \
  apps/backend/internal/task/repository/sqlite/step_completion_entry.go \
  apps/backend/internal/task/repository/sqlite/step_completion_entry_postgres_test.go \
  apps/backend/internal/task/repository/sqlite/step_completion_entry_test.go \
  apps/backend/internal/mcp/handlers/handlers.go \
  apps/backend/internal/mcp/handlers/step_complete_test.go \
  apps/backend/internal/mcp/handlers/step_complete_truncation_test.go \
  apps/backend/internal/orchestrator/event_handlers_step_completion.go \
  apps/backend/internal/orchestrator/event_handlers_step_completion_test.go \
  apps/backend/internal/orchestrator/event_handlers_workflow.go \
  apps/backend/internal/orchestrator/review_participant_seats_acceptance_test.go \
  apps/backend/internal/orchestrator/step_handoff_carry_write_test.go \
  apps/backend/internal/orchestrator/step_history_handoff_blockers_test.go \
  apps/backend/internal/orchestrator/step_history_test.go \
  apps/backend/internal/orchestrator/stuck_signal_watchdog.go \
  apps/backend/internal/orchestrator/stuck_signal_watchdog_cancellation_test.go \
  apps/backend/internal/orchestrator/stuck_signal_watchdog_test.go \
  apps/backend/internal/orchestrator/workflow_entry_before_advance_test.go \
  apps/backend/internal/orchestrator/workflow_store.go \
  apps/backend/internal/task/models/models.go \
  apps/backend/internal/task/repository/sqlite/agent_delivery_projection.go \
  apps/backend/internal/task/repository/sqlite/step_transitions.go
```

No browser surface changed, so screenshots and Playwright are not required.
No live agent CLI reproduction was claimed. Requirements/design/public recovery
pages describe the implemented stamp and entry contract; unrelated historical
companion work-order statuses are preserved. The original planning checkpoint
was completed before the user's later explicit implementation request.

Continue authorized commit/push/PR remediation and merge only after exact-head
CI/review gates pass. No release is authorized. Commit/PR SHAs are recorded by
the delivery artifacts rather than embedding a self-referential commit hash here.


## PR review remediation

The first published head passed CI (84 passed, 19 skipped, one neutral, zero
failed/pending). Review identified two integration gaps in the newly guarded
path: source-owned handoff updates/removals were lost during CAS rebasing, and a
legacy rejected move could leave the session running with its obsolete signal.
These findings are in scope for this work order's carry and lifecycle evidence.

Permanent regressions extend the real signal-gated engine/legacy paths for
handoff write/removal with admitted and WIP-queued destinations, and assert
legacy rejected-move session/signal settlement. Preserve only completion-owned
handoff intent during the locked CAS rebase; unrelated guarded moves must keep
the freshly read carry slot. Rejected moves clear only the observed signal and
settle the completed legacy turn through the existing waiting-state path.

Remediation red command:
`go test -race ./internal/orchestrator -run
'TestStepCompletionEntryHandoff|TestDeliveryFallbackStepCompletionMoveRace'
-count=1`: expected FAIL (1.086 s). Both engine/legacy admitted/queued handoff
write/removal cases retained the old token, and legacy state remained RUNNING.
The first green focused consumer run passed in 3.723 s.

Repository remediation command:
`KANDEV_TEST_POSTGRES_DSN=<disposable PostgreSQL 17 DSN> go test -race -v
./internal/task/repository/sqlite -run
'Test.*StepCompletionEntry|TestPostgresUpdateTaskWithWorkflowStepAdmissionIfAtStep'
-count=1`: PASS, 16.011 s. Includes SQLite/PostgreSQL completion-owned updates,
removals, unrelated guard behavior and concurrent metadata preservation.

Final consumer command:
`go test -race ./internal/orchestrator -run
'TestStepCompletionEntryHandoff|TestDeliveryFallbackStepCompletion|Test.*Handoff|Test.*StepHistory|Test.*StepCompletion'
-count=1`: PASS, 3.137 s, including successor-signal preservation.
Scoped lint uses normal commit hooks on all remediation Go paths. Specification
catalog/lint and whitespace checks pass after the design update.

Exact-head delivery and current-base synthetic validation remain pending. Initial CI evidence is
historical after a new push; final CI/review must be rerun. Main independently
advanced to `12bc3f1a38d91a40f787ceaa2cda66e9f88915b6` (bounded tool-display data).
Its paths are disjoint; final delivery requires synthetic merge validation of
canonical payload/turn/handler consumers against that authoritative base.

The Greptile handoff finding and CodeRabbit aggregate lifecycle finding are
addressed by these changes and tests. The latter is an aggregate review body,
not an unresolved inline thread. No extra local review agent was launched.

The first remediation commit attempt stopped at Go lint for one test-helper
if/else chain. Rewritten as a switch; targeted SQLite/real PostgreSQL race
command `go test -race ./internal/task/repository/sqlite -run
'Test.*StepCompletionEntryHandoffRebase' -count=1` passed (8.552 s) with the
real disposable DSN. Retry uses normal hooks with no bypass.
