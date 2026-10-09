---
created: 2026-09-21
status: complete
requirements:
  - REQ-TASKS-SESSION-STALL-VISIBILITY-001
  - REQ-TASKS-RESTART-ORPHAN-SESSIONS-002
  - REQ-TASKS-INTERRUPTED-TASK-INDICATOR-001
system_design:
  - ../../specs/tasks/system-design/session-stall-visibility.md
  - ../../specs/tasks/system-design/restart-orphaned-session-terminalization.md
  - ../../specs/tasks/system-design/interrupted-task-indicator.md
legacy_specs: []
---

# Implementation plan: Orphaned session recovery on task open

## Overview

Preserve idle and interrupted running conversations during reconciliation.
Both sweeps must leave interrupted sessions recoverable, rather than cancelled.
Focusing a task restores its selected agent. A new user message continues the
conversation. Opening Kandev must not eagerly resume every interrupted agent.
Implement recoverable settlement, then focus recovery, then the persistent warning indicator.

The task system owns this change because it owns session cancellation and
recovery eligibility. The UI uses the existing recovery flow.

## Confirmed evidence

Read-only diagnostics for task `52f801c5-b0ff-4b4f-8f00-0d37ba804d03`
identify session `a136900f-3546-4843-8a99-37194378222e`.
Times below include the Lisbon UTC offset.

- September 20, 11:37:44 +01:00: idle reclaim released the provider runtime.
  The log recorded a preserved resume token and worktree.
- September 21, 09:44:24 +01:00: startup reconciled `WAITING_FOR_INPUT`
  for lazy recovery, again with a token and worktree.
- September 21, 09:45:25 +01:00: `active-session sweep: healing orphaned sessions`
  named this session. The state event recorded `WAITING_FOR_INPUT -> CANCELLED`.
- The session-list MCP response independently recorded the same cancellation time.
- The diagnostic manifest identified deployed commit `1614720ceb`.
  The bundle was partial because its oldest logs exceeded the archive limit.
  The relevant September 21 cancellation records were present.

