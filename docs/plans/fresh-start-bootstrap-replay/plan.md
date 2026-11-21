---
created: 2026-11-07
status: implemented
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
system_design:
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
  - ../../specs/agents/system-design/agent-resume-runtime-recovery.md
legacy_specs: []
---

# Fix plan: recover an uncommitted fresh-start successor

## Overview

[Issue #5129](https://github.com/kdlbs/kandev/issues/5129) reports a permanent
resume failure after a fresh-start bootstrap fails at generation 2 or later.
The backend refuses the retained successor stream with `agent delivery checkpoint missing`.
One work order extends bootstrap preflight and proves subsequent admission through the existing successor commit wait.

Platform owns this repair because it owns authenticated delivery evidence and event admission.
The existing agent recovery contract defines the observable fresh-start outcome.
No new requirement, recovery action, or conversation-replacement permission is needed.

## Evidence and root cause

Investigation used repository head `5264536ba98` on 2026-11-07.
The canonical issue has one text comment linking [issue #5128](https://github.com/kdlbs/kandev/issues/5128).
It has no image attachments. The reported live journal and database were not available in this workspace.

Two temporary tests reproduced the failure through `StreamManager.initialDeliveryReplayCursor`.
Both used the real task SQLite repository and the existing preflight fixture.
The second used a real bbolt journal, process manager, and registered agentctl HTTP routes.
It appended an `agent_capabilities` event before native initialization.

| Selected generation | Committed SQL generation | Bootstrap flag | Replacement intent | Actual result |
| --- | --- | --- | --- | --- |
| 1 | Absent | true | false | Cursor 0, success, no fabricated SQL cursor |
| 2 | 1 | false | true | `agent delivery checkpoint missing` |
| 10 | 9 | false | true | `agent delivery checkpoint missing` |

Both commands exited 1 because generations 2 and 10 failed their expected-success assertions.
Generation 1 passed in both commands. Each test body took approximately 0.4 seconds after compilation.
The temporary files were removed after investigation.

```bash
(cd apps/backend && go test -trimpath -count=1 ./internal/agent/runtime/lifecycle -run '^TestReproIssue5129UncommittedFreshStart$' -v)
(cd apps/backend && go test -trimpath -count=1 ./internal/agent/runtime/lifecycle -run '^TestReproIssue5129RegisteredBootstrap$' -v)
```

These are historical reproduction commands. Their throwaway test file is not part of this package.
The work order names the permanent regressions to create during implementation.

The failure path is:

1. `Executor.applyDeliveryIdentity` selects committed generation N plus one for a fresh-start replacement.
   A failed uncommitted attempt leaves N current, so another attempt reuses the N+1 stream.
2. `SessionManager.InitializeAndPromptWithLayers` sets `deliveryBootstrap` only for generation 1.
   Later fresh starts correctly use the separate successor wait.
3. `authenticatedDeliveryReplayCursor` sees retained events without a SQL cursor.
   `canReplayFreshBootstrapEvents` requires generation 1, its bootstrap flag, and no SQL generation.
   `canReplaySilentRestoreSetup` requires an initialized, pinned native conversation.
4. The fallback `receiveResumedBootstrapCheckpoint` requires a committed generation-one native identity.
   An uncommitted fresh-start successor with an empty native ID satisfies none of these paths.

The registered API reports `Unresolved == false` for retained setup events without unsettled prompts.
`process.Manager.DeliveryRecoveryDescriptor` replaces the journal's aggregate storage capability with `DeliveryCapability`.
The latter checks unresolved submissions. Preserve this guard and test the actual HTTP response.
The bounded submission summaries cover recovery-relevant work across the session, including predecessors.
Completed or explicitly retired historical records do not require an empty physical submissions bucket.

## Requirement and design conformance

Reuse these existing criteria:

- [AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.2](../../specs/agents/requirements/agent-resume-runtime-recovery.md): explicit fresh-start conversation replacement.
- [AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.2](../../specs/platform/requirements/durable-agent-delivery.md): uncertain submissions cannot be resent.
- [AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.1 and .2](../../specs/platform/requirements/durable-agent-delivery.md): durable receipt, idempotent projection, and stale-owner refusal.
- [AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.4](../../specs/platform/requirements/durable-agent-delivery.md): storage failure cannot authorize legacy fallback.

This is an implementation gap in existing fresh-start behavior.
The [stream-processing design](../../specs/platform/system-design/durable-agent-stream-processing.md#uncommitted-fresh-start-successor-relaunch)
now states the successor exception and its authority boundaries.
The [agent recovery design](../../specs/agents/system-design/agent-resume-runtime-recovery.md#session-identity)
continues to own native identity and explicit replacement permission.

Existing decisions remain authoritative:
[harness persistence boundaries](../../decisions/2026-09-10-durable-harness-session-boundaries.md)
and [retirement checkpoints](../../decisions/2026-10-05-backend-delivery-retirement-checkpoints.md).
This repair extends the existing bootstrap exception without creating a new storage authority.
The system design preserves sufficient rationale, so no new ADR is needed.

The implemented [generation-one relaunch package](../uncommitted-bootstrap-relaunch/plan.md)
and [fresh-start generation package](../fresh-start-delivery-generation/plan.md) are companion records.
Their completed work stays historical. This pending package joins their previously uncovered boundary.
Their relevant regression families appear in the work order's exact verification commands.

## Scope

### In scope

- Permit zero-based replay for an explicitly marked, adjacent, uncommitted fresh-start successor with verified delivery evidence.
- Preserve generation-one eligibility, silent restore, committed-native checkpoint recovery, and all existing unsafe-state refusals.
- Prove that replay waits for the successor commit before receipt, projection, lifecycle callbacks, or ACK.
- Cover repeated bootstrap failure, predecessor history, stale process exits, and cancellation with backend integration evidence.

### Out of scope

- Repairing provider startup failures, untrusted `mise.toml`, or the diagnostic-loss problem in issue #5128.
- Rewriting or deleting journal streams, synthesizing checkpoints, or modifying a user's live data.
- Changing generation selection, resume tokens, successor deadlines, SQL schema, protocol versions, metrics, or feature flags.
- Generalizing committed-native checkpoint recovery to arbitrary generations.
- New UI copy, recovery controls, or error classifications.

## Technical approach

Change `canReplayFreshBootstrapEvents` in
`apps/backend/internal/agent/runtime/lifecycle/delivery_stream_preflight.go`.
Keep the common stream, native-identity, cursor, and submission safeguards.
After the existing generation reader resolves SQL authority, distinguish two cases:

- Generation 1 retains its bootstrap flag and absent-generation requirement.
- A later generation requires `ReplacesNativeConversation` and a positive adjacent SQL predecessor for the same session and incarnation.
  Reuse `replacementGenerationPending` for the ownership and adjacency predicate.
  A missing predecessor, an already committed selected generation, or a failed read refuses the exception.

Keep `deliveryBootstrap` false for later generations.
Changing that flag alone selects `awaitBootstrapGeneration`, which can return the already committed predecessor immediately.
The existing `awaitSilentRestoreGeneration` waits for the actual successor through its bound owner and two-minute limit.
SQL admission then fences the execution and generation again.

Preflight returns cursor 0 without database writes, projection, acknowledgments, or provider RPCs.
The normal startup and stream processor perform those operations under their existing authorities.
Keep manager identity fencing for retained errors from an earlier producer.

Compatibility scope:

| Provider or transport | Required evidence | Intended behavior | Verification or fallback |
| --- | --- | --- | --- |
| Claude ACP, reported configuration | Compatible durable peer, explicit fresh-start successor, complete prefix, no unsettled submissions | Replay eligible successor setup events | Real journal/HTTP/lifecycle reproduction and regression. No claim of a live Claude subprocess test. |
| Other providers using the same durable lifecycle | The same authenticated descriptor and launch shape | Use the shared eligibility rule | Provider-neutral integration evidence. Unsupported identity shapes remain blocked. |
| Generation-one fresh bootstrap | Existing bootstrap flag, absent generation, existing common guards | Existing replay behavior | Existing `TestDeliveryPreflight*` and bootstrap replay regressions |
| Pinned silent native restore | Existing initialized native identity and restore evidence | Existing restore path | Existing `TestSilentRestore*` regressions |
| Committed native session with a missing cursor | Existing checkpoint-recovery evidence | Existing dedicated recovery or refusal | `TestResumedDeliveryCheckpointReceivesAuthenticatedFirstEvent` |
| Legacy, unavailable, or mismatched peer | Existing positive compatibility rules | Existing legacy handling or typed refusal | `TestDeliveryPreflightRequiresPositiveCompatibilityEvidence` |

The exception concerns exactly the selected stream and its immediate SQL predecessor.
It does not search for any recoverable stream or require all historical streams to be empty.
A stray `:g10` stream beside committed g1 cannot authorize a g10 launch.
Unresolved submission evidence from any generation still blocks this exception.

## Tests

| Acceptance criterion | Permanent regression evidence |
| --- | --- |
| AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.2 | `delivery_stream_successor_bootstrap_preflight_test.go`: `TestDeliveryPreflightUncommittedFreshStartSuccessor`, with g2/g10 and first-generation controls |
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.2, AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.4 | Same file: `TestDeliveryPreflightUncommittedFreshStartRefusesUnsafeState`, including submissions, unknown storage, authority-read failures, and identity changes |
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.1 and .2 | `fresh_start_bootstrap_recovery_test.go`: `TestFreshStartBootstrapRegisteredJournalRecovery` and `TestFreshStartBootstrapCancellationLeavesUncommittedStream` |
| Existing generation and checkpoint contracts | Existing `TestDeliveryPreflight*`, `TestResumedDeliveryCheckpoint*`, `TestFreshStart*`, `TestBootstrapReplay*`, `TestSilentRestore*`, and `TestDeliveryRetention*` families |

The positive preflight regression must fail on unchanged production code with the reported missing-checkpoint error.
Negative cases must prove refusal without a successor receipt or ACK.
The integration sequence must retain a projected predecessor stream while recovering its uncommitted successor.

## E2E tests

Backend end-to-end evidence uses registered agentctl routes, a real retained journal, and real SQLite admission and projection.
It spans retained bootstrap events, process replacement, preflight, successor commit, ordered replay, and acknowledgment.
Count provider dispatches to prove that replay never resends an instruction.

No rendered UI changes are planned. Browser-only tests cannot establish the journal/SQL authority boundary and are not required for this package.
The reported native provider and trigger environment remain outside the deterministic integration coverage.

## Work orders

- [x] [Task 01: Replay an uncommitted fresh-start successor safely](task-01-replay-uncommitted-successor.md)

One sequential work order has no dependencies. No subagents are authorized.

## Verification results

Investigation reproduced the expected defect in both temporary tests described above.
Implementation reproduced the same failure in the permanent positive regression on
unchanged production code, then passed after the preflight correction.

Design-package checks on 2026-11-07:

- `python3 scripts/list-docs.py validate`: passed, 436 decisions and 1931 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- The repository's `validateCoverage` preflight: `ok: true`, `status: covered`.
  This check included the prospective production edit to validate the work order's cross-references.
  The exact command is in the work order.
- `git diff --check -- docs/specs docs/plans/fresh-start-bootstrap-replay`: passed.

Final verification on 2026-11-07:

- `(cd apps/backend && go test -trimpath -race -count=1 ./internal/agent/runtime/lifecycle -run '^TestDeliveryPreflightUncommittedFreshStartSuccessor$')`: `ok` (2.0s).
- `(cd apps/backend && go test -trimpath -race -count=1 ./internal/agent/runtime/lifecycle -run 'Test(DeliveryPreflight|ResumedDeliveryCheckpoint|FreshStart|BootstrapReplay|SilentRestore|DeliveryRetention)')`: `ok` (13.8s).
- `(cd apps/backend && go test -trimpath -count=1 ./internal/agent/runtime/lifecycle)`: `ok` (247.5s).
- `(cd apps/backend && go build -trimpath ./...)`: passed.
- `bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/delivery_stream_preflight.go apps/backend/internal/agent/runtime/lifecycle/delivery_stream_successor_bootstrap_preflight_test.go apps/backend/internal/agent/runtime/lifecycle/fresh_start_bootstrap_recovery_test.go`: 0 issues.
- `python3 scripts/list-docs.py validate`: passed, 436 decisions and 1931 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check`: passed.
- The repository's `validateCoverage` preflight: `ok: true`, `status: covered`.

The positive preflight regression failed on unchanged production code with
`agent delivery checkpoint missing` for selected generations 2 and 10. The
registered-journal recovery sequence admitted each retained successor event only
after its execution-fenced successor commit, acknowledged ordered replay, kept
the projected predecessor stream unchanged, and never dispatched an instruction.

### Fixup remediation

CodeRabbit review identified a retained-terminal prompt-attempt collision:
a replayed error from the failed attempt could share the retry's prompt
generation and fail its live prompt. `deliverySubmissionIdentityMismatch` now
fences terminal events by their original submission identity. A stale terminal
event is still received, projected, settled, and acknowledged, but it cannot
complete the live prompt. The retained-status resume-token finding is already
addressed by the orchestrator's stale ACP-session guard in `storeResumeToken`,
which drops a status event whose ACP session ID differs from the live execution's
current native session.

Fixup verification:

- `(cd apps/backend && go test -trimpath -count=1 ./internal/agent/runtime/lifecycle)`: `ok` (161.9s).
- `(cd apps/backend && go test -trimpath -race -count=1 ./internal/agent/runtime/lifecycle -run 'Test(DeliveryPreflight|ResumedDeliveryCheckpoint|FreshStart|BootstrapReplay|SilentRestore|DeliveryRetention|RecoveredTerminal|ClaimPromptCompletion)')`: `ok` (12.8s).
- `bash scripts/lint-go-changed <the fixup Go files>`: 0 issues.

## Risks

- Broadening only the numeric generation check can admit committed or unrelated streams. Require explicit replacement intent and exact predecessor adjacency.
- Reusing the generation-one wait for a successor can admit the predecessor prematurely. Keep the existing successor owner and wait.
- Replayed process-exit events can belong to a retired manager. Preserve producer identity checks and include this schedule in integration coverage.
- An actual unsettled submission remains a separate reconciliation problem. This fix cannot bypass it.
- The reported installation is not available for direct recovery. Deterministic source and real-store reproductions support the fix package, not a claim of repaired live state.

## Documentation impact

Internal docs are updated. Public recovery actions, terminology, and operator procedures stay the same.
The public-doc search found no guidance specific to this missing-checkpoint bootstrap window.
The package restores the existing fresh-start behavior and adds no manual journal-editing procedure.
Broader launch-error presentation belongs to the separate issue #5128 investigation.
