---
created: 2026-10-02
status: completed
requirements:
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-001
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-002
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-003
system_design:
  - ../../specs/platform/system-design/provider-interruption-continuation.md
legacy_specs: []
---

# Implementation Plan: Provider interruption continuation

## Overview

Make supported Cursor interruptions recover through native conversation restore
and a new continuation instruction, while retaining the original-prompt replay
fence. Establish positive compatibility/effect evidence first, extend the one
existing retry owner second, then make feedback truthful and prove the result in
desktop and phone Chat. Finish with operator guidance and contract reconciliation.

Platform owns this package because shared recovery admission and ownership, not
the Cursor profile or chat renderer, define the durable contract. Read the
[requirements](../../specs/platform/requirements/provider-interruption-continuation.md),
[design](../../specs/platform/system-design/provider-interruption-continuation.md),
and [decision](../../decisions/2026-10-02-safe-interrupted-conversation-continuation.md).

## Assumptions and investigation

- Confirmed intent: improve resilience to transient Cursor connection errors
  and produce a design package before implementation.
- Verified: current classification accepts the captured HTTP/2 error; the
  replay gate rejects work-bearing turns; the exhaustion message is selected
  even without a scheduled retry. Focused classifier, ACP, and orchestrator
  tests passed during investigation; delivery evidence is recorded below.
- Implemented conservative scope: output and confirmed completed reads can
  continue; writes, pending tools, and uncertain outcomes remain manual. The
  user has not selected broader write recovery. This scope does not solve every
  interruption in a session that has executed commands.
- Task 01 establishes native restore and safe re-reading; mocks cannot replace
  that prerequisite.
- Follow-up native probes confirm same-ID restore through `session/load` and
  successful continuation with a safe re-read. Interrupted read/output state
  is not retained, and direct `session/resume` is unavailable in this CLI.
  The user requires the current Cursor ACP ID; the narrower re-read contract
  is accepted by the user’s continuation instruction. Native identity is required.

## Scope

### In scope

- Typed supported-provider continuation evidence, native restore identity, and
  conservative tool outcome tracking through all terminal-event hops.
- One episode and one retry owner, five shared attempts with existing backoff,
  restore-failure handling, cancellation, queue and workflow settlement.
- Default-off runtime toggle, disabled-path compatibility, and startup cleanup.
- Truthful disposition metadata, localized recovery feedback, and desktop/phone
  inline recovery composition.
- Deterministic mock/native compatibility evidence and public operator guidance.

### Out of scope

- Write recovery, uncertain execution replay, generic provider fallback,
  dynamic/Office/utility policy changes, persistent recovery jobs, and flag
  promotion. No live-user-session replay or host Wi-Fi manipulation.

## Technical approach

### Evidence and rollout identity

Extend ACP `promptTurnState` and ordered tool observation before prompt-end
sweeps. Propagate proposed typed support/safety through
`streams.AgentEvent`, lifecycle `AgentEventData`, and watcher `AgentEventData`.
Keep original `EffectObserved` semantics intact and make absent fields unsafe.
Constructor/dialect support is conditional on native restore negotiation;
orchestration consumes an enum/contract version, not provider names.

Register `features.providerInterruptionContinuation` with environment
`KANDEV_FEATURES_PROVIDER_INTERRUPTION_CONTINUATION`, all shipped defaults off,
and restart-required experimental metadata. Update typed config, runtimeflags,
frontend defaults, agentctl configuration, and contract tests together.

| Compatibility shape                                                 | Recovery                                    | Required evidence/fallback                                                   |
| ------------------------------------------------------------------- | ------------------------------------------- | ---------------------------------------------------------------------------- |
| Cursor ACP native identity; no tool work                            | Same-conversation continuation after output | Native restore probe plus fixture; missing identity is manual                |
| Cursor ACP tested `read` kind; all tools completed                  | Same-conversation continuation              | Native read contract plus outcome fixture; titles/search heuristics excluded |
| Write, execute, MCP, pending read, subagent, background, permission | Manual recovery                             | Negative fixtures and sticky episode safety                                  |
| Other adapter or old remote fields absent                           | Existing replay/manual behavior             | Remote JSON round-trip/omission tests                                        |
| Dynamic, Office, utility, passthrough                               | Existing policy owner                       | Regression tests                                                             |
| Controlled mock dialect                                             | Test-only support                           | Provenance-gated fixture; never inherited by production dialects             |

### Recovery owner

