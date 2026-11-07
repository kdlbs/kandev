---
id: "01-replay-uncommitted-successor"
title: "Replay an uncommitted fresh-start successor safely"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.4
system_design:
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
  - ../../specs/agents/system-design/agent-resume-runtime-recovery.md
---

# Task 01: Replay an uncommitted fresh-start successor safely

## Summary

Allow authenticated retained setup events from an uncommitted fresh-start successor to pass initial delivery preflight.
Keep receipt, projection, lifecycle effects, and acknowledgment behind the existing successor-generation commit and ownership checks.

## In scope

- Write `TestDeliveryPreflightUncommittedFreshStartSuccessor` before changing production code.
  Use the real SQLite fixture with selected generations 2 and 10 and committed predecessors 1 and 9.
  Use an empty native ID, explicit replacement intent, absent selected-stream cursor, and fully retained unacknowledged startup events.
  Keep `deliveryBootstrap` false, as real startup does for later generations.
  Include fresh and resumed execution flags and the existing generation-one positive control.
- Run the new regression and record the expected `agent delivery checkpoint missing` failure.
- Extend `canReplayFreshBootstrapEvents` with the two authority cases in the system design.
  Preserve common guards and reuse `replacementGenerationPending` for later-generation adjacency.
  Require a positive predecessor and refuse generation-reader errors or unsupported readers.
- Write `TestDeliveryPreflightUncommittedFreshStartRefusesUnsafeState`.
  Cover absent or false replacement intent, no predecessor, a non-adjacent predecessor, an already committed selected generation, and foreign identities.
  Also cover native IDs, required native IDs, nonzero acknowledgments, a pruned prefix, submissions, truncated summaries, and unavailable storage.
  A stray g10 stream with committed g1 must be refused.
  Existing SQL cursor cases continue through ordinary authenticated cursor validation rather than this exception.
- Write `TestFreshStartBootstrapRegisteredJournalRecovery` with registered agentctl routes, a real journal, and real SQLite storage.
  Adapt `fresh_start_journal_integration_test.go` and `fresh_start_sql_wait_test.go` rather than building a parallel test harness.
  Retain a projected predecessor stream and completed historical submission evidence alongside an uncommitted successor.
  Append a setup event and an old manager's exit error, stop the owned peer, then reopen the same journal with a replacement manager.
  Prove that the successor is selected again without advancing another generation.
  Read the descriptor through the real endpoint, including `Unresolved == false` and zero recovery-relevant submission summaries.
  Run preflight and queue retained events before committing the successor.
  Use barriers to prove no successor receipt, projection, callback, or ACK precedes the commit.
  Commit through `CommitHarnessSessionGenerationForExecution` with the replacement native token and execution.
  Then prove ordered idempotent replay and ACK, unchanged predecessor history, and retirement of the old manager's exit error.
  Count provider dispatches and assert that replay dispatches no instruction.
- Write `TestFreshStartBootstrapCancellationLeavesUncommittedStream`.
  Cover cancellation, failed or rejected successor commit, and a subsequent eligible retry of the same retained stream.
  Preserve retained events and predecessor authority without manufacturing a cursor or acknowledging output.
  Keep the existing stale-owner and delayed-predecessor controls.

## Out of scope

- Changes to `session.go`, bootstrap flag meaning, `awaitSilentRestoreGeneration`, or generation selection.
- Changes to `resumed_delivery_checkpoint.go`, native token clearing, submission retirement rules, or legacy capability semantics.
- Provider startup repair, issue #5128 error presentation, frontend markup, locale catalogs, schema, journal format, or manual data repair.

If the evidence requires one of these changes, record the design conflict before expanding this work order.

## Acceptance

1. A verified uncommitted fresh-start successor passes preflight at g2 and g10 without creating a SQL checkpoint.
   The regression fails on the unchanged implementation and passes after the correction.
2. Missing, conflicting, committed, pruned, or submission-bearing evidence cannot use the exception.
   First-generation bootstrap, silent restore, committed-native checkpoint recovery, and positive legacy identification keep their current behavior.
3. The registered-journal recovery sequence admits each retained successor event only after its current execution commits the successor.
   Cancellation and rejected commits retain evidence. Replay preserves predecessor history and never redispatches a prompt.

## Verification

Run the first command before the production edit and record its expected failure.
Run the full block after implementation, from the repository root.

```bash
(cd apps/backend && go test -trimpath -race -count=1 ./internal/agent/runtime/lifecycle -run '^TestDeliveryPreflightUncommittedFreshStartSuccessor$')
(cd apps/backend && go test -trimpath -race -count=1 ./internal/agent/runtime/lifecycle -run 'Test(DeliveryPreflight|ResumedDeliveryCheckpoint|FreshStart|BootstrapReplay|SilentRestore|DeliveryRetention)')
(bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/delivery_stream_preflight.go apps/backend/internal/agent/runtime/lifecycle/delivery_stream_successor_bootstrap_preflight_test.go apps/backend/internal/agent/runtime/lifecycle/fresh_start_bootstrap_recovery_test.go)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run this documentation preflight from the repository root.
It includes the intended production path so the validator checks the complete references.

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const plan = 'docs/plans/fresh-start-bootstrap-replay/plan.md';
const work = 'docs/plans/fresh-start-bootstrap-replay/task-01-replay-uncommitted-successor.md';
const design = 'docs/specs/platform/system-design/durable-agent-stream-processing.md';
const source = 'apps/backend/internal/agent/runtime/lifecycle/delivery_stream_preflight.go';
const paths = [plan, work, design, source,
  'docs/specs/agents/requirements/agent-resume-runtime-recovery.md',
  'docs/specs/agents/system-design/agent-resume-runtime-recovery.md',
  'docs/specs/platform/requirements/durable-agent-delivery.md'];
const result = validateCoverage({
  changedFiles: [
    { filename: plan, status: 'added' },
    { filename: work, status: 'added' },
    { filename: design, status: 'modified' },
    { filename: source, status: 'modified' }],
  fileContents: Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')]))
});
console.log(JSON.stringify({ ok: result.ok, status: result.status, errors: result.errors }, null, 2));
if (!result.ok) process.exit(1);
NODE
```

