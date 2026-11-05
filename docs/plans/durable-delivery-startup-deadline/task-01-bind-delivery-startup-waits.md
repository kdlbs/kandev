---
id: "01-bind-delivery-startup-waits"
title: "Bind delivery waits to the startup attempt"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.12
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.4
system_design:
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
---

# Task 01: Bind delivery waits to the startup attempt

## Summary

Let valid native startup finish before the generation-one SQL authority wait
begins. Keep event admission fenced by real committed ownership and release
pending work when its own initialization fails or stops.

## In scope

- An attempt-scoped completion result for negotiation and native initialization.
- Existing startup context, startup generation, stream lifetime, and bounded
  post-native SQL authority wait.
- Wiring from `InitializeAndPromptWithLayers` through its connected event processor.
- Virtual-time regression tests and a real WebSocket/SQLite ordering fixture.
- Existing bootstrap, legacy, recovery, and relaunch guard regressions.

## Out of scope

- Repairing the original session's unexplained peer/SQL mismatch.
- Removing recovery blocks on Stop or changing queued-message deletion.
- Provider-specific exceptions, UI changes, timeout settings, or schema changes.
- Changes to successor-generation restoration or committed-session adoption policy.
- A commit, push, PR, or live-instance mutation without a later delivery request.

## Acceptance

1. A valid negotiation or native g1 new/load operation can finish after 35
   virtual seconds without an event-induced disconnect. Earlier caller
   deadlines and initialization failures still terminate their own attempt.
2. Before matching SQL authority commits, the fixture records zero receipts,
   projections, callbacks, and ACKs. After native success, a missing commit
   still exhausts the existing 30-second authority budget. Foreign identity
   continues to fail closed, and unresolved prompts are never resent.
3. Stop, shutdown, cancellation, and stale completion release only their
   captured startup or stream. Successful native initialization can complete
   its asynchronous generation commit after startup-context cleanup. A later
   attempt cannot inherit an earlier timer, completion, or teardown.

## Implementation sequence

1. Read this work order, its plan, the scoped backend guidance, and `/tdd`.
2. Mark this work order `in_progress` before changing permanent tests or production code.
3. Add the permanent virtual-time regressions first. Record the expected
   30-second failure before changing production code.
4. Extend the existing gate in `delivery_bootstrap.go`. Capture startup
   generation and initialization context once. Separate negotiation completion
   from native completion, including errors.
5. Bind the connected processor to that exact gate in `streams.go` and
   `stream_event_processor.go`. Do not reload a successor gate to process an
   older connection's event.
6. In `session.go`, complete native success after accepting the returned
   native ID and publishing `AgentACPSessionCreated`. Complete errors on every
   earlier exit. Configuration waits and initial prompt dispatch must not own
   the native completion result.
7. In `durable_delivery_stream.go`, wait for pending native completion before
   starting the existing missing-generation budget. Anchor that budget to
   native completion, so later event arrivals cannot reset it.
8. Keep post-success authority reads under stream ownership. Use the existing
   final bounded-read pattern when native success races with caller cleanup.
   Preserve the ordinary SQL and authenticated event validation path.
9. Add the WebSocket/SQLite fixture and run the exact verification commands.
10. Record results and synchronize this work order and its plan after all checks pass.

## Regression fixtures

`delivery_startup_deadline_test.go` uses `testing/synctest`:

- `TestDeliveryStartupNegotiationBudget`: delay the valid handshake to 35
  seconds under a longer caller deadline.
- `TestDeliveryStartupNativeBudget`: cover the shared event-preparation gate for
  fresh g1, a g1 native restore, and an uncommitted first-launch retry. Delay
  native completion to 35 seconds, then commit matching authority. Native
  `session/new` and `session/load` wiring is covered separately by
  `TestDeliveryStartupStreamOrdering` and the resumed bootstrap fixture.
- `TestDeliveryStartupMissingAuthority`: native success does not grant authority.
  The generation commit never arrives, and the bounded wait fails.