Extend `transientRetryEntry` and `handleTransientFailure`, factor
`retryTransientPrompt` into mode selection plus common cleanup/restore, and add
native-only restoration without changing manual resume fallbacks. Use existing
session/foreground locks, teardown claims, prompt admission, queue ownership,
error termination, and retry-notice cleanup. Count started attempts separately
from scheduled ordinal, and combine effect evidence across all replacements.

### Feedback

Carry real policy disposition through `createRecoveryStatusMessage`; stop
deriving exhaustion from classification. Extend `ActionMeta` and
`TransientRetryNotice` with recovery mode/phase. The same notice remains visible
and cancellable while continuation runs; no second banner or client timer owner.
New copy uses locale keys and the existing manual recovery model. Correct
exhaustion copy even when the release toggle is off.

### Related packages

Read the completed [Cursor reset](../cursor-retriable-stream-reset/plan.md),
[diagnostic expansion](../cursor-retriable-error-expansion/plan.md),
[provider output retraction](../provider-retry-output-retraction/plan.md), and
[stale-notice cleanup](../stale-transient-retry-notice/plan.md) packages as
regression context. Their work stays completed. This package does not change
response-attempt retraction or the data-only `/transport-lost` manual outcome.

## ASCII UI preview

Labels below are illustrative localized copy. Mode, phase, action order,
single-notice ownership, wrapped details, and phone target geometry are required.
Spacing and exact colors are not pixel specifications.

### UI-01: Automatic recovery, task Chat, waiting then reconnecting/continuing

Desktop (inside existing transcript scroll region):

```text
+---------------------------------------------------------------------+
| Connection interrupted. Continuing in 0:05.                 [Cancel] |
| Cursor | Attempt 1 of 5 | Previous conversation is preserved.         |
+---------------------------------------------------------------------+
  Phase replacement: Reconnecting... -> Continuing previous request...
  Successful turn: recovery notice is removed.
```

Phone (same Chat entry point, same single transcript scroll owner):

```text
+--------------------------------------+
| Connection interrupted               |
| Continuing in 0:05                   |
| Cursor | Attempt 1 of 5              |
| Previous conversation is preserved.  |
| [               Cancel             ] |
+--------------------------------------+
```

Cancel is 28px on fine-pointer desktop and at least 44px on phone/coarse pointer.
The composer retains existing safe-area behavior. Status does not steal focus;
countdown ticks are not repeated live announcements. Maps to `.003.1` and `.003.4`.

### UI-02: Manual recovery, current connection failure, unsafe work

Desktop:

```text
+---------------------------------------------------------------------+
| Connection interrupted                                              |
| Automatic continuation stopped because tool work needs checking.      |
| [Resume session] [Start fresh session]                                |
| > Technical details                                                  |
+---------------------------------------------------------------------+
```

Phone:

```text
+--------------------------------------+
| Connection interrupted               |
| Tool work needs checking before      |
| continuing automatically.            |
| [         Resume session           ] |
| [       Start fresh session        ] |
| > Technical details                  |
+--------------------------------------+
```

Existing alternative actions remain when eligible. Exhaustion instead says
automatic recovery did not succeed after the actual started count; cancellation
says automatic recovery was cancelled. Neither uses exhaustion copy when zero
attempts started. Details expand inline and wrap. Maps to `.003.2` and `.003.4`.

## Tests

The implementation tests below provide delivery evidence.

| Criteria (prefix `AC-PLATFORM-INTERRUPTION-CONTINUATION-`) | Evidence                                                                                                                                                                       |
| ---------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `001.1`, `001.2`, `001.5`                                  | `dialect_cursor_continuation_test.go`: `TestCursorContinuationEvidence`; native restore test; remote omission and sweep ordering                                               |
| `001.3`, `001.4`                                           | Continuation admission/refusal and native-only lifecycle/executor tests; missing native identity is covered at the repository/lifecycle boundary; existing replay-safety tests |
| `002.1`                                                    | `TestInterruptionContinuationBudget`: five attempts shared with transient restore failures and no progress reset                                                               |
| `002.2`                                                    | `TestInterruptionContinuationOwnership`: timer, restore, admission, dispatch, teardown, queued human work, and multi-viewer races                                              |
| `002.3`, `003.3`                                           | `TestInterruptionContinuationRestart`: shutdown, stale persisted notice, adopted live execution, and cleanup failure                                                           |
| `002.4`                                                    | `TestInterruptionContinuationSettlement`: interrupted turn, queue, workflow, CI outcome, and one successful completion                                                         |
| `002.5`                                                    | Registry/profile/features tests and `TestInterruptionContinuationDisabled`: no collection or dispatch on all entry paths                                                       |
| `003.1`, `003.2`, `003.4`                                  | Backend disposition tests and `interruption-recovery-feedback.test.ts`; existing action-message and recovery-model tests                                                       |

