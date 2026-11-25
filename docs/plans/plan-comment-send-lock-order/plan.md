---
created: 2026-11-03
status: complete
requirements:
  - REQ-TASKS-PLAN-COMMENTS-002
  - REQ-TASKS-PLAN-COMMENTS-003
system_design:
  - ../../specs/tasks/system-design/plan-comments.md
legacy_specs: []
---

# Implementation plan: Plan comment Send delivery

## Overview

Release session admission before notifying the orchestrator about accepted plan
feedback. Preserve the retained-receipt path during agent admission. One sequential
work order owns both corrections and regression coverage. Tasks owns prompt acceptance and comment
consumption. Existing requirements already define the intended delivery.

## Reproduction evidence

Investigation used commit `4a8db14fa4eb2d1bea1621345d2ef8a41a24abc3`.
`git ls-remote origin refs/heads/main` showed that this was remote main on
2026-11-03. The same nested notification exists in the locally running older
release, `cc687bdce96b`. No affected user incident was available for correlation.

In `apps/backend/internal/task/handlers/message_handlers.go`, `wsAddMessage`
calls `NotifyQueuedUserPrompt` inside its `createMessage` closure. The closure
runs inside `WithInitialTaskBriefAdmission`. That method holds the session
queue gate. Notification synchronously reaches
`drainQueuedMessageForPromptableSessionWithTaskAdmissionAndIdentity`, which
requests the session cancellation guard.

Lifecycle drains hold that cancellation guard before entering queue admission.
A concurrent drain can therefore wait for the queue gate while the submitting
handler waits for the cancellation guard. Neither operation can progress until
the queue waiter cancels or exhausts its admission timeout. The browser request
can time out first.

Temporary tests proved both parts of this chain:

- `TestRepro_PlanCommentMessageNotifiesAfterAdmission` entered the real
  `wsAddMessage` handler with a real SQLite repository. Its assertion failed
  because notification ran before the admission callback returned.
- `TestRepro_PlanCommentNotifyCompetesWithLifecycleDrain` held the cancellation
  guard while another operation held session admission. The captured stacks
  showed notification waiting in `cancelInFlightMutex.LockContext` and the
  competing operation waiting in `sessionAdmissionGate.lock`.
- `TestRepro_PlanCommentNotifyAfterAdmissionAvoidsLockCycle` used the same
  competing locks but notified after admission returned. It passed.

The concurrency test controlled the competing lifecycle lock sequence. It did
not reproduce a particular user's incident or force the race in a browser.
The investigation removed these temporary tests before the design handoff.

The stronger desktop test reproduced a second failure during implementation.
The server saved the user message and consumed its comments, but no agent reply
followed. The isolated backend logged prompt preparation without agentctl
acceptance. `executeQueuedMessageWithReservation` attached an ordinary queue
claim updater to the retained comment receipt. SQLite rejected its missing
dispatch attempt as `ErrQueueDispatchClaimChanged`. Admission treated this as
superseded work and returned before provider I/O.

## Scope

Repair the notification boundary for direct comment-bearing message admission,
including the shared preview-feedback branch. Preserve durable receipts, exact
comment consumption, replay identity, initial-brief ownership, queue policy,
selected-session Send, and primary-session Run.
Exclude plan-comment receipts from ordinary queue claim updates. Other queue
producers retain their existing validation rules.

This repair needs no schema, protocol, provider, configuration, localization,
or rendered UI changes. Existing completed comment persistence, recovery,
foreground-preservation, and session-switch packages remain historical.
This package owns the new repair and its verification results.

## Technical approach

Keep message creation and initial-brief tracking inside their current admission
callback. After successful admission returns, notify once when the existing
eligibility condition permits it. Use the caller context outside admission.
Do not retain a callback context that claims a released queue gate.

Preserve the current notification eligibility for a missing initial-brief
candidate, a selected candidate, or an accepted first prompt. A rejected or stale
attempt must not notify. A losing first-brief contender must retain its current
deferral behavior. An exact replay must continue to wake its existing receipt
without creating a second message or consuming comments again.

The correction changes the caller's ordering. It does not add a notification
goroutine, change cancellation intent, or change queue admission timeouts.
The existing synchronous notification API remains usable after admission.
The system design records the ordering rule. This local correction does not
need a new architecture decision.

Implementation coverage also exposed a stale-brief retry error in the same
handler. The retry rebuilt its candidate from `req.Content` after the handler
inserted a reserved plan-comment marker. Adding another marker caused
`plan comment placeholder must occur exactly once`. Retain the original
request before preparing stored content. Rebuild stale candidates from that
snapshot, then notify only after final acceptance. The regression failed
before this correction and passed after it.

Skip `deliveryClaimUpdater` for durable plan-comment receipts in queued dispatch.
Plan comments bypass ordinary claims. Lifecycle prompts keep their existing
handling, and managed inputs retain ordinary claim validation in this path.
Keep protocol negotiation and backend durable submission persistence for these
prompts. A real-SQLite orchestrator
regression must reach the agent admission callback for direct and queued
comments with legacy and durable agents. Ordinary messages are the positive
control for their existing claim persistence.

## Tests