PR [#3832](https://github.com/kdlbs/kandev/pull/3832), commit `00c7469d81`,
introduced the active-session sweep. It classifies every execution-less active
session as orphaned, including idle `WAITING_FOR_INPUT` sessions.
PR [#3833](https://github.com/kdlbs/kandev/pull/3833), commit `1614720ceb`,
added a separate restart pass limited to `STARTING` and `RUNNING`.
The logs identify #3832's pass as the cancellation source in this incident.
The merged-PR banner for #3819 is not evidence that its merge cancelled the session.

`GetTaskSessionStatus` automatically recovers archive cancellations, but other
`CANCELLED` sessions fall through to workspace restoration. The orphan reason
therefore requires a manual Resume despite preserved recovery data.

## Scope

### In scope

- Exclude waiting sessions without an unfinished turn from stall detection and healing.
- Replace execution-loss cancellation in both sweeps with recoverable waiting state.
- Recover existing exact orphan cancellations through normal task opening.
- Continue interrupted work only when a new message arrives, including during startup.
- Preserve same-session history, workspace identity, user preferences, and launch ownership.
- Test desktop and phone opening, failure fallback, and explicit-stop exclusions.
- Keep interruption warnings until successful resume. Replace the shared red circle
  with a warning-colored triangle in sidebar rows and task cards.

### Out of scope

- Prompt replay, automatic continuation of completed work, or workflow advancement.
- Removing reconciliation, changing thresholds, or adding a runtime flag.
- Live-instance repair, new sessions, database migrations, or provider redesign.

## Technical approach

Task 01 changes both reconciliation passes and their conditional repository
transitions. Reuse startup recovery semantics in `reconcileActiveSessionOnStartup`:
settle the abandoned turn without a success event, preserve recovery identity,
and expose `WAITING_FOR_INPUT`. Include missing-runtime-row sessions. Retain
live-execution, launch-grace, archive/stop, and newer-turn guards. Idle sessions
need no transition or stall event.

Task 02 changes exact cancellation eligibility in `task_operations.go` and
corrects the orphan-reason comment in `models/resume_safety.go`. Reuse existing
resume-token and same-session initialization paths. Keep unsupported recovery
and missing-profile errors actionable. Use the existing frontend recovery hooks
and automatic admission contract. Change frontend logic only if regression
coverage exposes a state-handling gap.

Task 03 updates the shared interruption renderer and moves marker clearing
from STARTING entry to confirmed recovery success. The existing durable marker
survives unsuccessful attempts. Follow its paired design for stale callbacks,
partial updates, warning styling, and desktop/phone coverage.

No separate ADR is needed. The existing
[session-open decision](../../decisions/2026-09-18-session-open-resumes-conversation.md)
already owns prompt-free provider recovery. This package also replaces automatic
orphan cancellation with recoverable interruption settlement, as the user requested.

## ASCII UI preview

UI-01: Focus an interrupted or legacy orphan-cancelled conversation. Labels are illustrative existing
localized states, not proposed new copy.

```text
Current desktop
[orphaned session] [Resume] [Start fresh session]
[existing transcript                            ]

Proposed desktop
[normal agent startup progress                  ]
[existing transcript                            ]
                  |
                  v
[existing transcript                            ]
[message composer                         Send  ]

Proposed phone: task route or task picker
[Task header / session picker]
[normal startup progress     ]
[existing transcript         ]  <- existing chat scroll
[composer              Send  ]
[existing bottom navigation  ]
```

After success, the warning disappears. A new user message continues the work. On actual recovery failure, retain the
existing error, Resume, and Start fresh session controls. With auto-start
prevention enabled, retain manual recovery. Opening never sends a message.
No new overlay, scroll owner, or touch control is added. Keep phone safe areas
and existing touch targets. Structural requirement: recovery occurs in the
selected conversation. Spacing is illustrative.

UI-02: Interruption status in the existing sidebar/card status slot.

```text
Before:          (red !) Task title
Desktop after:   /!\     Task title  [warning color]
Phone after:     /!\     Task title  [drawer row / board card]
Startup:         [normal progress; interruption marker retained]
Resume succeeds: [normal task status] Task title
Resume fails:    [marker retained; existing recovery error available]
```

Keep the localized accessible label "Interrupted by restart". The shared
triangle and warning color are required. Existing layouts and scroll owners
remain. No new hover-only information or touch control is introduced.

## Tests

- Task 01 covers stall AC .1-.7 and restart AC 002.1, .4-.7:
  idle exemption, resumable interrupted work in both sweeps, missing executor rows,
  active-turn read errors, mixed siblings, and completion races.
- Task 02 covers AC .8-.10 with `task_operations_resume_test.go` and frontend
  session-resumption tests: exact reason, supported/missing token, profile
  validation, duplicate opening, blocked recovery, and failure fallback.
- Task 03 covers interruption AC .2-.7 with marker lifecycle tests, shared icon
  tests, and desktop/phone sidebar and board assertions. Verify retained warnings
  across failed recovery, reload, and repeated restart, followed by live clearing.
- Retain restart-pass, archive-recovery, explicit-stop, deferred-owner, and
  session-ceiling regressions.

## E2E tests

The existing `session/session-resume.spec.ts`,
`session/session-recovery.spec.ts`, and their mobile counterparts already cover
backend interruption, lazy focus recovery, queued messages during startup,
same-session follow-up, auto-start prevention, and failed recovery retry using
the real status-to-launch path. The new backend tests establish the exact
legacy orphan eligibility and reconciliation classification without adding a
production endpoint. The task indicator checks below provide the rendered
desktop and phone assertions for the retained warning state.

Add `task/mobile-task-interrupted-icon.spec.ts` and extend
`task/task-interrupted-icon.spec.ts` for UI-02. Use the shared state-icon unit
tests for every consumer's shape/color contract and the recovery suites for
successful/failed resume behavior. Browser screenshots prove the rendered states.

## Work orders

- [x] [Task 01: Preserve resumable conversations](task-01-preserve-idle-conversations.md)
- [x] [Task 02: Recover orphan conversations on open](task-02-recover-orphan-conversations.md)
- [x] [Task 03: Keep interruption warnings](task-03-interruption-warning.md)

## Related delivery records

The implemented [stall package](../session-stall-visibility/plan.md) and
[restart package](../restart-orphaned-session-terminalization/plan.md) remain
historical records. Their tests remain regression inputs. This package supersedes their execution-loss cancellation policy and retains
their liveness, grace, and concurrency protections. Their historical results do
not prove the revised behavior.

## Verification results

- `python3 scripts/list-docs.py validate`: passed, catalog valid after the warning-indicator revision.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- `go test ./internal/orchestrator ./internal/task/service ./internal/task/repository/sqlite`: passed.
- Full affected Go packages pass after the review remediation, including
  missing-row focus recovery, successor barriers, fresh-context settlement,
  marker CAS, and committed-generation retry regressions.
- Targeted backend recovery, reconciliation, metadata-CAS, and marker-lifecycle tests: passed.
- Targeted frontend Vitest, typecheck, and ESLint checks: passed.
- Review-fixup regressions for fail-closed executor snapshots, CREATED-session
  recovery tokens, abandoned-turn tool-call retry, fresh delayed-commit effects
  context, and false-to-true-to-false marker generations: passed.
- Chromium and mobile-chrome interrupted-indicator E2E tests: passed (one test each).
- `node --test scripts/validate-public-docs.test.mjs` and
  `node scripts/validate-public-docs.mjs`: passed.

The implementation preserves idle conversations, settles interrupted sessions
to recoverable waiting state with race-safe repository predicates, and lazily
recovers the selected conversation without replaying its old prompt. Exact
legacy orphan cancellations remain compatible with the same recovery path while
archive, explicit-stop, ownership, capacity, and live-execution guards retain
their authority. The shared interruption marker now uses a warning triangle,
survives failed or stale recovery attempts, and clears only after confirmed
provider recovery.

## Integrated review remediation

The three work orders are complete together with the follow-up review fixes:

- Recovered CREATED, STARTING, and RUNNING sessions persist an immutable settlement token
  and executor snapshot. A missing `executors_running` row is still resumable,
  and task-open status plus focus recovery use the same session and profile.
- Active and restart reconciliation use a bounded post-commit context. Turn,
  tool, executor, marker, and event effects remain retryable until the durable
  settlement is cleared. Executor repair, session settlement, and marker writes
  compare the captured executor and committed session generations, so a
  successor cannot be stopped, settled, or marked by a delayed sweep.
- Recovery attempts retain the interruption-marker snapshot through active and
  tombstoned attempts. Boot callbacks without a valid generation fail closed,
  and a committed marker publishes `task.updated` with the interruption state
  so connected desktop and phone clients update without a reload. Live task
  merges carry a per-task marker generation, so an in-flight snapshot cannot
  erase a newer interruption episode when its final boolean matches the
  fetch-start value.

The integrated implementation and regressions are validated by the backend
package tests, the targeted marker and UI checks, the workspace build and lint,
and the specification and public-document validation commands listed above.

## Risks

- A waiting session can still hold an unfinished turn. State alone is insufficient.
- Re-reading a completed turn as absent can defeat the original cancellation snapshot.
- Broadening all cancellations would restart intentionally stopped work.
- A tokenless provider cannot promise native conversation context restoration.
- Partial recovery must not expire questions, duplicate prompts, or advance the workflow.
- Partial diagnostic retention limits reconstruction of earlier runtime history.
