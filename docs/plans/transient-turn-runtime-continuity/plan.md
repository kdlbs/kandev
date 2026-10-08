---
created: 2026-10-03
status: complete
requirements:
  - REQ-PLATFORM-TURN-CONTINUITY-001
  - REQ-PLATFORM-TURN-CONTINUITY-002
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-001
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-002
system_design:
  - ../../specs/platform/system-design/transient-turn-runtime-continuity.md
  - ../../specs/platform/system-design/provider-error-recovery.md
  - ../../specs/platform/system-design/provider-interruption-continuation.md
legacy_specs: []
---

# Implementation plan: Transient turn runtime continuity

## Overview

Preserve a usable ACP runtime when a temporary provider error ends its current turn.
Report the error in Chat and let the user continue through the normal composer.
Runtime preservation does not authorize automatic replay after unsafe or uncertain work.

First deliver failed-turn settlement without process teardown.
Then reuse that runtime for existing eligible recovery.
Finally correct historical presentation and prove continuity through desktop and phone Chat.
Each work order runs sequentially in the primary session.
Implementation and verification are complete in the primary session. The reported live task remains untouched.

## Evidence and baseline

Investigated task: `f4584500-cf79-4757-8baa-d20c4b1f4c92`.
Execution: `31387aa1-e988-4923-abbb-3347d866f588`.
At 14:44:57 UTC, ACP delivered Codex system-error and model-capacity notifications.
The adapter logged capacity after its prompt RPC completed.
Lifecycle assigned a synthetic failure exit code; orchestration explicitly stopped the process.
The process log records an intentional stop, followed by a graceful exit.
This was application teardown, rather than evidence of an initial agent crash.

The historical error row also used startup fallback copy because it contained an execution ID.
Its retry-exhaustion copy did not reflect the refused replay.
Raw notifications, backend logs, and persisted messages support these findings.
The archived raw log does not include the prompt response frame; RPC settlement follows from the adapter branch and its log.

Planning baseline is main `a81c68fe838a18d946365544dbb44c137d8f0d0f`, rebased without conflicts.
Its Cursor continuation package is completed and supplies shared recovery ownership.
Do not rewrite that completed package's historical results.
The existing recovery requirements and designs now cross-reference the lifetime amendment.
Their real-runtime-loss restoration, safety, budgets, and rollout contracts remain authoritative.
This package amends runtime lifetime through its own [requirements](../../specs/platform/requirements/transient-turn-runtime-continuity.md),
[design](../../specs/platform/system-design/transient-turn-runtime-continuity.md), and
[decision](../../decisions/2026-10-03-transient-turn-runtime-lifetime.md).

## Scope

### In scope

- Host-generated evidence that a supported transient provider error settled while ACP remained usable.
- Separate failed-turn settlement from terminal execution failure, including blocking callers and retained outcomes.
- Manual follow-up and supported model changes on the same process, connection, execution, and native conversation.
- Existing replay and enabled Cursor continuation on that live runtime, with their existing safety fences and budgets.
- Accurate error history, retry dispositions, composer availability, and localized desktop/phone presentation.
- Positive process-identity evidence and negative coverage for real runtime failures and unsupported contexts.

### Out of scope

- Automatic model switching, retry after writes, new continuation providers, new retry loops, or changed retry limits.
- Office, dynamic routing, utility calls, passthrough, and automation-owned execution policy changes.
- Retention on uncertain transport, unknown error shapes, failed initialization, or actual process exit.
- New rollout flags or promotion of `providerInterruptionContinuation`.
- Runtime adoption changes, database schema migrations, or edits to the affected production task.

## Technical approach

### Failed-turn runtime boundary

Add a validated optional `PromptFailureDisposition` to `streams.AgentEvent` and retained turn outcomes.
Only host code can attest `retain_runtime` after ordered prompt settlement and current transport checks.
Propagate it through process forwarding, instance outcome storage, JSON, lifecycle completion signals, and watcher snapshots.
Missing or invalid values retain conservative behavior.