| Contract | Required evidence |
| --- | --- |
| `AC-TASKS-PLAN-COMMENTS-002.2`, `003.1`, `003.3` | Real handler regression proves notification occurs after admission releases its session gate. |
| `AC-TASKS-PLAN-COMMENTS-002.5` | Committed message and receipt contain the resolved feedback. Acceptance consumes included comments once. |
| `AC-TASKS-PLAN-COMMENTS-002.6` | Rejection and stale retry do not notify. Exact replay does not duplicate delivery. |

Permanent regression tests must enter `wsAddMessage`. A test that manually calls
notification inside admission would remain broken after repairing the handler.
Use a gated coordinator in the handler fixture to observe the real call boundary.
Keep the real SQLite queue schema in that fixture.

## E2E tests

Strengthen the existing Send/Run routing scenario in
`apps/web/e2e/tests/session/task-plan-comments.spec.ts` (`chromium`) and
`apps/web/e2e/tests/session/mobile-task-plan-comments.spec.ts` (`mobile-chrome`).
Capture the prior agent-message IDs before each submission. Require fresh
agent output for the accepted submission in its intended session, then wait
for that turn to settle. Persisted user text and an idle composer alone do not
prove agent delivery. Keep secondary Send and primary Run distinct.

The phone path retains its shipped Plan Drawer and visible Send control.
The composition stays the same. Omit a UI preview and visual redesign.

## Work orders

- [x] [Task 01: Restore accepted plan feedback delivery](task-01-notify-after-admission.md)

Execute sequentially in the primary session. The user did not authorize delegation.

## Verification results

Investigation completed on 2026-11-03:

- The real-handler ordering assertion failed as expected (0.346 seconds).
- The controlled lock-cycle reproduction failed as expected (1.251 seconds).
- The temporary notification-after-admission experiment passed (0.380 seconds).
- The existing desktop Send/Run scenario passed: 1 test, 14.2 seconds total.
- The existing phone Send/Run scenario passed: 1 test, 25.5 seconds total.

The browser results establish normal routing only. They do not cover the
controlled concurrency failure or prove a production correction.
Initial browser setup failures came from inherited startup configuration and
missing Chromium library paths. The investigation removed both setup blockers
before the recorded passes.

Implementation added both production corrections and permanent regressions.
The handler race suite passed in 3.189 seconds. The orchestrator race suite
passed in 4.653 seconds, including all six legacy/durable admission cases.
Before the dispatch correction, all four comment cases failed to reach the
agent while the two ordinary-message controls passed.

The stronger desktop browser test reproduced a saved message with no fresh
agent reply before the dispatch correction. After that correction, the same
scenario passed: one test in 13.8 seconds. The phone scenario passed against
the same build: one test in 15.7 seconds. Both now require fresh agent replies
in the intended session and exactly one feedback transcript per submission.

E2E ESLint, catalog validation, full specification lint, documentation coverage
preflight for both production files, and `git diff --check` passed. Changed-Go
lint passed with zero issues using CI-pinned golangci-lint v2.9.0.
All owned isolated test instances stopped. Cleanup removed temporary logs and
diagnostic captures. The working tree retains the source and test changes for review.

Design-package validation passed: catalog validation, all 36 specification
linter tests, full specification lint, documentation coverage preflight for
the planned handler change, and `git diff --check`. The STE text check found
no violations in the plan or work order. The investigation removed temporary
reproduction files and the browser-library path file. The isolated browser instances
stopped, leaving the original instance list intact.

## PR review follow-up

The updater bypass now applies only to durable plan comments. Managed inputs
keep their existing ordinary claim validation. The CI eligibility test now
requires one plan-comment dispatch and zero managed-input dispatches. The broad
bypass failed the managed-input assertion locally before this correction.

Added real-handler delivery and deferred-contender coverage. Removing the
asynchronous completion call makes queued feedback remain undelivered. The
mutation probe restored the handler source byte-for-byte. Final focused race
suites passed for handlers in 3.094 seconds and the orchestrator in 6.663 seconds.
The spec restores guard scope and connection rationale and links receipt terms
to their defining section. Normal commit hooks must pass before publication.
Remote CI and review resolution remain external verification for the pushed head.

## CI integration follow-up

The complete backend and frontend workflows passed after the review correction.
Browser CI exposed a timing failure in the existing background follow-up fixture.
The screenshot showed that the follow-up finished before the browser could
enter queue mode. The focused test passed locally on its first attempt.

The branch merges the upstream fixture correction from main. It holds the mock
tool open until the browser writes a release marker after queue admission.
The merged handler,
orchestrator, and mock-agent race tests pass (2.753, 6.911, and 1.152 seconds).
The merged background follow-up test passed without retries in 14.7 seconds.
Desktop Send/Run passed against the same fresh build in 12.6 seconds.
Phone Send/Run passed without retries in 15.6 seconds.

Kubernetes 1.34 compatibility also failed while Kind loaded the fixture image,
before the application test started. Fresh CI must recheck that setup failure.
CI and review evidence from the previous head is historical.

## Risks

- Moving notification must preserve initial-brief contender and replay behavior.
- Reusing an admitted callback context after release would misrepresent lock ownership.
- Browser coverage must prove new agent output rather than only transcript persistence.
- Plan-comment receipts must not require an ordinary queue dispatch claim at agent admission.