- `TestDeliveryStartupCancellationAndReplacement`: cover initialization error,
  earlier caller deadline, Stop/stream cancellation, manager shutdown, stale
  completion, and context cleanup immediately after successful native startup.
  Pair successful late SQL commit with permanently missing authority.

`delivery_startup_stream_test.go` drives `InitializeAndPromptWithLayers`:

- `TestDeliveryStartupStreamOrdering`: use an actual WebSocket peer and the
  real SQLite repository from `runOwnerTestRepository`.
- Seed a valid task, incarnation, and current execution. Begin with no native
  generation or delivery cursor. Advertise a supported durable peer.
- Send a real early setup event. Hold the native response behind one channel
  and the orchestrator-equivalent generation commit behind another.
- Deliver the native response while event admission waits. Assert that its
  control response reaches the initializer and preserves the returned native ID.
- Keep receipts, projections, event callbacks, and ACKs at zero until the
  generation transaction commits. Then allow normal ordered processing and ACK.
- Capture outbound actions. The no-prompt fixture rejects prompt dispatch and
  asserts that no callback arrives beyond the real setup frame.
- Join all goroutines during cleanup. Do not use wall-clock sleeps to order
  these assertions or add public test-only timeout configuration.

The virtual-time and transport tests jointly prove deadline behavior and
production wiring. An isolated helper test is insufficient by itself.

## Verification

Run this block from the repository root after implementation:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race -count=1 ./internal/agent/runtime/lifecycle -run '^(TestDeliveryStartup|TestDurableRetention(Bootstrap|Negotiation)|TestResumedDeliveryBootstrap|TestDeliveryPreflight|TestNativeResume|TestSilentRestoreDelivery)')
(cd apps/backend && go test -trimpath -tags fts5 -race -count=1 ./internal/agent/runtime/agentctl -run '^(TestStreamUpdates|TestSendStreamRequest|TestCleanupPending|TestResolvePending)')
bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/delivery_bootstrap.go apps/backend/internal/agent/runtime/lifecycle/durable_delivery_stream.go apps/backend/internal/agent/runtime/lifecycle/session.go apps/backend/internal/agent/runtime/lifecycle/streams.go apps/backend/internal/agent/runtime/lifecycle/stream_event_processor.go apps/backend/internal/agent/runtime/lifecycle/delivery_startup_deadline_test.go apps/backend/internal/agent/runtime/lifecycle/delivery_startup_stream_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/durable-delivery-startup-deadline
```

Add every additionally changed Go file to the explicit lint command. If a
named test is renamed, update both the plan and this command before completion.
No package command needs a frontend dependency installation for this work.
Run the PR-documentation coverage preflight against the final changed-file
inventory and this package's requirement/design references.

The local reference preflight uses the repository validator without publishing:

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const documents = [
  'docs/plans/durable-delivery-startup-deadline/plan.md',
  'docs/plans/durable-delivery-startup-deadline/task-01-bind-delivery-startup-waits.md',
  'docs/specs/platform/system-design/durable-agent-stream-processing.md',
  'docs/specs/platform/requirements/durable-agent-delivery.md',
];
const source = 'apps/backend/internal/agent/runtime/lifecycle/delivery_bootstrap.go';
const result = validateCoverage({
  changedFiles: [source, ...documents.slice(0, 3)],
  fileContents: Object.fromEntries(documents.map(path => [path, fs.readFileSync(path, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.status !== 'covered') process.exitCode = 1;
NODE
```

This preflight validates package references using the planned runtime path.
The final PR coverage check must still use its complete actual diff.

## Files likely touched

Existing production files, all under `apps/backend/internal/agent/runtime/lifecycle/`:

- `delivery_bootstrap.go`
- `durable_delivery_stream.go`
- `session.go`
- `streams.go`
- `stream_event_processor.go`

New permanent tests in the same directory:

- `delivery_startup_deadline_test.go`
- `delivery_startup_stream_test.go`

Existing tests can receive small fixture adjustments if the internal gate signature changes:

- `durable_retention_negotiation_test.go`
- `durable_retention_bootstrap_test.go`
- `resumed_delivery_bootstrap_test.go`

