---
created: 2026-11-10
status: done
requirements:
  - REQ-TASKS-COMPLETION-OUTCOMES-001
  - REQ-TASKS-COMPLETION-OUTCOMES-002
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003
  - REQ-TASKS-SIGNAL-PAYLOAD-DELIVERY-001
  - REQ-TASKS-SIGNAL-PAYLOAD-DELIVERY-002
  - REQ-TASKS-SIGNAL-PAYLOAD-DELIVERY-003
system_design:
  - ../../specs/tasks/system-design/workflow-completion-outcomes.md
  - ../../specs/tasks/system-design/workflow-explicit-completion-signal.md
  - ../../specs/tasks/system-design/workflow-completion-signal-payload-delivery.md
legacy_specs: []
---

# Fix plan: source-specific workflow completion

## Overview

Resolve [issue #5173](https://github.com/kdlbs/kandev/issues/5173) with source-specific signal claims and retained transition receipts.
First implement admission and duplicate identity.
Then bind transition proof and destination handoff to that identity.
Both work orders run sequentially in the primary session.

This package is implemented.
Issue #5173 is assigned to the authenticated GitHub user, `carlosflorencio`.
All package files remain unstaged and uncommitted.

## Evidence and root cause

Investigation checkout: `361d0dbd641`, on 2026-11-10.
The canonical issue is open, with no comments or image attachments at investigation time.
It reports stock `0.99.1` (`813a741b`).
The reporter's installation and original transition logs were not available here.

The task state `REVIEW` means idle reviewable work.
It does not establish that the task entered the workflow step named Review.
`writeTaskReviewState` can set that state while the workflow pointer remains in Build.
This is also possible after a valid transition refusal or with no configured move.
The report does not establish which condition prevented its first move.

The reproducible defect is narrower and certain:

1. `stepCompletionLaunchStep` reads the latest turn's step and entry, but returns no turn identity.
2. `PendingStepCompletionSignal` contains no source-turn identity.
3. `ClaimStepCompletionSignal` retains the first payload for a whole entry.
4. User-prompt admission clears the pending signal, but native output projection can create a later turn without that path.
5. The later completion receives `already_signaled`, keeps the original handoff, and sends no new wakeup while RUNNING.
6. The duplicate result contains neither the stored signal identity nor the actual destination.

### Temporary reproductions

A throwaway `apps/backend/internal/mcp/handlers/repro_5173_test.go` used the real SQLite repository and completion handler.
`TestRepro5173NewNativeCorrection` first accepted a Build signal, then closed its source turn.
It received and projected a canonical agent-output event through `ReceiveAgentDeliveryEvent` and `ProjectCanonicalAgentDeliveryEvent`.
Projection created `agent-delivery-turn-correction-stream-1` in the same entry.
The next completion returned `{accepted:false, reason:already_signaled}`.
The stored handoff remained `old review handoff`, rather than `latest review handoff`.
Only one bus event existed after both calls.

`TestRepro5173DuplicateReceipt` called completion twice in one turn.
The duplicate response contained only `accepted` and `reason`.
Assertions for `step_id`, `signaled_at`, and `current` failed.

Both tests failed for the expected behavioral reasons:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/mcp/handlers -run '^TestRepro5173' -count=1 -v)
```

The first experiment used two stamped turns directly.
The second strengthened the reproduction with real canonical output projection.
The temporary file was removed after evidence capture.
No production or permanent test change remains.

## Intent and contract reconciliation

The tasks system owns this repair because it owns completion admission and durable workflow routing.
Platform delivery supplies turn identity but does not own step advancement.
Review isolation remains with the existing profile-session routing contract.

The assumption check found no material product question that blocks planning.
The issue explicitly requires duplicate safety, later-source handoff, structured outcomes, executor authorization, and independent Review.
Routine design choices follow those constraints.

This is a contract amendment, rather than a claim that existing code violates every old criterion.
Payload-delivery AC-001.13 explicitly selected the first signal for the whole step.
That scope becomes the source turn, with replacement only before transition commitment.
AC-002.8 follows the winning signal, and AC-003.5a permits additive outcome fields on duplicates.

The new requirements are `REQ-TASKS-COMPLETION-OUTCOMES-001` and `-002`.
The [outcomes design](../../specs/tasks/system-design/workflow-completion-outcomes.md) and
[accepted ADR](../../decisions/2026-11-10-source-specific-step-completion.md) define identity, transaction proof, and compatibility.

Existing packages remain completed implementation records:

- [Agent-woken turn attribution](../agent-delivery-turn-step-stamp/plan.md): preserves entry stamps and A-to-B-to-A rejection.
- [Stale-turn recovery](../step-completion-stale-turn-recovery/plan.md): preserves the fresh-turn recovery instruction.
- [Dynamic workflow completion](../dynamic-workflow-completion/plan.md): preserves destination preflight and recoverable source sessions.

Those manifests were inspected. Their completed scopes and recorded results are not reopened by this proposal.

## Scope

### In scope

- Source-turn identity, same-source deduplication, and replacement of an older pending signal.
- One retained receipt per source session, with additive result fields.
- Exact-signal and entry checks at the transition transaction.
- Matching receipt, carry, and audit provenance across legacy and engine paths.
- Service-level destination prompt and Review/Iterate/Review regression coverage.
- Completion-tool guidance and public recovery documentation with implementation.

### Out of scope

- Changes to executor or credential authorization, including issues #5167 and #5172.
- Automatic reopening after a committed move or injection into an active Review conversation.
- A guarantee of prompt delivery after startup or carry-claim failure.
- New workflow configuration, rendered UI, feature flags, or a fallback button.
- Historical signal reconstruction, new tables, or complete receipt history.

## Technical approach

### Admission and response

Extend `mcp/handlers/step_completion_entry.go` and `sqlite/step_completion_entry.go` with source identity and atomic receipt claims.
Add typed metadata in `task/models/step_completion_outcomes.go` and source fields in `PendingStepCompletionSignal`.
Keep handler changes focused in `step_complete_outcomes.go` rather than enlarging `handlers.go`.
Use the existing backend-authenticated task/session binding.

Keep a bounded `last_step_completion_receipt` key on source-session metadata.
Duplicates return the existing receipt, including after bag cleanup.
Do not trust summary differences, client timestamps, or agent-supplied identity.
No-turn compatibility remains explicit and cannot fabricate transition evidence.

### Transition and destination

Extend the completion guard and `taskStepTransitionGuardMatches` with exact signal identity.
Use the task transaction already shared by move admission, step ledger writes, and carry metadata.
Persist the matching receipt's transition ID in that transaction.
Keep asynchronous history separate from proof.

Wire both consuming paths in `event_handlers_workflow.go` and the carry path in `workflow_store.go`.
Reject an older prepared signal after a replacement wins.
Condition every cleanup and blocked-result update on the signal it observed.
Reconcile the replacement after releasing guards so a lost wakeup cannot strand it.

Keep destination credential preflight, WIP admission, profile selection, session switching, and independent Review policies intact.
The result reports a committed step separately from actual reviewer dispatch.

### Compatibility and evidence

| Path | Identity | Intended behavior | Evidence |
| --- | --- | --- | --- |
| Ordinary Kandev prompt | Authenticated session, stamped turn and entry | Same-source deduplication, normal reopening | Handler and repository tests |
| Native canonical output | Persisted fallback turn and entry | Later completion replaces an older pending source | Canonical projection regression |
| ACP or passthrough | Same backend claim and step routing | Configured Review recipient receives the winning handoff | Service acceptance cases for both transports |
| No persisted turn | Explicit compatibility identity | Existing entry-wide deduplication, unproven history | Compatibility test |
| Stale entry or foreign task | Rejected evidence | No move, receipt disclosure, or prompt | Entry and authorization regressions |

## Tests

Permanent names below are the implemented regressions.

| Criteria | File and test target |
| --- | --- |
| OUTCOMES-001.1-.3, .6, payload AC-001.13 | `mcp/handlers/step_complete_outcomes_test.go`: `TestHandleStepComplete_NewerNativeTurnReplacesPendingSignal`, `TestHandleStepComplete_SameTurnKeepsFirstPayload`, `TestHandleStepComplete_NativeVerificationKeepsHandoff` |
| OUTCOMES-001.3-.5, .7 | `task/repository/sqlite/step_completion_outcomes_test.go`: `TestStepCompletionOutcome_ReplacementWinsCommitRace`, `TestStepCompletionOutcome_MoveAndReceiptRollback`, `TestStepCompletionOutcome_ReentryRejectsOldSource` |
| OUTCOMES-002.1-.7, explicit AC-003.1-.7 | `mcp/handlers/step_complete_outcomes_test.go`: `TestHandleStepComplete_DuplicateOutcome`, `TestHandleStepComplete_ReceiptAfterApplied`, `TestHandleStepComplete_ReceiptDeniedToForeignCaller`, `TestHandleStepComplete_LegacyOutcomeUnknown` |
| OUTCOMES-001.3-.5, .8, payload AC-002.8 | `orchestrator/step_completion_outcomes_test.go`: `TestStepCompletionOutcome_LegacyAndEngine`, `TestStepCompletionOutcome_NativeVerificationKeepsAcceptedSource`, `TestStepCompletionOutcome_SupersededConsumerReconcilesLatest`, `TestStepCompletionOutcome_ProofFailureRetriesSameSignal`, `TestStepCompletionOutcome_PreflightRefusalRecordsBlocked`; recipient/prompt acceptance in `step_completion_outcomes_acceptance_test.go` |
| OUTCOMES-002.2, .4-.7, payload AC-003.5a | Handler `TestHandleStepComplete_CurrentSnapshotAfterPublishMove`, `TestHandleStepComplete_CurrentSnapshotSurvivesProjectionMove`, `TestHandleStepComplete_ReceiptAfterDestinationEntry`, duplicate/foreign/legacy cases, MCP serialization, sysprompt, and tool-description tests |
| Dialect-sensitive claim and commit logic | `task/repository/sqlite/step_completion_outcomes_postgres_test.go`: environment-gated `TestPostgresStepCompletionOutcome_*` cases |

Use deterministic barriers for races.
Include mixed sessions with a valid current signal and a stale or foreign signal.
The predicate is per originating source, and at most one transition leaves the entry across all competing sources.

## End-to-end evidence

No rendered UI changes, so this package requires no Playwright scenario or UI preview.
Service acceptance coverage uses the real repository, engine reconciliation, profile routing, and the mock runtime prompt recorder in `orchestrator/step_completion_outcomes_acceptance_test.go`:

- `TestStepCompletionOutcome_ReviewRecipientGetsLatestHandoffOnce`: a newer correction wins, the configured Review session (not the source session) receives exactly one prompt, and that prompt carries only the latest handoff.
- `TestStepCompletionOutcome_RequestChangesIterateReviewIgnoresStaleEvent`: Review, a manual request-changes move into Iterate, and a completion return to Review reach the same configured recipient twice; a delayed earlier-entry event changes nothing.
- `TestStepCompletionOutcome_QueuedReviewAndPromotion`: a full Review destination queues the committed move without a prompt, then promotion after repository reload delivers the winning handoff once to the Review recipient.
- `TestStepCompletionOutcome_CredentialRefusalKeepsSourceRoute`: a real destination credential preflight refusal leaves the source session primary and retryable, records a blocked outcome, and starts no destination prompt.

`TestStepCompletionOutcome_LegacyAndEngine`, `NativeVerificationKeepsAcceptedSource`, `SupersededConsumerReconcilesLatest`, `ProofFailureRetriesSameSignal`, and `PreflightRefusalRecordsBlocked` cover the remaining repository and retry boundaries.

## Work orders

- [x] [Task 01: Bind signal claims to their source](task-01-source-claims.md) (`done`)
- [x] [Task 02: Commit outcome proof and preserve Review handoff](task-02-transition-proof.md) (`done`, depends on Task 01)

## Verification results

Implementation complete:

- Source identity, source-turn deduplication, replacement, bounded receipts, commit-proof fencing, receipt projection, and supersession reconciliation are implemented.
- Handler regressions passed: `go test -trimpath -tags fts5 ./internal/mcp/handlers -run '^TestHandleStepComplete_' -count=1`, including later-native replacement, same-turn payload retention, native verification neutrality, single-snapshot current projection across publication and projection moves, committed-receipt reopen after destination entry, unconfigured projection, foreign-caller denial, and legacy unproven outcomes.
- Service acceptance passed: `go test -trimpath -tags fts5 ./internal/orchestrator -run '^TestStepCompletionOutcome_' -count=1`, including configured Review recipient prompt text, Review/Iterate/Review stale events, queued promotion after repository reload, and real credential refusal without source substitution.
- Transient proof failure regression passed: the same durable signal moves once on retry without a newer session generation, and records one receipt and handoff.
- Repository and model regressions passed under race: `go test -trimpath -tags fts5 -race ./internal/task/models ./internal/task/repository/sqlite ./internal/mcp/handlers -run 'Test(StepCompletionOutcome|PostgresStepCompletionOutcome|StepCompletionEntry|HandleStepComplete_)' -count=1`.
- Full affected packages passed: `go test -trimpath -tags fts5 ./internal/task/models ./internal/task/repository/sqlite ./internal/mcp/handlers ./internal/mcp/server ./internal/sysprompt -count=1` (SQLite 473s, handlers 73s).
- Full orchestrator package passed: `go test -trimpath -tags fts5 ./internal/orchestrator -count=1` (242s).
- Orchestrator race subset passed: `go test -trimpath -tags fts5 -race ./internal/orchestrator -run 'Test(StepCompletionOutcome_|StepCompletionEntryHandoff|WorkflowCredentialPreflightFailureDispatchesFollowUp)' -count=1`.
- `go run -trimpath ./cmd/sqlguard ./internal` passed.
- `go test -trimpath -tags fts5 -race ./internal/persistence/storeconformance -count=1` passed (49s).
- `bash scripts/lint-go-changed` over every changed Go file passed with 0 issues.
- `gofmt -l` over the affected packages reported no files.
- PostgreSQL behavior cases are implemented in `step_completion_outcomes_postgres_test.go` and `step_completion_entry_postgres_test.go`; `KANDEV_TEST_POSTGRES_DSN` was unset, so database coverage is recorded as skipped.
- Public documentation now reflects the shipped result semantics in `automation-and-mcp.md` and `workflow-tips.md`.
- Public-docs validation and the plan documentation preflight are recorded in Task 02 results.

All files remain unstaged and uncommitted.

## Documentation preflight

Run this block from the repository root.
It exercises cross-reference coverage against the actual implementation diff.

```bash
node <<'NODE'
const fs = require('node:fs');
const cp = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const packagePath = 'docs/plans/step-completion-outcomes';
const changed = [...new Set([
  ...cp.execFileSync('git', ['diff', 'HEAD', '--name-only'], { encoding: 'utf8' }).trim().split('\n'),
  ...cp.execFileSync('git', ['ls-files', '--others', '--exclude-standard'], { encoding: 'utf8' }).trim().split('\n'),
  'apps/backend/internal/mcp/handlers/step_completion_entry.go',
])].filter(Boolean);
const documents = [
  ...fs.readdirSync(packagePath).filter(p => p.endsWith('.md')).map(p => `${packagePath}/${p}`),
  ...['requirements', 'system-design'].flatMap(kind => [
    'workflow-completion-outcomes', 'workflow-explicit-completion-signal',
    'workflow-completion-signal-payload-delivery',
  ].map(name => `docs/specs/tasks/${kind}/${name}.md`)),
];
const fileContents = Object.fromEntries(documents.map(p => [p, fs.readFileSync(p, 'utf8')]));
const result = validateCoverage({
  changedFiles: changed.map(filename => ({ filename, status: 'modified' })),
  fileContents,
});
console.log(JSON.stringify({ ok: result.ok, status: result.status, workOrders: result.workOrders, errors: result.errors }, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
```

## Risks

- The original installation's first blocked transition remains unverified. This package proves and repairs the independent deduplication gap on current code.
- New source identity must use persisted turn evidence and preserve task/session lock order on PostgreSQL.
- A receipt cannot certify prompt delivery. Destination startup and carry-claim errors retain their existing recovery contract.
- The old payload specification stated entry-wide first-wins behavior. This package explicitly amends that scope rather than silently replacing it.
- Environment-gated PostgreSQL tests require `KANDEV_TEST_POSTGRES_DSN`. A skipped database test is not passed database coverage.
