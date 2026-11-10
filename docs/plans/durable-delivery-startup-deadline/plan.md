---
created: 2026-11-05
status: completed
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
system_design:
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
legacy_specs: []
---

# Fix plan: preserve the native session startup budget

## Overview

Fix the confirmed startup deadline defect in [issue #5057](https://github.com/kdlbs/kandev/issues/5057).
An early durable setup event can close a healthy updates stream after 30 seconds.
The pending native session request then fails before its own allowed deadline.
One work order implements and verifies the correction.

Platform owns this repair because durable event admission controls the stream
that carries native startup responses. Native conversation identity, task
recovery policy, and executor lifetime retain their existing owners.

The package uses existing delivery requirements. It clarifies the internal
wait lifecycle without adding a public timeout, recovery action, or protocol.
The implementation now binds both waits to the startup attempt and adds the
permanent regressions described below.

## Evidence and root cause

### Baseline

- Investigation date: 2026-11-05.
- Local HEAD and verified remote `main`: `a8b8ac18e0f08cafc48f303eafc88ffd546f31f4`.
- Reported release: v0.98.0, commit `2d93502c256780e1f3d9df4348dafbd97b584188`.
- The issue had no comments or attached images during investigation.
- The workspace was clean before temporary reproductions.
- Issue #5057 is assigned to the authenticated GitHub user, `carlosflorencio`.
  Assignment was verified through a fresh issue read.

Refresh the base and inspect concurrent edits before implementation.

### Confirmed failure path

1. `InitializeAndPromptWithLayers` marks generation-one delivery bootstrap
   before connecting the updates stream.
2. `InitializeSessionWithSettingsPolicy` completes delivery negotiation after
   `agent.initialize`, then sends `agent.session.new` or restores a native session.
3. An early retained setup event reaches `prepareDurableAgentDeliveryEvent`
   and `deliveryEventIsStale`.
4. `deliveryHarnessGeneration` polls for SQL authority with an independent
   30-second timeout. SQL cannot contain the new native generation yet.
5. The native ID and `AgentACPSessionCreated` publication occur only after
   `initializeACPConnection` returns successfully. The orchestrator then
   persists that generation in `persistFirstHarnessGeneration`.
6. A native request that takes more than 30 seconds therefore loses its stream
   before that publication. Processor failure cancels the connection, and
   `cleanupPendingRequests` releases the pending RPC with a disconnect error.

`constants.SessionNewTimeout` and `constants.SessionLoadTimeout` each allow two
minutes at the agentctl API. Neither value authorizes the earlier event timer.
The generation poll must distinguish pending native initialization from a
failed SQL commit after native success.

`waitForDeliveryNegotiation` has the same independent 30-second cutoff. An early
event can also expire before a valid initialization response arrives.
Its implementation is unchanged between the reported release and this baseline.

### Reproduction evidence

Temporary tests ran against the unmodified lifecycle package:

| Test | Controlled condition | Observed result |
| --- | --- | --- |
| `TestIssue5057BootstrapAllowsSessionNewBudget` | SQL generation becomes available after 35 virtual seconds. Caller allows two minutes. | Failed at exactly 30 seconds, with 90 seconds remaining. |
| `TestIssue5057NegotiationAllowsInitializeBudget` | Negotiation succeeds after 35 virtual seconds. Caller remains active. | Failed at exactly 30 seconds. |
| `TestIssue5057SlowSessionNewStream` | Real WebSocket response waits 35 seconds. An early setup event has no committed generation. | Failed after 30.003239175 seconds with both reported error signatures. Caller context remained active. Receipt count stayed zero. |

The full transport reproduction logged:

```text
durable agent event ownership check: context deadline exceeded
agent.session.new request failed: agent stream not connected while waiting for response
```

The two virtual-time cases ran with:

```bash
(cd apps/backend && go test -trimpath -count=1 ./internal/agent/runtime/lifecycle -run '^TestIssue5057' -v)
```

The full transport case ran separately with:

```bash
(cd apps/backend && go test -trimpath -count=1 ./internal/agent/runtime/lifecycle -run '^TestIssue5057SlowSessionNewStream$' -v)
```

These failures establish the local defect. They reproduce the reported timing
and error sequence without deleting any session. They do not prove every cause
in the reporter's environment. The temporary reproduction file was removed.
The work order names permanent replacements for these tests.

### Other reported symptoms

- **Original peer/SQL disagreement:** the public report lacks the actual
  submission inventory, hashes, terminal references, and authenticated descriptor.
  The exact mismatch remains unknown. AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.12
  requires refusal for conflicting or unavailable evidence.
- **Stop leaves an unknown outcome:** stopping a process does not establish
  whether its earlier instruction had effects. Clearing that historical block
  without evidence contradicts the existing recovery contract.
- **Retry reports a missing checkpoint:** merged
  [PR #4971](https://github.com/kdlbs/kandev/pull/4971) already permits a relaunch
  when no native identity, SQL generation, receipt, or submission ever committed.
  Its negative cases retain the required refusals. Do not duplicate that repair
  or claim that it covers every checkpoint failure.
- **Unknown outcome after restart:** merged
  [PR #4991](https://github.com/kdlbs/kandev/pull/4991) addresses confirmed remote
  recovery absence. It does not prove why the original records disagreed.
- **Queued-message deletion warning:** this is a separate destructive-action
  requirement. No warning design or queue deletion change is included here.
- **Issue #4405:** remains open. Similar recovery goals do not establish the
  same evidence mismatch or startup cause.

The package addresses the confirmed startup path. Its completion alone is
insufficient to close every concern in #5057.

## Requirement conformance

The [delivery requirements](../../specs/platform/requirements/durable-agent-delivery.md)
remain the behavioral source. No new repair requirement or duplicate specification is needed.

| Criteria | Application |
| --- | --- |
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.3 and 007.4 | Compatible startup uses durable delivery. A slow valid handshake does not require a toggle or unsafe downgrade. |
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.1 and 005.2 | Every receipt and effect still requires committed authority. Obsolete attempts cannot change current state. |
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.3 and 006.12 | Stop and cancellation remain available. Missing or conflicting adoption evidence still blocks admission. |

The existing requirements and stream design remain `draft`. This repair does
not change their status or claim complete delivery release acceptance.
The accepted [harness boundary decision](../../decisions/2026-09-10-durable-harness-session-boundaries.md)
and [recovery convergence decision](../../decisions/2026-10-12-total-session-recovery.md)
remain authoritative. This local wait correction needs no new ADR.

## Scope

### In scope

- Pending negotiation and generation-one bootstrap waits use their captured
  startup attempt and its existing cancellation/deadline authority.
- The missing-generation SQL wait starts after native startup succeeds.
- Initialization failure releases pending event processing with its original cause.
- Late completion and context cleanup cannot affect another startup attempt.
- Existing event, cursor, ownership, and no-resend checks retain authority.
- Focused virtual-time and stream integration regressions prove the correction.

### Out of scope

- Original-session data repair or a claim that its mismatch is now explained.
- Clearing unknown outcomes on Stop, prompt replay, or automatic queue release.
- New recovery actions, UI copy, deletion warnings, or frontend changes.
- New timeout settings, broad timeout increases, schemas, or journal formats.
- Changes to successor-generation restore or committed-session adoption budgets.
- Live-instance mutation, issue comments, publication, or release work.

## Technical approach

### Attempt and stream ownership

Extend the existing attempt gate in `delivery_bootstrap.go`. Keep separate
results for delivery negotiation and native initialization. Capture its
initialization context and `startupAttemptSnapshot()` before streams connect.
Publish results only for that captured attempt.

Wire the exact attempt through `session.go`, `streams.go`, and
`stream_event_processor.go`. A processor must retain its attempt rather than
reading a replacement gate from the mutable execution. Reuse the existing
startup generation and stream cancellation boundaries.

Pending negotiation observes its initialization owner and stream context.
It has no separate 30-second event deadline. A completed successful gate takes
precedence over routine cleanup of its former initialization context.

### Native result and SQL authority

In `InitializeAndPromptWithLayers`, publish native success after accepting the
native ID and publishing `AgentACPSessionCreated`. Publish failure on every exit
before that point. Do not wait for session model/configuration projection,
readiness, or prompt dispatch to publish the native result.

In `deliveryHarnessGeneration`, a pending generation-one attempt waits for that
native result. After success, the existing 30-second SQL authority budget starts
from the result's completion time. The budget cannot restart on another event.
Queries and cancellation remain bounded under the connected stream.

If native success races with startup-context cleanup, SQL can still commit.
Do not bind the post-success SQL wait to the short-lived startup context.
Reuse the final bounded authority-read pattern in
`awaitSilentRestoreGeneration` when cancellation races with a completed commit.
Its separate successor-generation policy remains unchanged.

SQL remains authoritative. The gate cannot fabricate a native generation,
cursor, receipt, projection, or ACK. `deliveryEventIsStale`, normal SQL
admission, and authenticated event identity validation still reject foreign work.

### Compatibility

| Provider or transport | Identity and capability | Intended behavior | Evidence and fallback |
| --- | --- | --- | --- |
| Claude through ACP | Supported durable peer, generation one | Early setup events survive valid slow `session/new` or native restore. | Shared lifecycle tests and a delayed WebSocket fixture. The exact Claude 0.100.0 production trace remains unavailable. |
| Other agents using the shared structured path | Same negotiated durable shape | The same attempt and authority rules apply. | Provider-neutral fixture proves the shared path. No claim about untested provider timing. |
| Relaunch without a committed native identity | Complete g1 retained prefix, zero submissions and ACKs | Keep PR #4971 preflight behavior, then use the corrected startup wait. | Existing `TestDeliveryPreflight*` and new attempt tests. Unsupported evidence still blocks. |
| Existing native g1 first entering durable delivery | Saved native ID and matching ownership | Wait for native restore, then require its committed SQL generation. | Extend `resumed_delivery_bootstrap_test.go`. No identity replacement. |
| Committed or successor generation | Existing recovery evidence | Keep its dedicated restore and adoption paths. | Existing native-resume and silent-restore regressions. Conflicts still block. |
| Genuine older peer or unsupported retained storage | Positively identified legacy capability | Keep the existing legacy route. | Existing negotiation tests. Timeout or malformed evidence never implies legacy. |
| CLI passthrough | No structured delivery bootstrap | No change. | No new routing or provider-specific branch. |

## Tests

New files: `delivery_startup_deadline_test.go` and
`delivery_startup_stream_test.go`, beside the lifecycle source.

| Criteria | Required regression |
| --- | --- |
| 007.3 | `TestDeliveryStartupNegotiationBudget`: negotiation completes after 35 virtual seconds within the caller deadline. |
| 007.3, 005.1 | `TestDeliveryStartupNativeBudget`: native new/load completion at 35 virtual seconds precedes generation commit and valid admission. |
| 005.1, 007.4 | `TestDeliveryStartupMissingAuthority`: successful native initialization followed by no SQL commit still fails after its bounded authority wait. |
| 005.2, 006.3 | `TestDeliveryStartupCancellationAndReplacement`: cancellation, Stop, shutdown, initialization failure, and stale completion drain only their owned attempt. |
| 005.1, 005.2, 006.12 | `TestDeliveryStartupStreamOrdering`: real control responses remain available while setup events wait. Separate native-result and generation-commit barriers prevent early receipt, callback, or ACK. |

Use `testing/synctest` for elapsed-time assertions. The stream fixture uses
channels for ordering, so permanent integration coverage does not need a
35-second wall-clock sleep. Keep the existing foreign-owner, missing-owner,
resume-inventory, legacy, and relaunch refusal tests.

## End-to-end evidence

The diagnostic WebSocket reproduction traversed initialization, event admission,
processor cancellation, pending-request cleanup, and the native request error.
Permanent evidence belongs in `TestDeliveryStartupStreamOrdering` and its
virtual-time deadline companion. Both must drive the production lifecycle entry
point and existing SQL authority boundary where applicable.
Use the real SQLite test repository for the committed generation and receipt
assertions in the permanent stream fixture.

No browser test or UI preview is required. The correction changes backend
timing without changing a rendered interaction. No frontend files are owned.

## Work orders

- [x] [Task 01: Bind delivery waits to the startup attempt](task-01-bind-delivery-startup-waits.md)

The work order runs sequentially in the primary session. No delegation is authorized.

## Verification results

- Diagnostic regressions: expected failures reproduced on unchanged production code.
- Temporary reproduction cleanup: complete.
- `(cd apps/backend && go test -trimpath -count=1 ./internal/agent/runtime/lifecycle -run '^(TestDurableRetention(Bootstrap|Negotiation)|TestResumedDeliveryBootstrap|TestDeliveryPreflight|TestNativeResumeAcknowledgesRealJournalBeforeNewWork|TestNativeResumeReconcilesJournalUnknownSQLDispatch)')`:
  passed on unchanged production code in 2.486 seconds.
- `python3 scripts/list-docs.py validate`: passed, with 431 decisions and 1920 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, with 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- PR-documentation reference preflight through `validateCoverage`: `covered`,
  with zero errors. The local run used the planned runtime path to exercise
  validation, plus the actual plan, work order, design, and requirement files.
- `git diff --check -- docs/plans/durable-delivery-startup-deadline docs/specs/platform/system-design/durable-agent-stream-processing.md`:
  passed. The untracked-file inventory contains both new package files.

Implementation and permanent regression results passed on 5 November 2026:

- `TestDeliveryStartupNegotiationBudget` and `TestDeliveryStartupNativeBudget`
  complete after 35 virtual seconds. The native case also proves no SQL read
  happens before native success.
- `TestDeliveryStartupMissingAuthority` fails only at the anchored 30-second
  budget.
- `TestDeliveryStartupCancellationAndReplacement` covers initialization
  failure, caller deadline, stream cancellation, manager shutdown, stale
  completion, startup-context cleanup, and authority that commits after the
  budget.
- `TestDeliveryStartupStreamOrdering` drives the real WebSocket lifecycle and
  SQLite repository. The delayed native response keeps its identity, control
  processing stays responsive, and receipts, projections, callbacks, and ACKs
  remain at zero until the generation commit.
- `TestDeliveryStartupAuthorityQueryIsBounded` proves the initial SQL read
  carries the native-anchored deadline, that a released successful attempt
  keeps that anchor for late events, and that an already-expired attempt gets a
  separately bounded authority probe.
- Transient authority-read failures retain bootstrap admission for a reattached
  stream, and a commit landing at the budget boundary is observed by one final
  bounded authority read.
- The bootstrap reset runs under the execution's startup ownership lease.
  `TestDeliveryStartupCancellationAndReplacement` advances a successor during
  the old reset's ownership interval and proves the replacement keeps its own
  native and SQL bootstrap wait.
- `connectUpdatesStream` captures the startup generation and attempt as one pair
  before the replay RPC, and the stream fixture rejects prompt dispatch and
  unexpected extra callbacks.
- Temporary reverts reproduced both the original 30-second failure and the
  unbounded-query and check-then-mutate review defects before the corrections.
- Focused `-race` lifecycle and agentctl suites passed, the full lifecycle
  package passed, `go build ./...` passed, and `scripts/lint-go-changed`
  reported zero issues.

## Risks and remaining evidence

- Completing native readiness too late can create another dependency cycle
  with setup-event projection or an initial prompt.
- Reusing a canceled startup context after native success can reject a valid
  asynchronous generation commit. The connected stream owns that later wait.
- Removing event deadlines without completing error paths can strand a worker.
  Cancellation and shutdown tests are required.
- Loading the latest gate from a shared execution can transfer obsolete work
  to a successor attempt. Tests must force that overlap.
- The local reproduction establishes a matching defect, not the reporter's
  exact cause or an explanation for the original submission mismatch.
- No public-doc change is required for this design-only package. The eventual
  fix preserves current commands, recovery controls, and public configuration.
