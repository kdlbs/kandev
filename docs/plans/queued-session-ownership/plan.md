---
created: 2026-09-16
status: in_progress
requirements:
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-001
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-002
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-003
system_design:
  - ../../specs/tasks/system-design/queued-session-ownership.md
legacy_specs: []
---

# Implementation plan: Queued session ownership

## Overview

Keep Luna queued when a workflow enters Implement, keep the parked Astra
conversation untouched during inspection, and make waiting visible on desktop
and phone. Deliver three sequential outcomes: inspection safety, queue lifecycle
correctness, then a complete queue projection and UI.

The task system owns this package because the deferred workflow entry owns its
recipient and state. Agent capacity and workflow WIP policy remain separate.
Read the [requirements](../../specs/tasks/requirements/queued-session-ownership.md),
[design](../../specs/tasks/system-design/queued-session-ownership.md), and
[decision](../../decisions/2026-09-16-passive-session-inspection.md).

## Evidence and conformance

Read-only investigation of task `ef15115f-12b9-4f8e-ba9f-bc0cd7e01bd8`, using
stored conversations, session lists, and backend build `1293d65a8e` (dirty):

| Lisbon time, 2026-09-16 | Observed event |
| --- | --- |
| 21:15:44 | Manual move to Implement selects new primary Luna session `c7451c7d-8468-4b45-8c67-ef6427390c01`; task becomes SCHEDULING. |
| 21:15:48 | Astra `8bc0ec46-6aef-43a5-b8f2-56c64cd680d0` is parked; Luna automatic launch is deferred at 5/5. |
| 21:19:02 | Task open/focus and status checks precede Astra resume by about 135 ms. Resume is classified manual and admitted at 8/5. |
| 21:19:06 | Astra boot-ready changes it to WAITING_FOR_INPUT, then changes the task from SCHEDULING to REVIEW. Luna remains CREATED. |
| Through 21:23:00 | The scheduler continues retrying Luna every 20 seconds; its accepted queue entry survives. |

The backend evidence does not prove a scheduler prompt to Astra. It shows a
resume through the shared launch path. Timing and frontend source support
open-time resumption; the exact user click and empty-output warning event remain
unproven. Do not encode either as an established cause.

The original diagnostic archive is task-local, not a required implementation
input: `.kandev/diagnostics/c79c09e35c0b6b608246d9280e0d220f.zip`. Its manifest
reports truncation of older logs; the incident window was present.

Confirmed defects are `buildResumeRequest` sharing manual admission with passive
recovery, and `writeTaskReviewState` ignoring a CREATED deferred destination.
Existing lifecycle AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.4 defines parking but
does not explicitly exclude inspection. The new requirement pair defines that
gap and queue visibility. Manual overrides remain allowed by
AC-AGENTS-SESSION-CEILING-001.2.

## Scope

### In scope

- Durable workflow parking, explicit passive intent, guarded status/ensure/resume.
- Exact queued recipient and entry identity; state reconciliation and replay races.
- Revisioned queue details in sidebar, task details, and phone navigation.
- Regression tests, restart evidence, localization, and user documentation.

### Out of scope

- Hard limits for explicit manual launches, new limits/settings, queue ordering or ETA.
- A global queue page, Office scheduling, provider policy, or new session states.
- Changes to source/destination profile selection or workflow WIP admission.
- Rewriting historical transcript warnings or assuming the unexplained warning's cause.

## Technical approach

Task 01 adds parking metadata and passive-source handling end to end, including
status/launch/ensure eligibility and the browser recovery hook. Task 02 binds
workflow deferrals to their entry and guards state and clear operations. Task 03
extends `task/statussummary` and delivers all queue surfaces together.

No new SQL column or scheduler is planned. Conditional operations on existing
metadata and task state must be tested through both supported database contracts.
Never clear callback tombstones to implement unpark. Never use background-work
parking as workflow parking. Existing old records need conservative migration
or passive suppression, not guessed ownership.

## ASCII UI preview

### UI-01: Desktop task row and detail, queued destination

Entry: move to Implement, then reopen the task or select Astra.

```text
Before (observed)             After
Review                       Scheduling
  Investigate issue            [clock] Investigate issue  Queued

+-------------------------------------------------------------------+
| Investigate issue                                      Implement  |
| Queued: Luna. Waiting for session capacity                         |
| 5 of 5 in use. Checked just now. Queued since 21:15                 |
| Starts automatically when capacity is available.                   |
| [Astra] [Luna: Queued] [Plan]                                      |
| Parked for workflow. Send a message to continue this conversation.  |
| Astra conversation ...                                            |
+-------------------------------------------------------------------+
```

The task queue region is outside conversation scroll and remains when Astra is
selected. The parked note is conversation-specific. Selecting Luna removes
the parked note but keeps queue status; it does not show an unsolicited Start
or Resume prompt. Copy and spacing are illustrative; placement and identity are required.