In lifecycle, keep a validated execution live and tracked instead of assigning a synthetic exit code.
Publish a separate internal `AgentTurnFailed` event, rather than `AgentFailed` or normal success `AgentReady`.
Preserve the foreground admission fence until the observed turn has a durable owner and settlement.
A typed blocking-caller error must not trigger a second `handlePromptError` settlement.

The dedicated orchestrator handler preserves startup, cancellation, generation, and queue ownership guards.
It settles the failed turn once and creates a durable error message without blocking `last_agent_error` recovery controls.
True process exit remains authoritative and cannot be reversed by a delayed failure callback.
No schema migration is expected; new metadata carries presentation facts, not runtime authority.

### Recovery on the live runtime

Extend the existing transient retry owner with the retained execution evidence.
For an eligible replay, dispatch through ordinary prompt admission on the same runtime.
Do not run cleanup or native restoration first.
Keep the existing five-attempt budget, delays, evidence checks, dispatch acceptance, and queued-input priority.

For eligible Cursor continuation with its existing toggle enabled, branch before predecessor teardown.
Use the existing instruction and safety snapshot on the preserved native conversation.
Retain the dead-runtime restoration branch and existing unsupported-provider refusal.
Cancel or exhaust an idle retry without stopping the usable runtime.
Ordinary dispatched-turn cancellation keeps its bounded escalation rules.

### Compatibility matrix

| Provider or context | Transport and evidence | Intended result | Planned evidence and fallback |
| --- | --- | --- | --- |
| Codex ACP capacity | Ordered systemError plus capacity chunk; settled prompt RPC; current initialized ACP remains open | Retain runtime, fail turn; manual continuation after tool activity | Adapter wire fixture and full-stack capacity fixture; missing correlation or closed transport uses existing recovery |
| Tested ACP overload/rate limit | High-confidence application reply; settled RPC; exact generation and live transport | Retain runtime; replay only if existing pre-effect fence permits | Application-error fixtures with second prompt on the same connection; generic internal/Data-only evidence receives no attestation |
| Cursor ACP terminal stream diagnostic | Tested transient stream shape; RPC settled; live current connection | Retain runtime; eligible continuation follows existing feature and safety checks | Existing Cursor fixtures plus retained-runtime recovery tests; unsupported capabilities retain manual policy |
| Cursor SDK peer disconnect | SDK connection is closed or settlement cannot establish usability | Existing terminal recovery and native restoration | Keep existing transport-loss tests; semantic retryability cannot override lost transport |
| Real process exit or initialization failure | Authoritative exit, stream close, or uninitialized runtime | Existing recovery controls and cleanup | Barrier-controlled lifecycle/service tests and true-failure UI tests |
| Older remote agentctl or unknown shapes | Field omitted/invalid, missing identity, or untested provider evidence | Existing conservative policy | Wire omission/invalid tests; no provider name alone grants support |
| Office, dynamic, utility, passthrough, automation | Unsupported runtime ownership context | Existing policy | Context exclusion tests, even with valid provider attestation |

Shared classification does not prove provider support or transport liveness.
An in-session model change is covered only when the adapter already supports that operation.
Preserving ACP does not promise preservation of an interrupted model invocation.

### Error projection and documentation

Update the shared historical recovery model so an execution ID alone cannot select bootstrap presentation.
Render retained turn failures as normal transcript errors with the ordinary composer.
Keep genuine initialization and workspace recovery controls.
Report actual retry attempts, including refusal, cancellation, and exhaustion.
Use existing disclosure, selector, send controls, and mobile Chat scroll ownership.
Localize new copy through all supported catalogs and the repository generators.

After delivery, update `docs/public/sessions-and-review.md` with the actual behavior.
Update scoped mock-agent guidance if it currently describes mandatory restart for transient fixtures.
Do not publish speculative public behavior during this design turn.

## ASCII UI preview

**UI-01: Task Chat, capacity failure after tool activity.**
The current screenshot and historical view model support the before view.
The proposed views map to AC-PLATFORM-TURN-CONTINUITY-002.1 through 002.5.

