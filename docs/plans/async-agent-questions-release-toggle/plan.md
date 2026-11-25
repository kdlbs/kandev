---
created: 2026-11-21
status: done
requirements:
  - REQ-TASKS-ASYNC-QUESTIONS-011
system_design:
  - ../../specs/tasks/system-design/async-agent-questions.md
legacy_specs: []
---

# Implementation plan: Async agent questions release toggle

## Overview

Ship [PR #4980](https://github.com/kdlbs/kandev/pull/4980) with async agent
questions disabled in every shipped profile. Add a dedicated installation
toggle that takes effect after restart. The Needs-you Inbox keeps its own
toggle. Tasks owns availability because it owns the question record, wait,
and answer delivery. Platform supplies the shared flag infrastructure.

The design was checked against PR head `5a3d4028eb037218604ad085db4c39b50b37db53`.
The [original package](../async-agent-questions/plan.md) records the implemented
enabled behavior. Reconcile the implementation with current `main` and the
live PR head before changing source.

Deliver this package in order: backend and agent gates, rollback for retained
state, then operator surfaces and end-to-end proof. Land the complete change
on PR #4980 before it ships.

## Scope

### In scope

- `features.asyncQuestions` and `KANDEV_FEATURES_ASYNC_QUESTIONS`. Use the existing
  administrator override, startup precedence, feature payload, and restart flow.
- Backend enforcement at agent, HTTP, WebSocket, worker, queue, and startup
  boundaries. Preserve existing blocking questions in both flag states.
- Disabled restart with retained requests, parked waits, queued prompts, or a
  delivery with an unknown outcome. Preserve canonical records and existing
  recovery rules.
- Desktop and phone chat, transcript, and Inbox visibility. Verify both
  settings independently and document the operator workflow.

### Out of scope

- Default-on promotion, flag retirement, live reload, per-workspace rollout,
  and any change to the Inbox default.
- A new question state, new queue model, or changes to blocking clarification,
  question permissions, and already delivered conversation history.
- Refreshing a provider's cached tool list inside a retained agent process.
  The running backend enforces availability. Relaunching the session refreshes
  its advertised tools.

## Technical approach

### 1. Resolve and enforce availability

Add `AsyncQuestions` to `FeaturesConfig`, root `profiles.yaml`, and the existing
typed registration in `internal/runtimeflags/registry.go`. The profile values
and frontend default each start false. The computed value feeds both
`GET /api/v1/features` and the Go boot payload. Use the existing experimental
feature card and restart metadata in Settings.

Wire the startup value before task profiles or prompts are prepared.
Have `resolveTaskSessionMCPProfile` and the prepared profile path use that value.
Remove async capability from `profile.Legacy`. Check guidance passed through
manual and workflow task start, relaunch, and initial-prompt or context-ceiling
recovery. A stored capability or guidance option cannot enable an off boot.

At backend composition, leave the async question service, its worker, and
async wait and notification hooks absent while off. This keeps the dedicated
HTTP group unmounted and removes the async reader from the shared Inbox and boot
summary. Preserve the guarded MCP dispatcher. Both ask handlers reject
`async: true` before starting any legacy waiter or dispatch. An unavailable
companion action fails at the backend even if an old agent advertises it.
`message_task_kandev` accepts a valid blocking correlation through
`validateParentQuestionReply`. An unmatched async ID fails there before dispatch.

| Entry | Off | On |
| --- | --- | --- |
| Normal task over typed MCP | Blocking operator question schema and behavior | Add async mode and the allowed Core companion tools |
| Autopilot child over typed MCP | Blocking direct-parent ask and reply | Add async mode, with the same parent-only ask authority |
| Autopilot root | Existing peer messaging | Add eligible direct-child reads. Ask and forwarding authority stay restricted. |
| Legacy, ephemeral, Office, configuration, passthrough, or external profile | Existing permitted tools | Existing permitted tools. No new async grant. |
| Stale agent or direct relay call | Backend rejects async calls without performing the action | Existing identity, ownership, and scope checks apply |
| Operator HTTP and shared Inbox | Async routes absent. Inbox contains only its existing population. | Async routes available under existing authorization. Inbox adds async questions only when the Inbox is enabled. |

### 2. Settle retained state before recovery

An off boot after use is different from a fresh off install. A persisted wait
can still reach the pending-action SQL, and the general queue can dispatch a
retained prompt even without the async worker. Run narrow, repeatable cleanup
before runtime recovery or queue admission. Cancel active async waits without
synthesizing a wake or completion, rebuild the affected pending-action and task
summaries, and retire undelivered async prompts by their server admission
identity. A session parked only by an async wait can accept an ordinary prompt
subject to existing session controls.

Keep canonical requests, forwarding links, outcomes, transcript data, and
delivery receipts. On re-enable, normal authority checks apply to retained
requests and transcript repair can finish. The plan retires old unsent
automatic prompts so a rollback cannot later wake a session with obsolete
work. Submitted or uncertain work uses the queue's existing recovery rules.
Task and workspace deletion continues to clean up async data when the feature
service is absent.

### 3. Gate presentation and document opt-in

Use the hydrated feature flag at the chat strip, transcript row, and async Inbox
branches. Put the condition before mounting any read or event hook. Keep the
Inbox route, count, hidden panel, and history usable for blocking questions.
Reuse the existing settings card, native phone answer surface, and desktop
inline answer row. Apply test-scoped opt-in to the existing async browser suites.

Update [Agent communication](../../public/agent-communication.md) with enable,
restart, and disable behavior. Qualify the async examples and API availability
in [Automation and MCP](../../public/automation-and-mcp.md), the parent flow in
[Coordination](../../public/coordination.md), and the event in the
[WebSocket API](../../public/websocket-api.md).

## ASCII UI preview

### UI-01: Feature Toggles, administrator view

Same System settings route and scroll owner on both viewports. The card adds
one registry-driven switch. Desktop uses the existing side-by-side header.
Phone uses the existing stacked card and accessible settings save action.

```text
Desktop                              Phone
Feature Toggles                      Feature Toggles
+---------------------------------+  +-----------------------------+
| Async agent questions     [Off] |  | Async agent questions       |
| Experimental                     |  | Experimental                |
| [Use default]                    |  | [Off]                       |
+---------------------------------+  | [Use default]               |
                                     +-----------------------------+
After edit: [Save changes]           After edit: [Save changes]
After saving: Restart required       After saving: Restart required
             [Restart]                            [Restart]
```

The shared restart alert offers the action on supported installations and
shows manual guidance elsewhere. With an environment value set, the switch
and reset are disabled and the existing environment-controlled notice appears.
The grouping is required. Spacing and example labels are illustrative.
Coverage: AC-TASKS-ASYNC-QUESTIONS-011.1.

### UI-02: Task chat and Inbox at each feature state

```text
Async off, desktop and phone
Task chat: [Transcript and blocking questions] -> [Composer]
Inbox (if enabled): [Existing rows and History]

Async on, desktop                     Async on, phone
[Transcript with request cards]       [Transcript with request cards]
[Questions (1) v]                     [Questions (1) >]
  [Open question] [Submit]            [Composer]
[Composer]                           Tap question -> [Back   Question]
                                                    [Question body]
                                                    [Reject] [Submit]
```

No async loading row or empty gap appears while off. With async on, the
desktop strip stays above the composer. Phone keeps the existing single
full-height answer surface with a scrolling body, safe-area actions, and
touch-accessible controls. Both share the same question state. When both
features are on, the Inbox uses its existing grouped list and phone answer
surface. The existing blocking Inbox remains usable when only it is on.
Coverage: AC-TASKS-ASYNC-QUESTIONS-011.6.

## Tests

Test names below identify planned regressions unless already present. Run the
work orders' exact commands and record results against the implementation head.

| Acceptance criterion | Planned unit or integration evidence |
| --- | --- |
| AC-TASKS-ASYNC-QUESTIONS-011.1 | `profiles/async_questions_test.go` checks shipped defaults with `TestAsyncQuestionsOffInEveryShippedProfile`. `runtimeflags/config_test.go` checks precedence and restart with `TestAsyncQuestionsFlagPrecedenceAndRestart`. `features-contract.test.ts` checks key parity and false fallback. |
| AC-TASKS-ASYNC-QUESTIONS-011.2 | `executor/executor_mcp_mode_test.go` covers fresh and stale prepared profiles. `mcp/profile/profile_test.go` and `mcp/server/async_questions_test.go` cover the legacy profile and the two catalog shapes. `TestAsyncQuestionsDisabledDirectAskRejectsBeforeWrites` and `TestHandleMessageTask_AsyncQuestionIDCannotBypassBlockingReplyValidation` cover the relay. `TestAsyncQuestionGuidanceOffDuringPromptReplay` covers replay. The existing restart catalog and prompt tests remain controls. |
| AC-TASKS-ASYNC-QUESTIONS-011.3 | `TestAsyncQuestionsDisabledCompositionLeavesNoServiceOrRoutes`, `TestAsyncQuestionNotificationSubscriptionsFollowFlag`, and `TestAsyncQuestionsDisabledInboxSidecarReturnsNotFoundBeforeWrites` cover the server boundaries. The restart browser flow covers retained rows, Inbox projection, and blocking controls. |
| AC-TASKS-ASYNC-QUESTIONS-011.4 | `orchestrator/async_question_disabled_boot_test.go` covers projection, unsent queue retirement, and recovery. `messagequeue/async_question_retirement_test.go` checks reserved rows and ordinary FIFO. Task 02 records the SQLite and PostgreSQL rollback cases. |
| AC-TASKS-ASYNC-QUESTIONS-011.5 | `orchestrator/async_question_disabled_boot_test.go` checks retained records and re-enable authority. `task/repository/sqlite/async_questions_test.go` checks task and workspace deletion. `backendapp/async_questions_composition_test.go` checks worker startup after successful preparation. |
| AC-TASKS-ASYNC-QUESTIONS-011.6 | Chat strip, native message list, and transcript adapter tests check missing and false flags and retained messages. Inbox page, hidden panel, and refresh controller tests check retained rows, count decisions, and async-only subscriptions. The managed browser specs verify HTTP behavior and the enabled answer flows. |

## E2E tests

| Flow | Existing or planned spec and project | Acceptance criteria |
| --- | --- | --- |
| Fresh default-off boot. Blocking answer succeeds. Features page saves override and restart applies it. | New `tests/chat/async-agent-questions-toggle.spec.ts`, Chromium | 011.1, 011.2, 011.3, 011.6 |
| Same data across on, off, and on boots. Off removes async wait attention and frontend reads. History and valid open requests return on enable. | Same new Chromium spec. Task 02 verifies that an old prompt stays retired through SQLite and PostgreSQL recovery tests. | 011.3, 011.4, 011.5, 011.6 |
| Phone boot with async off. Inbox with only blocking content. Existing phone task answer reachable after opting in with Inbox off. | New `tests/chat/mobile-async-agent-questions-toggle.spec.ts`, mobile Chrome | 011.3, 011.6 |
| Preserve enabled operation, including chat without Inbox, forwarded requests, answer/rejection, wait, and withdrawal. | `tests/chat/async-agent-questions.spec.ts` and `tests/chat/mobile-async-agent-questions.spec.ts`, Chromium and mobile Chrome | 011.2, 011.3, 011.6 and existing flow criteria |

## Work orders

- [x] [Task 01: Gate async question entry points](task-01-gate-async-entry-points.md)
- [x] [Task 02: Settle retained state on a disabled boot](task-02-settle-disabled-state.md)
- [x] [Task 03: Gate operator surfaces and document rollout](task-03-gate-operator-surfaces.md)

## Verification results

Work order 01 passed its backend, frontend contract, and scoped lint checks
on the merged base. The [work order](task-01-gate-async-entry-points.md#results)
records the results. [Work order 02](task-02-settle-disabled-state.md#results)
records the rollback checks on SQLite and PostgreSQL 16.16, race suites,
SQL guard, store conformance, and scoped Go lint.
[Work order 03](task-03-gate-operator-surfaces.md#results) records 255 Vitest
passes, 10 managed Chromium passes, three managed phone passes, and the
public documentation checks. Delivery evidence is linked from the work order.

Implementation checks ran on the base merged at `dac7649546d`. The follow-up
merge of `4ebc1e2fdf4` updated the PR monitoring scripts. Both
`bash scripts/pr-await.test.sh` and `bash scripts/pr-state.test.sh` passed.

Design review checks passed on 2026-11-21. Catalog validation covered 459
decisions and 2,072 specifications. All 36 specification linter tests and the
full specification lint passed. The PR documentation preflight covered the new
three orders and all 15 orders when combined with the original package. All six
new acceptance criteria trace to work orders and planned tests.

The work orders record focused Go and frontend tests, named PostgreSQL cases,
managed desktop and mobile runs, and docs coverage. The
[PR checks](https://github.com/kdlbs/kandev/pull/4980/checks) track CI for the
delivered head.
Run the Go lint command for tasks 01 and 02 from the repo root using the
authoritative base and the actual changed files. Save the pre-work commit in
`work_order_base` at the start of each work order:

```bash
backend_go_files=':(glob)apps/backend/**/*.go'
{
  git diff --name-only -z --diff-filter=ACMRT "$work_order_base" -- "$backend_go_files"
  git ls-files --others --exclude-standard -z -- "$backend_go_files"
} | sort -zu | xargs -0 -r bash scripts/lint-go-changed
```

## Risks

- A stale tool catalog or prepared prompt may still mention async mode after
  an off boot. The backend must reject its use, and a fresh session catalog
  must reflect the current flag.
- Removing the worker alone cannot clear old wait indicators or queued work.
  The disabled restart tests must check the first projected state as well as
  subsequent recovery. Pre-crash uncertain delivery still needs its existing
  operator recovery boundary.
- Both browser projects need targeted opt-in that unwinds after the test.
  Enabling it globally would hide the default-off regression.