`types.go` is needed only if the existing gate pointer cannot retain the
attempt's state. Keep helpers in `delivery_bootstrap.go` where possible.
The orchestrator and SQLite repository provide existing authority operations.
Their production behavior is outside this work order.

## Dependencies

None. PRs #4971 and #4991 are already merged into the recorded baseline.

## Risks

- A completion signal after model projection creates another startup dependency cycle.
- An initialization context can end before the asynchronous SQL commit.
- A gate without complete failure paths can strand a worker during shutdown.
- A replaced attempt must not inherit old events or late teardown.
- Matching the production error signature does not explain the original session's data mismatch.

## Parallelism

`sequential`

## Inputs

- [Plan evidence and scope](plan.md).
- [Delivery startup waits](../../specs/platform/system-design/durable-agent-stream-processing.md#delivery-startup-waits).
- [Resumed bootstrap recovery](../../specs/platform/system-design/durable-agent-stream-processing.md#resumed-bootstrap-recovery).
- [Delivery requirements](../../specs/platform/requirements/durable-agent-delivery.md), especially the listed acceptance criteria.
- Existing `durable_retention_*`, `resumed_delivery_bootstrap_test.go`,
  `delivery_stream_bootstrap_*`, and `native_resume_*` fixtures.
- `awaitSilentRestoreGeneration` in `durable_silent_restore_generation.go`
  for the cancellation-versus-commit read pattern.
- The current `agentctl` stream reader and pending-request cleanup tests.

## Results

Completed on 5 November 2026.

- `delivery_bootstrap.go` now owns one `deliveryStartupAttempt` with separate
  negotiation and native results, the captured initialization context, and the
  captured startup generation. Successful native completion anchors the single
  30-second SQL authority budget; failure completes both waits with its cause.
- `InitializeAndPromptWithLayers` publishes native success after accepting the
  returned native ID and publishing `AgentACPSessionCreated`. Configuration
  waits and prompt dispatch no longer own that result.
- `streams.go`, `stream_event_processor.go`, and `durable_delivery_stream.go`
  carry the captured attempt into every prepared event. `connectUpdatesStream`
  captures the attempt together with its startup generation before the replay
  RPC. Replays and direct callers keep their existing entry points. `types.go`
  replaces the private negotiation pointer with the startup-attempt pointer.
- A successful attempt remains attached to the execution so queued or replayed
  setup events keep its native completion anchor. Failed attempts are cleared.
- `delivery_startup_deadline_test.go` covers the 35-second virtual-time
  negotiation and native budgets, missing authority, cancellation, replacement,
  startup cleanup, an authority commit after the budget, bounded initial and
  expired-attempt authority reads, retry after a transient read failure, a final
  authority commit at the budget boundary, and a deterministic successor overlap
  during the bootstrap reset.
- `delivery_startup_stream_test.go` drives the production lifecycle over a real
  WebSocket and SQLite repository. It holds the native response and generation
  commit at separate barriers, asserts zero delivery evidence before authority,
  and observes the resulting receipt, cursor, callback, and ACK.
- The initial authority read now shares the native-anchored deadline; an
  already-expired attempt uses a separate short probe instead of the stream
  lifetime. Releasing a successful attempt retains its anchor, so a late event
  cannot restart the budget. A transient authority-read failure keeps bootstrap
  admission set, and an expiring budget gets one final bounded authority read so
  a commit in the last poll interval is observed.
- The stream fixture rejects prompt dispatch and asserts that only the real
  setup frame reaches the callback.
- `clearDeliveryBootstrapForAttempt` now runs its ownership check and flag
  mutation as one `withStartupAttempt` lease, so a successor cannot advance
  between them and inherit a cleared bootstrap.
- Temporary reverts reproduced the pre-fix 30-second failure, the unbounded
  initial query, and the check-then-mutate bootstrap race.
- Verification passed: focused lifecycle and agentctl `-race` suites, the full
  lifecycle package, `go build ./...`, `bash scripts/lint-go-changed`,
  `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.test.py`,
  `python3 scripts/lint-spec-files.py --all`, and `git diff --check`.