```text
Before: historical row on desktop
  [Resolved]
  The agent could not start. Retry resume or restore...
  [v Technical details]
  Resumed agent Codex

After: desktop transcript and composer
  [!] Selected model is at capacity.
      Try again or choose another model.
      [> Technical details]
  ----------------------------------------------------
  [Type a message...                                 ]
  [6.1 Sol v]                                  [Send]

After: phone transcript and composer
  [!] Selected model is at
      capacity. Try again or
      choose another model.
      [> Technical details]
  ------------------------------
  [Type a message...            ]
  [6.1 Sol v]              [Send]
```

Transcript content scrolls; the existing composer remains fixed in its current safe-area region.
The phone uses the same inline status entry with wrapping and touch-accessible existing controls.
There is no new sheet, overlay, model picker, or required recovery button.
The provider condition, normal composer, and inline hierarchy are structural requirements.
Exact localized wording, icons, spacing, and model label are illustrative.

**UI-02: Task Chat, existing automatic recovery notice.**
This view maps to AC-PLATFORM-TURN-CONTINUITY-001.4 and 002.3 through 002.5.

```text
Pending, desktop or phone with wrapping
  Temporary provider error. Retry in 5s. [Cancel]

Cancelled or exhausted, runtime still usable
  [!] Recovery stopped. You can send another message.
      [> Technical details]
  [Type a message...]
  [Model v]                                  [Send]
```

Reuse the existing retry notice and backend cancellation owner.
Never claim exhaustion when replay was refused before an attempt started.
Details retain actual attempt information and sanitized diagnostics.
The same desktop/phone composer rules as UI-01 apply.

## Tests

The following test names are planned, except explicitly identified existing suites.
Implement focused Red-Green-Refactor coverage within each work order.

| Acceptance criteria | Planned unit or integration evidence |
| --- | --- |
| 001.1, 001.3 | `adapter_prompt_capacity_continuity_test.go`: `TestCodexCapacityKeepsACPConnectionForNextPrompt`; `manager_turn_failure_test.go`: `TestTransientTurnFailureKeepsExecutionReady` |
| 001.1, 001.6, 001.8 | `prompt_failure_disposition_test.go` in streams and process: `TestPromptFailureDispositionWireValidation`; instance `turn_outcome_test.go`: `TestTurnOutcomeRetainsPromptFailureDisposition`; watcher `turn_failure_test.go`: `TestTurnFailurePreservesIdentityAndDisposition` |
| 001.2, 001.8 | `event_handlers_turn_failure_test.go`: `TestCapacityAfterToolsKeepsRuntime`, table cases for read/write/unknown effects; `TestTurnFailureUnsupportedContextsKeepExistingPolicy` |
| 001.6, 001.7 | Lifecycle `manager_turn_failure_test.go`: `TestTurnFailureProcessExitWins`, `TestTurnFailureExplicitLifecycleActionsWin`, `TestTurnFailureDuplicateAndStale`; orchestrator `event_handlers_turn_failure_test.go`: `TestTurnFailureSettledOnceBeforeFollowup`, `TestRetainedTurnFailureDoesNotAdvanceWorkflow`, `TestTurnFailureSettlementFailureBlocksAdmission` |
| 001.4, 002.3 | `event_handlers_transient_retained_runtime_test.go`: `TestTransientReplayUsesRetainedRuntime`, `TestRetainedRetryCancelAndExhaustionKeepRuntime`; existing transient ordering, replay-safety, resolution, and transport-loss suites |
| 001.5, 001.7 | `provider_interruption_retained_runtime_test.go`: `TestContinuationUsesRetainedRuntime`, `TestContinuationTransportLossStillRestores`, `TestRetainedContinuationQueuedUserWorkWins`; existing interruption budget, ownership, refusal, cancellation, and restart suites |
| 002.1, 002.2, 002.4 | Orchestrator `TestRetainedTurnFailurePersistsOneNonblockingError`; web `action-message-recovery.test.tsx`: capacity legacy record, retained error, true bootstrap, and later successful turn cases; existing recovery model and last-agent-error suites |
| 002.3, 002.5 | Existing `action-message.test.tsx`: refused versus started retry attempts and localized notice results; locale checks and rendered Playwright scenarios below |
| Amended provider recovery 001.11; interruption continuation 001.3 and 002.2 | Retained-runtime replay/continuation service tests and desktop/phone idle-cancellation scenarios; real-loss restoration and recovery-control regression tests |