## E2E tests

Add `apps/web/e2e/tests/session/provider-interruption-continuation.spec.ts`
(`chromium`) and `mobile-provider-interruption-continuation.spec.ts`
(`mobile-chrome`). Use test-base isolated runtimes and a mock-only support
contract. Set the toggle through `backend.restart(overrides)`; inherited
`KANDEV_FEATURES_*` values are stripped by the fixture. Restore baseline after
each scenario and never alter the shipped e2e profile default.

| Flow                                                                                                                                                    | Criteria                                   |
| ------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------ |
| Output-only and completed-read interruption, native identity retained, no original-prompt resend, partial history preserved, eventual success           | `001.1`-`001.5`, `002.4`, `003.1`, `003.3` |
| Pending read, write, or unknown tool enters manual recovery without false exhaustion; missing restore identity is checked in repository/lifecycle tests | `001.2`, `001.3`, `003.2`                  |
| Restore fails transiently then succeeds; hard/ambiguous restore stops                                                                                   | `001.5`, `002.1`                           |
| Cancel waiting, cancel dispatched continuation, queued user prompt supersedes, two viewers do not duplicate dispatch                                    | `002.2`, `003.3`                           |
| Reload during accepted continuation, persisted manual state after cleanup, backend restart without redispatch, live adoption preserved                  | `002.3`, `003.3`                           |
| Disabled toggle keeps prior behavior and truthful failure copy                                                                                          | `002.5`, `003.2`                           |
| Phone Cancel and manual actions have 44px targets; expanded details wrap; no horizontal overflow                                                        | `003.4`                                    |

Record provider session ID and dispatched prompt count through bounded mock
evidence/API state, in addition to UI assertions. Arm causal observations before
actions; do not use arbitrary sleeps or `/transport-lost` as the new fixture.
Keep existing desktop/phone transient-retry tests in the focused regression run.

## Work orders

All work is sequential in the primary session. Continuation dependencies are
01 -> 02 -> 03 -> 04 -> 05. Task 00 extracts the independent baseline copy fix
from Task 03 so it can proceed without weakening native continuation gates.
No subagents are authorized by this package.

- [x] [Task 00: Correct manual recovery copy independently of continuation](task-00-truthful-manual-recovery.md)
- [x] [Task 01: Establish continuation evidence and rollout contract](task-01-continuation-evidence.md)
- [x] [Task 02: Add bounded same-conversation continuation](task-02-continuation-owner.md)
- [x] [Task 03: Render truthful recovery feedback](task-03-recovery-feedback.md)
- [x] [Task 04: Prove isolated desktop and phone recovery](task-04-recovery-proof.md)
- [x] [Task 05: Reconcile contracts and document rollout](task-05-contracts-and-rollout.md)

## Verification results

Implementation and task-defined verification are complete. The
native prerequisite proves same-ID saved-history restoration and successful
continuation with safe re-reading. The default-off runtime flag, bounded safety
ledger, single continuation owner, feedback, and operator guidance are implemented.

Verification on 2026-10-02:

- Cursor Agent `2026.10.01-e373342` advertises `loadSession=true`; direct
  `session/resume` returns `-32601`. Owned cancellation and abrupt-process-loss
  probes restore the same conversation and finish the read request. Interrupted
  read results and assistant output are not retained in those probes. See
  [compatibility evidence](compatibility-evidence.md).
- Focused race tests cover adapter evidence, lifecycle and watcher propagation,
  native-only restoration, owner/admission/queue/cancellation/restart boundaries,
  configuration, profiles, registry, and mock wire fixtures. The final affected
  orchestrator/executor race run passes; scoped lint reports zero issues.
- Feedback/action/recovery unit tests pass (39 tests). Web typecheck, lint,
  complete locale checks, and formatting checks pass.
- Sequential Docker desktop runs cover 14 new checks and six existing replay/copy
  regressions. The final fresh-artifact run passes all seven flows affected by
  startup/admission changes: read, output, transient restore, accepted cancellation
  with reload/two viewers, ambiguity, and both restart survival policies. The
  first-turn overload regression passes its isolated rerun after a timeout in
  the longer matrix run. These results do not claim one all-green 20-case run.
- Phone continuation cancellation passes: Cancel remains visible during RUNNING,
  touch targets are at least 44px, history remains, manual actions appear, expanded
  details wrap, and the document has no horizontal overflow. Both mobile tests pass, including the existing recovery regression.