### UI-02: Phone task navigation and detail

```text
Task navigator (existing bottom drawer)
+------------------------------------+
| Tasks                         Close|
| [clock] Investigate issue   Queued  | <- scrolling task list
+------------------------------------+

Task detail (direct navigation)
+------------------------------------+
| Investigate issue        Implement |
| Queued: Luna                       |
| Waiting for session capacity       |
| 5 of 5. Checked just now.           |
| Queued since 21:15                 |
| Starts automatically.              |
| [Astra                         v]  | <- existing session picker
| Parked for workflow.               |
| Send a message to continue here.   |
| Conversation ...                   | <- single chat scroll owner
| Composer                           |
| Chat | Plan | Changes | More       | <- existing safe-area nav
+------------------------------------+
```

Reuse `SessionTaskSwitcherSheet`, `TaskSwitcherDrawer`, `MobileSessionsPicker`,
and the dedicated mobile layout. No stacked desktop panes or extra disclosure
drawer. Required queue text is visible without hover; existing navigation owns
touch/focus behavior. New interactive targets, if needed, are at least 44 px on
phones/coarse pointers; ordinary desktop sizing remains unchanged.

### UI-03: Freshness and settlement, both viewports

```text
Loading:       Queue status loading... (do not offer a duplicate launch)
Disconnected:  Queued: Luna. Reconnecting. Capacity last checked 21:16.
Read failure:  Queued: Luna. Capacity unavailable. Automatic retry pending.
Replay error:  Queued: Luna. Launch retry pending. [existing error details]
Ambiguous:     Queued launch needs recovery. [existing recovery surface]
Dispatched:    Queue region removed; ordinary Luna Starting/Running status.
Dropped:       Queue region removed; existing disposition/error remains visible.
No deferral:   No queue region. Ordinary task behavior.
```

UI-01 through UI-03 cover requirement 003. Counts are timestamped observations,
not an ordinal queue position. Pending questions and real failures remain visible.
Desktop and phone share the view model, while phone uses stacked text. Status
changes use accessible text; frequent population updates are not live announcements.

## Tests

The following are planned tests, not tests run during package creation. AC suffixes
refer to `AC-TASKS-QUEUED-SESSION-OWNERSHIP-`.

| Work order | Test file and planned cases | Criteria |
| --- | --- | --- |
| 01 | `orchestrator/queued_session_inspection_test.go`: `TestQueuedSessionInspection` (parked/queued, free/full, explicit, unknown); `TestWorkflowParking` (no runtime, consumed tombstone, newer stamp, re-entry) | 001.1-6 |
| 01 | `task/repository/sqlite/workflow_parking_test.go`: `TestWorkflowParking` (conditional writes and restart); backend service/turn and frontend recovery tests | 001.1, 001.4, 001.6; 003.7 |
| 02 | `orchestrator/queued_session_ownership_test.go`: `TestQueuedSessionOwnership` (sibling boot/idle/failure, explicit sibling run); `TestQueuedEntryReplay` (exact-once, restart, deleted/terminal/moved, newer-record CAS races) | 002.1-6 |
| 03 | `task/statussummary/projector_launch_queue_test.go`, `rebuild_launch_queue_test.go`; task service/DTO tests; `task-status-summary` handler and queue view-model tests | 003.1-5 |
| 03 | Desktop/mobile rendered specs below | 001.1-3; 002.1-4; 003.1-7 |

Each work order must record behavioral RED before correction and all exact GREEN
commands. Existing test scaffolds are not evidence of this defect until exercised.

## E2E tests

Create `apps/web/e2e/tests/workflow/queued-session-ownership.spec.ts` (`chromium`)
and `mobile-queued-session-ownership.spec.ts` (`mobile-chrome`) with shared
`queued-session-ownership-helpers.ts`. Extend the desktop file from Task 01
through Task 03 rather than duplicating the same lifecycle fixture.

Use the isolated worker backend's `useEnv` for a ceiling of 1, with restoration
in `finally`. Start and settle a source session, occupy capacity using a separate
controllable mock task, then move through the topbar into a different profile's
auto-start step. Assert Luna exists and is queued before inspection. Capture
transport before opening the task, selecting Astra, reloading, and reconnecting.
Prove no Astra boot/prompt/queued-message additions and unchanged primary ownership.
Do not use helpers that click recovery or reload as part of an idle wait.

Release only the fixture's capacity holder. Observe Luna's one prompt delivery,
queue removal, and untouched Astra. Add explicit Astra follow-up as a separate
case; it must leave Luna's queue intact. Use real service/repository tests for
deterministic restart and simultaneous CAS races. E2E restart supplements them
by checking visible restoration without manually reconstructing queue state.

