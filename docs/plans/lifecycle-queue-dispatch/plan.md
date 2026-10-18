---
created: 2026-10-19
status: complete
requirements:
  - REQ-UI-CI-PR-AUTOMATION-001
system_design:
  - ../../specs/ui/system-design/ci-pr-automation-01.md
legacy_specs: []
---

# Implementation Plan: Dispatch lifecycle queue prompts

## Overview

Repair [issue #4551](https://github.com/kdlbs/kandev/issues/4551) by selecting
the ordinary delivery-claim updater only for the existing non-lifecycle path.
One sequential work order owns regression tests, the minimal correction, and
targeted validation. ROOT reviewed the complete design package and later explicitly released
implementation in this same primary session with a bounded heavy lease.

## Evidence and assumptions

Canonical issue read, including its environment and proposed alternatives; no
comments or image attachments were present. HEAD and remote main were both
`530493cce701430cb9b68387eadd1d0126e78c63` at design intake.
The issue reports SQLite, ACP agents advertising `durable_v1`, and a lifecycle
head repeatedly requeued as superseded, obstructing `on_turn_complete`.

Source confirms `reserveRetainedHead` retains lifecycle rows without an ordinary
attempt, while `executeQueuedMessageWithReservation` always installs an updater
requiring that attempt. `persistDeliveryClaim` maps the rejection to superseded
under both legacy and v1 preparation, before executor dispatch.

ROOT supplied independently qualified real-consumer evidence: SQLite lifecycle
failed with zero ordinary claims, zero executor calls, one pending row; memory
lifecycle and SQLite ordinary controls passed. This session has not rerun that
experiment and has not accessed ROOT's private fixture. Author permanent tests
independently from production behavior after implementation authorization.

Assumption check: dispatch through negotiated transport, preserve existing
guards, and exclude async drain redesign are settled by the request. The owning
pair remains the existing PR automation requirement/design despite its migrated
UI directory; Platform's ordinary claim contract is a dependency, not a second
repair specification. Existing control labels and outcome criteria do not
explicitly cover retained delivery, so add only `.18` to the existing requirement.
No new ownership or persistence model is proposed; no new ADR is needed.

## Scope

### In scope

- `AC-UI-CI-PR-AUTOMATION-001.18`: real SQLite lifecycle consumer regression,
  legacy and advertised compatible `durable_v1`, accepted acknowledgement,
  transient unaccepted retry, and invalidation controls.
- Preserve ordinary claim persistence and fail-closed rejection; explicitly
  characterize non-lifecycle retained-kind updater eligibility.

### Out of scope

Async drain/on-turn-complete architecture, startup reconciliation changes,
new schema/claim/retry mechanisms, all-retained-row bypass, plan-comment or
managed-input delivery repair, exactly-once crash recovery, provider adapter
changes, UI/copy/config/public API/feature flags, unrelated paused tasks, and
foreign refs/processes/caches/worktrees.

## Technical approach

In `apps/backend/internal/orchestrator/event_handlers_agent.go`, construct
`promptTaskOptions.deliveryClaimUpdater` only for non-lifecycle entries, using
the existing `isLifecycleAutomationMessage`/`IsDurableLifecycle` classifier.
Preserve all other options and callbacks. Do not infer eligibility from an empty
attempt ID: a missing ordinary attempt must continue to reject dispatch.
Do not modify SQL claim validation or `persistDeliveryClaim` error mapping.

| Queue kind / transport | Identity and intended behavior | Evidence |
| --- | --- | --- |
| Lifecycle / legacy peer | Retained lifecycle reservation; no ordinary updater; accept then acknowledge | SQLite consumer success and failed unaccepted handoff |
| Lifecycle / compatible advertised v1 | Same reservation; negotiate v1, prepare real backend submission/hash, pass matching identity to mocked transport, acknowledge acceptance | SQLite consumer success, persisted submission, recovery-block control |
| Lifecycle / unresolved or incompatible peer | Existing recovery block; no transport send, no acknowledgement as success | Consumer negative control |
| Ordinary user / legacy and v1 | Ordinary attempt and claim-to-submission association remain mandatory | Valid, missing, and stale claim consumer controls |
| Plan comment / either mode | Retained replay receipt and attempted boundary; updater selection unchanged | Hook eligibility and existing persistence/failure/no-replay tests |
| Managed input / either mode | Retained receipt with exact turn/generation; updater selection unchanged | Hook eligibility and existing attempted-input/settlement tests |

`IsDurableDelivery` is a retention classifier, not a transport capability.
Plan-comment and managed-input rows also reserve without ordinary claims, so a
similar incompatibility may exist there. That is explicitly not silently fixed
by this lifecycle-only package. If lifecycle recovery itself requires a new
persisted claim-to-submission association, stop and checkpoint ROOT with that
boundary rather than disable durable negotiation or invent persistence.

## Tests and end-to-end evidence

Add `apps/backend/internal/orchestrator/event_handlers_queue_lifecycle_delivery_test.go`
with independent real SQLite task and queue repositories, the real queue service,
reservation, task-service transcript/turn persistence, prompt preparation and
consumer. Mock external agent transport; reuse the existing workflow lookup
fixture with no configured step actions. Use bounded channel barriers and join every
consumer; prevent uncontrolled automatic follow-up drains in failure cases.

- `TestQueuedLifecycleDeliverySQLite`: table of review-requested, merged, closed,
  legacy/v1 modes; no ordinary claim before/after; one
  transport call; row retained at transport boundary; accepted row removed and
  no pending head afterward. For v1, assert prepared submission/hash and the
  matching transport options. Configure the real CI attempt binding when needed.
- `TestQueuedLifecycleDeliveryCIAutoFix`: real GitHub Store/Service and task
  service turns bind the queued CI attempt to its accepted turn in both modes;
  acceptance removes the retained row without creating an ordinary claim.
- `TestQueuedLifecycleDeliveryFailure`: unaccepted transient failure retains one
  retry, invalidated/archive/reset/session-replacement controls never send, and
  durable recovery-blocked peer never falls back to legacy. Existing targeted
  lifecycle tests remain part of verification.
- `TestQueuedOrdinaryDeliveryClaimControls`: real valid ordinary claim succeeds
  in both modes and records its protocol/submission; missing/deleted or stale
  attempt rejects before transport. Never suppress claim-changed errors.
- `TestQueuedDeliveryUpdaterEligibility`: hook eligibility includes ordinary,
  plan-comment and managed-input rows and excludes lifecycle rows (including
  legacy provider-origin fallback); this test alone is not consumer evidence.
- Reuse `TestProcessOnTurnComplete_ExplicitSignalGating` in
  `event_handlers_step_completion_test.go`: real task/session persistence and
  production completion processing prove absent/stale signals block and a
  matching signal advances. Reuse
  `TestReviewWatchLifecycle_BootReadyStaysOnReviewUntilTurnComplete` in
  `event_handlers_github_review_test.go`: boot-ready does not advance, genuine
  turn completion does. These are compatibility controls, not integrated
  lifecycle transport simulations. The new consumer test proves accepted
  lifecycle rows no longer leave a blocking head. No duplicated workflow suite
  is planned for unchanged completion logic.

All new behavior maps to `.18`; `.9` and `.10` remain compatibility constraints
for auto-fix coalescing and exact accepted-turn binding. The SQLite consumer provides backend end-to-end delivery evidence; existing
completion controls independently verify unchanged signal/ready semantics. No browser rendering changes
require a synthetic Playwright test. Mock transport evidence does not certify
every provider or process-crash replay scenario.

## Work orders

- [x] [Task 01: Dispatch retained lifecycle prompts](task-01-dispatch-lifecycle-prompts.md)

## Verification results

Implementation and task-defined local checks are complete. Publication and
hosted validation are tracked in the external task plan. All commands use the work order's task-local toolchain and
bounded resource settings.

- Causal RED: exit 1, all six lifecycle legacy/v1 event cases failed with zero
  transport calls despite a retained reservation and no ordinary claim.
  Test runtime 6.692s; compilation and test execution completed normally.
- Initial GREEN: exit 1, four v1 test assertions distinguished nil from an empty
  attachment slice after JSON omission. Corrected the test oracle; production
  fix unchanged.
- Refined GREEN: exit 1, only two retained-kind controls expected the superseded
  log even though post-attempt failure handling returns earlier. Corrected the
  fixture to use exact production queue identity and assert protocol preparation
  plus no transport. Production fix unchanged.
- Final targeted GREEN: exit 0, `go test -race -trimpath -tags fts5 -p=1
  ./internal/orchestrator` with the work-order selector, `-count=1
  -timeout=300s` and GNU outer 420s/kill10. Test runtime 3.537s. Original
  native handle 28156 joined; observed owned wrapper/timeout PIDs absent.
- Scoped lint: original exit 4 after reporting `0 issues` and an internal
  `Timeout exceeded` error. The canonical script's 5-minute timeout did not
  produce a successful result. No unchanged retry. ROOT subsequently granted
  one attempt with the same package/config/diff base and a larger resource and
  deadline budget; that attempt passed with original exit 0 (`0 issues`).
- Authorized lint: `golangci-lint run ./internal/orchestrator
  --new-from-rev=530493cce701430cb9b68387eadd1d0126e78c63 --timeout=10m
  --concurrency=2`, GNU outer 620s/kill10, task-local GOMEMLIMIT3GiB,
  GOMAXPROCS2 and GOFLAGS=-p=2. Original native 48964 joined exit 0; observed
  wrapper absent. Canonical shared script unchanged.
- One frozen install: original native 87426 joined exit 0, pnpm 9.15.9, 1.7s.
  Normal hooks remain active; their receipts belong to the external task plan.
- Cheap design validation: catalog (388 decisions, 1656 specifications), spec
  lint, diff check and local PR-docs `validateCoverage` all passed. The initial
  preflight found `.18` outside its owning requirement section; moving it into
  that section produced covered status with zero errors.

The external task plan owns original process handles, log locations, joins,
lease and hosted-observer checkpoints. No tests or heavy work ran during design.

## Risks

- In-memory queues omit the ordinary SQL updater, hiding this defect.
- A metadata-wide retained bypass would alter unrelated receipt contracts.
- A v1 test that only calls `prepareAgentDeliverySubmission` misses this bug;
  the consumer must reach the transport seam and verify SQL effects.
- Retained lifecycle rows do not acquire ordinary restart recovery guarantees
  by skipping an inapplicable hook. Preserve the existing recovery block.

## Documentation impact

Only the owning internal requirement/design pair and this delivery package need
updates. Public controls, copy, configuration, flags and API contracts remain
the same; this restores the documented PR automation delivery behavior. Related
outcome retry/scope plans keep their completed statuses and results.