- Native probes interrupt owned processes; mock integration simulates transient
  errors. No live user session or Wi-Fi connection was changed, and these checks
  do not claim a real upstream HTTP/2/Wi-Fi reproduction.

The initial macOS browser launch failure and Linux integration failures were
resolved through Docker rendering and regression-backed ownership fixes. The
broader lifecycle suite remains subject to unrelated sandbox global-Git-config
and unsafe-temporary-checkout failures; affected scoped checks pass.

- Catalog/spec lint, public-doc validation, actual changed-file documentation
  coverage, and diff/whitespace checks pass. All six work orders are complete;
  the requirement/design/decision lifecycle records describe the implemented
  experimental boundary. The flag is not enabled by publication.

Final focused browser commands, run sequentially from `apps/web`:

```bash
GOCACHE=/private/tmp/kandev-provider-interruption-go-cache GOMAXPROCS=4 pnpm e2e:run --docker --project chromium tests/session/provider-interruption-continuation.spec.ts --grep 'integration:|accepted continuation|backend restart|read-ambiguous'
GOCACHE=/private/tmp/kandev-provider-interruption-go-cache GOMAXPROCS=4 pnpm e2e:run --docker --no-build --project mobile-chrome tests/session/mobile-provider-interruption-continuation.spec.ts tests/session/mobile-transient-retry.spec.ts
```

Desktop passed 7/7; phone passed 2/2. The phone run reused the exact fresh
production artifacts from the preceding desktop build without intervening
production changes. Both runs used one worker.

Commit preflight additionally enforced the runtime import boundary and the
E2E helper parameter limit. Recovery now consumes the existing runtime facade
constants/errors, and fixture options occupy one argument. Focused recovery
race tests and web typecheck pass; this is an internal boundary correction
without a product-contract change. A shorter focused run also exposed mock
fixture lifetime cleanup; accepted mock contexts are now explicitly released
at test teardown without changing production shutdown survival policy.

## PR review remediation

PR #4165 reconciles the main-branch workspace and startup recovery contracts.
Focused race regressions verify atomic owner publication, uncertain lookup
refusal, cancellation identity fencing, automation turn binding, failed stop
claim release, interrupted-turn persistence, and replay notice retirement.
Adapter regressions cover terminal finalizer retirement and foreign-session
permissions. All focused backend checks and scoped lint pass. Linux Docker
also verifies failed native restore reports both startup and worktree-release
errors; the macOS sandbox could not reach that checkout boundary.

The final web unit run passes 55 tests across the recovery consumers and fixture
cleanup. Typecheck, localization including Korean, focused lint, catalog/spec
lint, public-doc validation, and actual changed-file reference checks pass.
Live Cursor assertions were strengthened but those opt-in probes were not
rerun in review fixup.

Fresh Docker desktop verification passes 10 checks: nine recovery scenarios and
one capture. The phone rerun passes all three checks: continuation cancellation,
legacy retry cancellation, and capture. Its initial failure was a stale legacy
cancellation-copy expectation; desktop and phone assertions now use the typed
localized recovery wording. Each run uses one worker and the same production
build, without intervening production changes.

```bash
# From apps/web; run these sequentially.
pnpm e2e:run --docker --shards 1 --project chromium tests/session/provider-interruption-continuation.spec.ts --grep 'integration:|accepted continuation|read-ambiguous|requires manual recovery'
pnpm e2e:run --docker --no-build --shards 1 --project mobile-chrome tests/session/mobile-provider-interruption-continuation.spec.ts tests/session/mobile-transient-retry.spec.ts
```

Disposable capture specs add one check per viewport and are removed before
commit. Phone coverage is Pixel 5 browser emulation; no physical-device or real
Wi-Fi-change validation is claimed. The continuation toggle remains off.

## Remaining risks

- Native Cursor `session/load` may restore history without enough evidence of
  a completed read outcome. The tested contract permits safe re-reading and does not promise interrupted
  result retention.
- The investigated task has execute/edit activity in its history. Turn- and
  episode-scoped evidence, not whole-transcript scanning, decides eligibility;
  an unsafe interrupted episode still needs manual recovery.
- Prompt-end sweeping can erase real pending-tool evidence. Snapshot first and
  test queued update ordering.
- A restore path can fall back to `session/new`. Native-only recovery must
  reject that before dispatch without changing manual resume behavior.
- Cancel after provider acceptance requires real session cancellation, not
  timer cleanup alone. Teardown/adoption races must not stop the successor.
- The package promises neither write recovery nor exactly-once arbitrary agent
  actions. Promotion beyond conservative scope requires a separate design.
