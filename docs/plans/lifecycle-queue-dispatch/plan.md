---
created: 2026-10-19
status: completed
requirements:
  - REQ-UI-CI-PR-AUTOMATION-001
system_design:
  - ../../specs/ui/system-design/ci-pr-automation-01.md
legacy_specs: []
---

# Implementation Plan: Dispatch lifecycle queue prompts

## Overview

Repair [issue #4551](https://github.com/kdlbs/kandev/issues/4551) by keeping
retained lifecycle dispatch independent of ordinary queue claims. One work order
owns the minimal updater correction, SQLite consumer coverage and accurate
legacy/durable recovery documentation.

## Root cause

In baseline `530493cce701430cb9b68387eadd1d0126e78c63`,
`reserveRetainedHead` retains lifecycle rows without an ordinary dispatch
attempt, while `executeQueuedMessageWithReservation` always installs an updater
requiring that attempt. `persistDeliveryClaim` maps the SQL rejection to
superseded before executor dispatch. The lifecycle event repeatedly requeues
and obstructs normal subsequent turn-completion processing.

The causal SQLite regression failed in all six review-requested/merged/closed
legacy/v1 cases with zero transport calls despite a retained lifecycle row.
The correction conditionally attaches the ordinary updater only when
`!lifecyclePrompt`, preserving SQL claim validation and error mapping.

## Scope

### In scope

- `AC-UI-CI-PR-AUTOMATION-001.18`: initial SQLite lifecycle delivery through
  negotiated legacy or compatible `durable_v1`, retained-row acceptance,
  legacy failed-handoff retry, conservative durable-error recovery and existing
  archive/reset/ownership guards.
- Ordinary valid/missing/stale claim controls and non-lifecycle retained-kind
  compatibility. Existing auto-fix coalescing and exact-turn binding remain
  constrained by `.9` and `.10`.

### Out of scope

Async drain/on-turn-complete redesign, startup reconciliation changes, new
schema/claim/retry mechanisms, all-retained-row bypass, plan-comment or managed
input repair, runtime/agentctl outcome propagation, shared ordinary/direct
settlement changes, exactly-once crash guarantees, UI/copy/config/public APIs,
feature flags and unrelated resources.

## Technical approach

In `apps/backend/internal/orchestrator/event_handlers_agent.go`, select
`promptTaskOptions.deliveryClaimUpdater` with the existing lifecycle classifier.
Keep final active-task/current-token/generation checks and accepted
acknowledgement. Never infer ordinary eligibility from a missing attempt ID or
suppress an ordinary claim-changed error.

| Queue kind / transport | Intended behavior | Evidence |
| --- | --- | --- |
| Lifecycle / legacy | Retain reservation until acceptance; retry a known unaccepted handoff | SQLite acceptance and failed-handoff control |
| Lifecycle / compatible v1 | Persist submission/hash, deliver and acknowledge acceptance | SQLite consumer and payload/hash assertions |
| Lifecycle / ambiguous v1 error | Retain event and interrupted-unknown submission behind the matching recovery block; reject another admission | Durable-error consumer regression |
| Lifecycle / unresolved capability | Preserve recovery block without legacy fallback | Existing consumer negative control |
| Ordinary / legacy and v1 | Require ordinary claim and association; missing/stale attempts fail closed | Ordinary claim consumer controls |
| Plan comment / managed input | Keep existing receipt, attempted-delivery and updater behavior | Eligibility and existing receipt/no-replay controls |

`IsDurableDelivery` classifies row retention, not transport capability.
Lifecycle delivery still negotiates protocol and preserves payload, generation,
ownership and recovery checks. Other retained kinds do not silently acquire a
new delivery contract through this correction.

## Existing durable classification limitation

[Review finding 4309336036](https://github.com/kdlbs/kandev/pull/4574#discussion_r4309336036)
identifies a valid shared limitation already present in baseline
`530493cce701430cb9b68387eadd1d0126e78c63`:

- `apps/backend/internal/agentctl/server/process/submission_delivery.go:19-25,128-138`
  recognizes known/deterministic failures and can record `SubmissionFailed`.
- `apps/backend/internal/orchestrator/task_operations.go:7554-7567` marks durable
  executor errors interrupted-unknown and opens `unknown_prompt_outcome`.
- `apps/backend/internal/orchestrator/agent_delivery_submission.go:40-55`
  owns that backend transition; `session_native_delivery_recovery.go:112-127`
  restricts existing native reconciliation for automation-owned retry sessions.

The bounded repair does not fix that shared classification gap. It restores
initial delivery and accepted acknowledgement while preserving conservative
recovery whenever an authoritative backend terminal outcome is unavailable.
Typed outcome propagation and ordinary/direct settlement changes are outside
this package. Neither `dispatchAccepted == false` nor error text proves safe
retry. An authoritative terminal outcome remains subject to existing settlement
and admission rules; the contract does not require permanent blocking.

## Tests and end-to-end evidence

All new tests live in
`apps/backend/internal/orchestrator/event_handlers_queue_lifecycle_delivery_test.go`.
The fixture uses real SQLite task/queue repositories, queue service, task-service
transcript/turn persistence and prompt preparation, with mocked agent transport.
Consumers run synchronously without an uncontrolled follow-up retry loop.

- `TestQueuedLifecycleDeliverySQLite`: review-requested/merged/closed across
  legacy/v1; no ordinary claim, retained row at handoff, accepted acknowledgement
  and matching persisted v1 payload/hash.
- `TestQueuedLifecycleDeliveryCIAutoFix`: real GitHub Store/Service bind the
  queued attempt to its accepted task-service turn without an ordinary claim.
- `TestQueuedLifecycleDeliveryFailure`: known unaccepted legacy failure retains
  one retry; archive and unresolved-capability controls preserve their guards.
- `TestQueuedLifecycleDeliveryDurableError`: a generic ambiguous transport error
  produces a persisted interrupted-unknown submission and recovery block with
  matching session/submission identities. The event is retained without an
  accepted acknowledgement. A second real `queueAndDrainLifecyclePrompt`
  admission returns recovery-required, sends nothing and preserves the evidence.
  This characterizes existing conservative behavior; it does not manufacture
  proof that agentctl recorded a known terminal failure.
- `TestQueuedOrdinaryDeliveryClaimControls`: valid claims persist transport
  association; missing/stale claims reject before transport in both modes.
- `TestQueuedDeliveryUpdaterEligibility`: retained non-lifecycle kinds reach
  protocol preparation while keeping their existing updater rejection.

Existing `TestProcessOnTurnComplete_ExplicitSignalGating` and
`TestReviewWatchLifecycle_BootReadyStaysOnReviewUntilTurnComplete` establish
unchanged completion/ready semantics. The SQLite consumer provides backend
end-to-end delivery evidence; no browser rendering changes require Playwright.
Mock transport does not certify every provider or process-crash replay scenario.

## Work orders

- [x] [Task 01: Dispatch retained lifecycle prompts](task-01-dispatch-lifecycle-prompts.md)

## Verification results

The published updater repair passed its original targeted race/FTS5 tests
(3.537s test runtime), owning-package lint, formatting, catalog/specification and
documentation coverage checks, and normal commit hooks. Causal RED failed with
zero lifecycle transport calls before the correction. Two intermediate test
oracle failures (nil/empty JSON attachments and post-attempt log ordering) were
corrected without additional production changes.

The initial canonical lint exited 4 after an internal timeout, despite reporting
`0 issues`; it was not counted as a pass. The same package/config/diff-base lint
subsequently passed with a 10-minute timeout. The workspace frozen install also
passed and the normal delivery hooks remained enabled.

The durable-error characterization and legacy failure controls passed the
affected race/FTS5 selector (1.503s test runtime). Canonical changed-file lint
passed with zero issues; catalog/specification, documentation coverage and diff
checks passed. The work order records the correction and validation details.

## Risks

- In-memory queues omit the ordinary SQL updater and hide the original defect.
- A generic retained-row bypass would change unrelated receipt contracts.
- Legacy unaccepted retry coverage cannot prove safe automatic durable retry.
- Preserve interrupted-unknown evidence and recovery blocks until authoritative
  existing reconciliation permits admission; do not infer certainty from a
  missing dispatch callback.

## Documentation impact

Only the owning internal requirement/design pair and this delivery package need
updates. Public controls, copy, configuration, flags and API contracts remain
unchanged. Related outcome retry/scope plans retain their completed results.