Phone tests enter through the task drawer, choose each session, inspect queue
details, and observe automatic dispatch. Assert no document horizontal overflow,
visible queue text at 393 px, long-name wrapping, and existing picker target
geometry. A genuine empty prompt retains its warning in unit tests; queue waiting
and passive navigation produce none in the rendered scenario.

## Work orders

- [in_progress] [Task 01: Preserve parked sessions during inspection](task-01-preserve-parked-sessions.md)
- [in_progress] [Task 02: Preserve queued entry lifecycle](task-02-preserve-queued-entry.md)
- [in_progress] [Task 03: Expose queue status across task surfaces](task-03-expose-queue-status.md)

Implementation is complete for all three sequential slices. The work orders
remain in progress because the required PostgreSQL checks need
`KANDEV_TEST_POSTGRES_DSN`, which is not configured in this workspace. No
subagents are authorized by this plan.

## Related delivery records

The [replay and cancellation deadlock repair](../ceiling-replay-cancellation-deadlock/plan.md)
owns the concurrency regression observed on 2026-09-17. It narrows Task 02's
lock scope while preserving its entry and state contracts. Its regression
matrix is complete. Historical results and outstanding PostgreSQL checks in
this package remain unchanged.

[Workflow lifecycle](../workflow-profile-session-reuse/plan.md),
[explicit targeting](../workflow-session-targeting/plan.md),
[same-profile fresh sessions](../workflow-same-profile-new-session/plan.md), and
[session ceiling](../session-concurrency-ceiling/plan.md) are compatibility inputs.
Their results are historical and do not prove this intersection. This package
owns the new cases; it does not reopen completed work or change lifecycle defaults.

## Documentation impact

During Task 03, update `docs/public/tasks-and-workflows.md` (explanation) and
`docs/public/agents-and-profiles.md` only where they describe inspection/recovery.
Explain the distinction between WIP waiting and session-capacity waiting.
No public page is changed during this design turn. Preserve existing overview
and configuration defaults. Check root README/screenshot references for affected wording.

## Verification results

Design-package validation on 2026-09-16:

- `python3 scripts/list-docs.py validate`: passed (282 decisions, 971 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- Catalog queries for `queued-session-ownership` and `passive-session-inspection`:
  discovered the new requirement, design, and decision.
- Work-order reference check: all three files reference existing requirement/AC
  IDs and system-design paths, with pending status and verification sections.
- `git diff --check`: passed.
- `git status --short -- docs/plans/queued-session-ownership`: confirmed the new
  untracked package; files are ready for review and have not been committed.

Implementation and available rendered verification on 2026-09-17:

- Task 01 inspection and recovery coverage passed with
  `go test -race ./internal/orchestrator -run
  'TestQueuedSessionInspection|TestWorkflowParking|Test.*Resume|Test.*ProfileSwitch'
  -count=1`, the repository parking test, the focused frontend recovery tests,
  typecheck, the backend build, and the desktop queued-session scenario.
- Task 02 queue lifecycle coverage passed with the race-focused orchestrator
  suite, SQLite deferred-entry and service suites, the created-session Send Now
  test, the desktop queued-session and workflow-targeting scenarios, and the
  stale-successor compare-and-set test.
- Task 03 projection and UI coverage passed with statussummary/DTO/service
  tests, the race-focused launch-queue suite, 93 focused frontend tests,
  frontend typecheck and lint, the desktop and mobile scenarios, and the
  localized queue view-model/component tests.
- `make -C apps/backend build` passed. `pnpm run i18n:check`,
  `pnpm run i18n:ratchet`, public-doc validation, specification validation,
  and `git diff --check` passed.
- `cd apps/web && pnpm test` passed (2,140 files, 18,465 tests, 4 skipped).
- `make -C apps/backend test` exercised all backend packages and passed the
  task-related packages, but the repository-wide command exits nonzero on
  unrelated environment-sensitive existing tests in process probing, home
  configuration discovery, launcher configuration/restart, and the Office
  priority migration fixture.
- The scoped E2E sleep lint preview and sleep ratchet passed. The full
  `lint:e2e-sleeps` audit still reports the checkout's unrelated baseline
  violations, so it is not used as a clean repository gate.
- The required PostgreSQL checks were not run because
  `KANDEV_TEST_POSTGRES_DSN` is unset. The PostgreSQL parking contract test is
  present and skips without that environment value. Until an isolated DSN is
  supplied, Tasks 01 through 03 remain in progress.

## Risks

- Callback tombstones outlive actual parking; using them alone blocks valid reuse.
- A stale REVIEW writer or replay clear can overwrite a newer accepted entry.
- Passive-source omission in a fallback can still give opening manual privileges.
- Projection refreshes can accidentally count as user activity and reorder tasks.
- Legacy ambiguous ownership must surface recovery without dispatch or data loss.
- The unexplained warning must not cause blanket suppression of genuine empty turns.