Number-only IDs in this table use the `AC-PLATFORM-TURN-CONTINUITY-` prefix.
The amendment row references the linked provider recovery and interruption requirement documents.
Mock-agent unit tests verify fixture semantics without launching the reported agent.
Physical process, connection, execution, native identity, and initialization/restoration counters prove continuity.
A single boot row or stable task-session ID is insufficient evidence.

## E2E tests

| Flow | Criteria | Planned file and project |
| --- | --- | --- |
| Capacity after visible output/tools; send again and change a supported model without restart | 001.1-.3, 001.7, 002.1-.2 | New `e2e/tests/session/transient-turn-runtime-continuity.spec.ts`, `chromium` |
| Refresh, historical pagination, second viewer, and later success keep one truthful error | 002.2-.4 | Same new desktop suite |
| Capacity, model selection, draft/send, wrapped details, retry cancellation through touch interaction | 001.3-.4, 002.1-.5 | New `e2e/tests/session/mobile-transient-turn-runtime-continuity.spec.ts`, `mobile-chrome` |
| Eligible replay succeeds; idle cancellation and exhaustion preserve the runtime | 001.4, 002.3 | Existing `transient-retry.spec.ts` and `mobile-transient-retry.spec.ts`; update retained-path expectations |
| Enabled safe Cursor continuation uses the live runtime; actual disconnect still restores | 001.5-.7 | Existing desktop and mobile `provider-interruption-continuation.spec.ts` suites; retain restore coverage |
| Data-only transport-loss and genuine startup recovery retain their controls | 001.6, 001.8, 002.2 | Existing `transient-retry-transport-lost.spec.ts`, `chromium`; `mobile-session-error-recovery-ui.spec.ts`, `mobile-chrome` |

Use deterministic mock traces for runtime identity and ordinary public Chat behavior for user interaction.
Run managed desktop and mobile commands sequentially with the repository's worker limits.
Work order 03 owns exact browser commands and updated fixture scenarios.

## Work orders

- [x] [Task 01: Retain failed-turn runtimes](task-01-retain-failed-turn-runtime.md)
- [x] [Task 02: Reuse runtimes for recovery](task-02-reuse-runtime-for-recovery.md), depends on 01
- [x] [Task 03: Explain errors and prove continuity](task-03-explain-errors-and-prove-continuity.md), depends on 01 and 02

## Verification results

Implementation completed on 2026-10-03; review-fix verification completed on 2026-10-04:
Design-package checks passed on 2026-10-03:

- `python3 scripts/list-docs.py validate`: 348 decisions and 1331 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- The documentation preflight below: actual docs-only changes exempt; planned source scope covered by all three work orders.
- `git diff --check`: passed. New-document links and trailing whitespace were checked separately because the files are untracked.
- `git status --short`: four modified specifications and seven new design-package files; no production or permanent test changes.

Task 01 implementation checks passed in the current implementation turn:

- `go test -trimpath -race -tags fts5 ./internal/agentctl/types/streams ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/process ./internal/agentctl/server/instance -count=1`: all four packages passed.
- `go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle ./internal/orchestrator/watcher ./internal/orchestrator -run 'TurnFailure|TransientTurnFailure|CapacityAfterTools|PromptFailureDisposition|SessionRecovery|Completion' -count=1`: lifecycle and orchestrator passed; the full watcher package also passed separately.
- Task 02 orchestrator and lifecycle/ACP retained-runtime race blocks passed; the mock-agent race suite passed.
- Focused Chat Vitest: 7 files, 117 tests passed. TypeScript typecheck, changed-file ESLint with zero warnings, Prettier, and i18n check/ratchet passed.
- Final managed desktop Playwright rerun: 23 tests passed; final phone rerun: 12 tests passed. Both used one worker and ran sequentially after the mock prompt fixture and production lifecycle adapter fixes.
- Backend build and frontend pseudo-locale QA build passed during implementation.
- Public-doc tests: 62 passed; all 47 public docs validated.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.test.py`, and `python3 scripts/lint-spec-files.py --all` passed after implementation.
- The exact documentation coverage preflight below passed for actual changes and planned source coverage.
- The production backend lifecycle adapter acknowledgement regression test passed, and a real-runtime continuation E2E passed against the backend plus mock ACP process.
- Race-instrumented lifecycle, orchestrator, backendapp, agentctl regression checks and `make -C apps/backend build` passed. `TestStreamUpdates_DisconnectCleansPending` passed 20 race-instrumented repetitions after its test synchronization was fixed.
- Implementation and review remediations are committed on `feature/investigate-acp-capa-781` and are being delivered through PR #4193.

Review follow-up for the four reported defects also passed: retained lifecycle publication no longer holds the prompt lock and fences successor admission through synchronous settlement; retry finalization compare-retires only its exact owner; both automation origins retain their existing terminal failure owner; and inconclusive runtime probes block without teardown. Race-instrumented barrier tests cover the cancellation inversion and stale retry successor handoff. Replay and continuation separately cover confirmed runtime absence. The corresponding test commands and outcomes are recorded in work orders 01 and 02.


### Documentation coverage preflight

Run the following from the repository root.
It checks actual changes, then probes the planned source scope to validate every work-order reference during docs-only planning.
The source probe does not create or change a production file.
After implementation, the actual-changes pass must report covered rather than exempt.

```bash
node <<'NODE'
const fs = require('node:fs');
const cp = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const tracked = cp.execFileSync('git', ['diff', '--name-only', '-z', 'HEAD']).toString().split('\0').filter(Boolean);
const untracked = cp.execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z']).toString().split('\0').filter(Boolean);
const changed = [...new Set([...tracked, ...untracked])];
const documents = [
  ...fs.readdirSync('docs/plans/transient-turn-runtime-continuity').map(p => `docs/plans/transient-turn-runtime-continuity/${p}`),
  'docs/specs/platform/requirements/transient-turn-runtime-continuity.md',
  'docs/specs/platform/system-design/transient-turn-runtime-continuity.md',
  'docs/specs/platform/requirements/provider-error-recovery.md',
  'docs/specs/platform/system-design/provider-error-recovery.md',
  'docs/specs/platform/requirements/provider-interruption-continuation.md',
  'docs/specs/platform/system-design/provider-interruption-continuation.md',
];
const fileContents = Object.fromEntries(documents.map(p => [p, fs.readFileSync(p, 'utf8')]));
for (const [label, paths] of [
  ['actual changes', changed],
  ['planned source coverage', [...changed, 'apps/backend/internal/orchestrator/event_handlers_turn_failure.go']],
]) {
  const result = validateCoverage({ changedFiles: paths, fileContents });
  console.log(JSON.stringify({ label, status: result.status, workOrders: result.workOrders, errors: result.errors }));
  if (!result.ok) process.exitCode = 1;
}
NODE
```

## Risks

- Process exit can race turn failure; admission and late callbacks must retain exact identity and terminal precedence.
- A blocking caller and asynchronous watcher can otherwise settle twice or damage a successor turn.
- Retention consumes an execution slot while the process remains live; existing idle and explicit-stop policies still apply.
- RPC settlement cannot prove a provider's internal invocation survived; only the ACP runtime is retained.
- Older remote components lack the attestation and keep conservative recovery until upgraded.
- Legacy history may contain inaccurate retry copy. Preserve available diagnostics without inventing missing attempt history.
- Model changes are adapter-dependent. They must use existing capability checks and cannot create another native session.