Record actual command results in this file and the plan.
No database method or dialect-sensitive query change is planned, so new PostgreSQL infrastructure is not required.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/delivery_stream_preflight.go`
- `apps/backend/internal/agent/runtime/lifecycle/delivery_stream_successor_bootstrap_preflight_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/fresh_start_bootstrap_recovery_test.go` (new)
- `docs/plans/fresh-start-bootstrap-replay/plan.md` (status and evidence)
- `docs/plans/fresh-start-bootstrap-replay/task-01-replay-uncommitted-successor.md` (status and evidence)

## Dependencies

None. The generation-one relaunch and fresh-start generation companion packages are already implemented in the investigation snapshot.

## Risks

- A fixture that encodes a journal descriptor directly can differ from the registered API's capability projection.
  The real endpoint is required integration evidence.
- The selected stream must be adjacent to the current SQL generation.
  Finding one apparently usable stream among unrelated retained streams is insufficient.
- Replayed predecessor-manager exits must not fail the replacement child, including equal process-generation counters.
- A generation commit can race preflight or event replay.
  The existing execution-fenced SQL admission remains the authority after preflight.

## Parallelism

`sequential`

## Inputs

- [Plan](plan.md), especially the evidence, compatibility matrix, and exclusions.
- [Stream processing](../../specs/platform/system-design/durable-agent-stream-processing.md#uncommitted-fresh-start-successor-relaunch).
- [Agent recovery requirements](../../specs/agents/requirements/agent-resume-runtime-recovery.md), criterion 001.2.
- [Durable delivery requirements](../../specs/platform/requirements/durable-agent-delivery.md), criteria 003.2, 005.1, 005.2, and 007.4.
- [Agent recovery design](../../specs/agents/system-design/agent-resume-runtime-recovery.md#session-identity).
- Existing `delivery_stream_bootstrap_preflight_test.go`, `delivery_stream_bootstrap_replay_test.go`, `fresh_start_journal_integration_test.go`, and `fresh_start_sql_wait_test.go`.
- `durable_silent_restore_generation.go`, especially `replacementGenerationPending` and `awaitSilentRestoreGeneration`.

## Results

Completed on 2026-11-07.

- `TestDeliveryPreflightUncommittedFreshStartSuccessor` failed on unchanged
  production code with `agent delivery checkpoint missing` for selected
  generations 2 and 10, in both fresh and resumed execution shapes. Generation
  one passed as the positive control.
- `TestDeliveryPreflightUncommittedFreshStartRefusesUnsafeState` covers absent
  replacement intent, missing, non-adjacent, already committed, and foreign
  predecessors, native identities, the generation-one bootstrap flag on a
  successor, acknowledgement, pruned history, submission evidence,
  authority-read failures, unsupported readers, and unavailable storage. Every
  case leaves no successor cursor.
- `TestFreshStartBootstrapRegisteredJournalRecovery` reopens a real bbolt
  journal through registered agentctl routes and real SQLite admission. It
  proves preflight replay, successor-commit barriers for receipt, projection,
  lifecycle callback, and acknowledgement, ordered idempotent replay, unchanged
  predecessor history, retirement of the old manager's exit error, and zero
  provider dispatches.
- `TestFreshStartBootstrapCancellationLeavesUncommittedStream` covers
  cancellation, a rejected execution-token commit, a failed SQL insert, and a
  later eligible retry of the same retained stream. It also keeps the stale-owner
  and delayed-predecessor controls.
- `bash scripts/lint-go-changed <the changed Go files>`: 0 issues.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.test.py`,
  `python3 scripts/lint-spec-files.py --all`, `git diff --check`, and the
  `validateCoverage` preflight all passed. Exact command results are in the plan.

Fixup follow-up:

- CodeRabbit identified a retained-terminal prompt-attempt collision. The
  submission-identity fence now prevents a replayed terminal event from
  completing the live prompt while preserving receipt, projection, settlement,
  and ACK. `TestRecoveredTerminalSubmissionDoesNotCompleteLivePrompt`,
  `TestRecoveredTerminalSubmissionMatchingLivePromptCompletes`, and
  `TestClaimPromptCompletionRejectsSupersededSubmission` cover the behavior.
- The retained-status resume-token finding was already addressed by the
  orchestrator's stale ACP-session guard in `storeResumeToken`.
- The full lifecycle package and targeted race suites passed again; lint
  reported 0 issues. Exact results are in the plan.
