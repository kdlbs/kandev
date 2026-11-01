---
created: 2026-10-30
status: in_progress
requirements:
  - REQ-TASKS-ASYNC-QUESTIONS-001
  - REQ-TASKS-ASYNC-QUESTIONS-002
  - REQ-TASKS-ASYNC-QUESTIONS-003
  - REQ-TASKS-ASYNC-QUESTIONS-004
  - REQ-TASKS-ASYNC-QUESTIONS-005
  - REQ-TASKS-ASYNC-QUESTIONS-006
  - REQ-TASKS-ASYNC-QUESTIONS-007
  - REQ-TASKS-ASYNC-QUESTIONS-008
  - REQ-TASKS-ASYNC-QUESTIONS-009
  - REQ-TASKS-ASYNC-QUESTIONS-010
system_design:
  - ../../specs/tasks/system-design/async-agent-questions.md
legacy_specs: []
---

# Implementation plan: Async agent questions

## Overview

Deliver [issue #4836](https://github.com/kdlbs/kandev/issues/4836) as an opt-in question lifecycle.
An agent asks early, continues independent work, and reads answers.
When no independent work remains, it explicitly waits.
This is a feature package created through the issue-fix workflow, not a repair of an active acceptance-criterion violation.

The implementation order is durable state, operator flow, parent flow, explicit waits, then Inbox integration.
Each stage preserves the blocking path and has a focused verification boundary.
Work orders 01 to 05 are implemented on PR #4980.

### Remediation after review of `226eca1`

Review found that the shipped design leaves operators without a surface by default, because the Inbox flag is off in production.
Work orders 06 to 12 make the task chat the primary surface and keep the Inbox as an optional view.
They also fix reproduced backend failures, add guidance for choosing a mode, and prove that answers reach agents.
Backend recovery comes first, because the chat surface and final E2E flows depend on correct terminal behavior.
The end-to-end flows run last, so they exercise the finished surface.

## Investigation and confidence

Investigated source: `838043071d99ae3e9df22fd6ef8efa161ea01c73`, dated 2026-10-30.
The issue had no comments or image attachments. No PR mentioning #4836 appeared in the repository search.
The issue is assigned to the authenticated user, `carlosflorencio`.

The cause is an existing blocking assumption across four boundaries:

1. `registerInteractionTools` has no async input. `askUserQuestionHandler` relays no mode and extracts a completed answer.
2. `handleAskUserQuestion` registers a waiter. `waitForAskUserQuestion` sets waiting state before calling that waiter.
3. `handleAskParentQuestion` persists a child message and calls the pause path before notifying its parent.
4. `clarificationBundleTableExpr` restricts active discovery to current-turn blocking messages and excludes parent questions.

A read-only trace through these functions reproduces the reported behavior.
The existing parent-pause tests passed:

```bash
(cd apps/backend && go test -trimpath ./internal/mcp/handlers \
  -run 'TestHandleAskParentQuestion_(PersistsRoutesAndPauses|HoldsUnrelatedChildQueue)$' -count=1)
```

Result: package passed in 0.398 seconds. This proves the current pause controls, not the proposed feature.
No new feature test or browser flow ran in the design turn.

The fix path is sufficiently grounded to plan and assign.
It uses existing authorization, question validation, queue admission receipts, and Inbox refresh infrastructure.
Runtime correctness still requires the race, crash, and browser evidence named below.

## Scope

### In scope

- The [task-owned requirements](../../specs/tasks/requirements/async-agent-questions.md), `REQ-TASKS-ASYNC-QUESTIONS-001` through `010`.
- A task chat question surface that works with the Inbox on or off, plus transcript history and operator notifications.
- Recovery from wait races, reparenting, replacement parent sessions, and withdrawn parent prompts.
- Agent guidance for choosing async or blocking mode.
- Operator and direct-parent async creation, answer reads, withdrawal, and correlated resolution.
- Normal-parent forwarding to the operator, with one canonical child request.
- Durable outcome/outbox handling and an explicit dependency wait.
- Typed Inbox population, task grouping, desktop/phone answers, and withdrawn history.
- Blocking compatibility, supported task-mode profiles, and public API/workflow guidance at implementation time.

### Out of scope

- Issue #4820, historical blocking-question reopening, and changes to current-turn clarification predicates.
- Multi-hop forwarding, Autopilot-root operator access, Office routing, and external MCP creation.
- New runtime flags, changed flag defaults, or provider-specific mid-turn push channels.
- Generic queue redesign, broad local QA/review, and unrelated Autopilot cleanup.
- Removing the Inbox surface, or changing its production default.
- A task-list indicator for open questions on running agents, and real-agent evaluation of mode choice. Both are follow-up candidates.

## Technical approach

Use the [system design](../../specs/tasks/system-design/async-agent-questions.md) as the technical source.
The [ADR](../../decisions/2026-10-30-async-question-lifecycle.md) records the lifecycle, delivery boundary, and chat-first surface.

- Add neutral task question models and `internal/task/questions/` service ports.
  Task SQL storage owns async requests, forward links, delivery intents, and explicit wait gates.
- Route explicit async mode before the blocking waiter and response extractor.
  Retain strict boolean validation and a union output schema.
- Supply a backendapp queue adapter using `QueueMessageWithMetadataForSessionWithClientQueueID` and durable admission lookup.
  Keep question notifications outside editing, Send now, and auto-merge behavior.
- Extend correlated parent replies by request kind. Preserve existing parent ownership and tool selection.
- Integrate an explicit gate with guarded pause, restart reconciliation, completion, and queue drain.
  Mere request existence does not become a workflow barrier.
- Add a typed async population to the existing Inbox read, count, boot, hiding, history, and refresh paths.
  Share question editor behavior with an explicit async submission adapter.

Remediation boundaries:

- **Gate settlement:** `Service.Wait` and `releaseRacedWait` in `task/questions/wait.go`, and `markAsyncQuestionStaleTx` in `async_question_resolution.go`.
- **Authority sweep:** a bounded query on the existing reconciliation tick in `task/questions/delivery.go`. It replaces a hook on each parent-changing path.
- **Parent routing:** `ResolveParent` and `Forward` in `service.go`, and session selection through `GetActiveTaskSessionByTaskID` in `backendapp/async_questions.go`.
- **Mode guidance:** `KandevContextOptions` and the question sections in `sysprompt.go`, and tool descriptions in `mcp/server/server.go`.
- **Chat surface:** a task-scoped store slice and hook, and a strip in `ComposerFooterAllocation` in `task-chat-panel.tsx`, reusing `AsyncQuestionForm` and the phone surface.
- **Transcript and status:** an `agent_question_request` message written by a `transcript` outbox intent, `pendingActionsBySessionQuery` for parked gates, and `HandleClarificationRequested` for notifications.

The provider/transport/capability compatibility matrix is in the design's
[compatibility section](../../specs/tasks/system-design/async-agent-questions.md#compatibility-and-capability-selection).
This package claims MCP-level support only for the listed persistent task surfaces.

## ASCII UI preview

These views show structural requirements. Labels, spacing, and example task content are illustrative.
All rendered copy must use locale catalogs. A preview is not implementation evidence.

### UI-01: Desktop Inbox, task group with an expanded async question

Entry: existing workspace Inbox, Needs-you tab. Applies to `AC-TASKS-ASYNC-QUESTIONS-007.1` through `.5`.

```text
Inbox
[Needs you (3)] [Failed] [History]

Task: Checkout integration                     Open task
  > Database choice        Agent continues work      ...
  v API response value     Forwarded by Planning     ...
    Child: API batch       Agent continues work
    Which value belongs in the missing API field?
    ( ) Use documented default
    ( ) Keep the field empty
    [Additional answer text                         ]
    [Reject]                                  [Submit]

Task: Copy update                              Open task
  > Heading wording        Waiting for answer        ...
```

Task headings group requests. The count is three requests, not two groups.
Desktop uses inline expansion and existing page scrolling.
Blocking rows retain their current answer path and appear in the same typed population.
The forwarded row identifies both child and parent. Unforwarded parent questions are absent.

### UI-02: Phone Inbox and focused answer surface

Entry: existing phone navigation, Inbox. Applies to the same criteria as UI-01.

```text
Inbox                         Answer surface
+------------------------+    +------------------------+
| Menu   Inbox           |    | Back    API batch      |
| Needs you | Failed |... |    | From Planning          |
| Checkout integration   |    | Agent continues work   |
| Database choice      > |    |------------------------|
| API response value   > | -> | Question and context   |
| Copy update            |    | ( ) Documented default |
| Heading wording      > |    | ( ) Empty field        |
|                        |    | [Additional text     ] |
|       list scrolls     |    |  question body scrolls |
+------------------------+    |------------------------|
                              | Reject        Submit   |
                              |      safe area         |
                              +------------------------+
```

The phone answer is a full-height surface, not a squeezed desktop expansion.
Keep the header and action region fixed. The question body owns its vertical scroll.
Use dynamic viewport height and keyboard/safe-area clearance.
Back returns to the same group and row, retaining an unsent draft.
The primary row tap opens the answer. Secondary actions use a visible menu trigger.
Phone/coarse-pointer controls measure at least 44px. Desktop ordinary controls remain 28px.
The closest shipped precedents are the Inbox page shell and `SessionTaskSwitcherSheet` scroll/focus structure.

### UI-03: Terminal, loading, and retry states

```text
History
  API response value   Withdrawn
  Reason: The API documentation supplied the value.

Initial read:   Loading questions...
Read failure:  Questions could not load.              Retry
Submit failure inside the answer surface:
  Your answer remains here. It could not be submitted. Retry
Concurrent winner:
  The request was withdrawn.       (close actionable row)
Empty current list:
  No questions need your answer.   (retain existing exclusions)
```

The withdrawn outcome is readable and never counts as answered.
Failures preserve drafts and never masquerade as caught-up state.
Async question cards in the author's conversation are non-modal.
They do not cover the composer or claim the running agent is paused.

### UI-04: Desktop task chat strip, collapsed and expanded

Entry: any task chat with open operator requests. Applies to `AC-TASKS-ASYNC-QUESTIONS-009.1` and `.2`.

```text
Collapsed
| ...transcript scrolls...                                 |
|----------------------------------------------------------|
| ? 2 open questions, agent continues work          [Show] |
|----------------------------------------------------------|
| [Composer                                         Send ] |

Expanded
| ...transcript scrolls in the remaining height...         |
|----------------------------------------------------------|
| ? 2 open questions                                [Hide] |
|   v API response value       Agent continues work        |
|     Which value belongs in the missing API field?        |
|     ( ) Use documented default                           |
|     ( ) Keep the field empty                             |
|     [Additional answer text                            ] |
|     [Reject]                                   [Submit]  |
|   > Database choice          Waiting for answer          |
|----------------------------------------------------------|
| [Composer                                         Send ] |
```

The strip sits above the composer, outside the transcript scroller, and is absent with no open requests.
The expanded region scrolls internally and never covers the composer.
While a blocking clarification is active, the strip stays collapsed to its count.

### UI-05: Phone task chat strip and answer surface

Entry: phone task chat. Applies to `AC-TASKS-ASYNC-QUESTIONS-009.2`.

```text
Task chat                     Answer surface
+------------------------+    +------------------------+
| Menu   Checkout   ...  |    | Back    Checkout       |
|  transcript scrolls    |    | Agent continues work   |
|                        |    |------------------------|
|                        |    | Question and context   |
|------------------------|    | ( ) Documented default |
| ? 2 open questions   > | -> | ( ) Empty field        |
|------------------------|    | [Additional text     ] |
| [Composer        Send] |    |  question body scrolls |
+------------------------+    |------------------------|
                              | Reject        Submit   |
                              |      safe area         |
                              +------------------------+
```

With several requests, the strip opens a list of rows first, and a row opens the answer surface.
This reuses the Inbox phone surface. Controls measure at least 44px.

### UI-06: Forwarded request in the parent's chat

Applies to `AC-TASKS-ASYNC-QUESTIONS-009.3`.

```text
| ? 1 forwarded question                            [Hide] |
|   v API response value       From child: API batch       |
|     Which value belongs in the missing API field?        |
|     ( ) Use documented default    ( ) Keep field empty   |
|     [Reject]                                   [Submit]  |
```

### UI-07: Transcript entries and terminal states

Applies to `AC-TASKS-ASYNC-QUESTIONS-009.4` and `007.7`.

```text
? Asked: Which value belongs in the missing API field?  Open
? Asked: Database choice                 Answered: PostgreSQL
? Asked: Heading wording                 Rejected: Out of scope
? Asked: Retry budget                    Withdrawn: Found in the docs
? Asked: Deploy region                   Closed: parent task changed
```

Entries show current state and never open a blocking overlay. An open entry links to the strip.

## Tests

All tests below are planned. Each implementation order starts with a failing behavior assertion.
The first API regression is **new** `TestAsyncQuestionsOperatorAskKeepsTurnRunning` in
`internal/mcp/handlers/async_question_operator_test.go`.
It requests async mode, observes the creation receipt before any answer, and verifies that the original generation remains running.
Before the feature, the request enters the blocking wait. The RED assertion must expose that behavior, not a missing import.

| Acceptance criteria | Planned test file and named evidence |
| --- | --- |
| `001.3`, `002.2`, `002.4` | `internal/task/repository/sqlite/async_questions_test.go`: `TestAsyncQuestionsCreateReplay`, `TestAsyncQuestionsResolutionWinner`. |
| `002.1`, `002.3`, `003.3` | Same repository package: `TestAsyncQuestionsRestart`, `TestAsyncQuestionsLifecycleCleanup`, `TestAsyncQuestionsOutboxCrashWindow`, plus PostgreSQL variants. |
| `001.1`, `001.2`, `003.1` | `internal/mcp/handlers/async_question_operator_test.go`: `TestAsyncQuestionsOperatorAskKeepsTurnRunning`, `TestAsyncQuestionsOperatorCheckpointRead`. |
| `003.2`, `003.3`, `008.2` | `internal/orchestrator/async_question_delivery_test.go`: `TestAsyncQuestionsDeliveryWaitsForBoundary`, `TestAsyncQuestionsDeliveryBehindLiveClarification`. |
| `004.1` through `.3` | `internal/task/questions/withdraw_test.go`: `TestAsyncQuestionsWithdrawWinner`, `TestAsyncQuestionsWithdrawReplay`. |
| `001.2`, `003.4`, `005.1` through `.4` | `internal/mcp/handlers/async_parent_question_test.go`: `TestAsyncQuestionsParentKeepsChildRunning`, `TestAsyncQuestionsParentListScope`, `TestAsyncQuestionsForwardedWinner`, `TestAsyncQuestionsForwardAuthorization`. |
| `006.1` through `.4` | `internal/orchestrator/async_question_wait_test.go`: `TestAsyncQuestionsNoImplicitWait`, `TestAsyncQuestionsWaitAnswerRace`, `TestAsyncQuestionsWaitMixedDependencies`, `TestAsyncQuestionsWaitNewerTurn`, `TestAsyncQuestionsWaitRestart`. |
| `007.1`, `007.5` | `internal/clarification/inbox_async_test.go`: `TestAsyncQuestionsInboxMixedPage`, `TestAsyncQuestionsInboxBootHiddenCount`. New frontend grouping/slice tests. |
| `007.2` through `.4` | `components/needs-you-inbox/async-question-row.test.tsx`, `app/needs-you-inbox/needs-you-inbox-page-client.test.tsx`: running/wait labels, draft retention, winner handling, and responsive view reuse. |
| `008.1`, `008.3` | `internal/mcp/server/async_questions_test.go`: `TestAsyncQuestionsSchemaModes`, `TestAsyncQuestionsProfileMatrix`. Existing ask/parent-pause and prompt tests. |
| `006.5` | `internal/task/questions/wait_settlement_test.go`: `TestAsyncQuestionsWaitReadyLeavesNoGate`, using a store wrapper that resolves between gate creation and re-check. |
| `002.3`, `002.5`, `006.6` | `internal/task/questions/authority_sweep_test.go`: `TestAsyncQuestionsSweepStalesReparentedRequest`, `TestAsyncQuestionsSweepStalesArchivedParent`, `TestAsyncQuestionsStaleDependencyWakesAuthor`. PostgreSQL variant in `async_questions_postgres_test.go`. |
| `004.4` | `internal/task/questions/withdraw_delivery_test.go`: `TestAsyncQuestionsWithdrawAbandonsUnsentParentPrompt`, `TestAsyncQuestionsWithdrawRemovesQueuedParentPrompt`. |
| `005.5` | `internal/task/questions/parent_routing_test.go`: `TestAsyncQuestionsReplacementParentSessionAnswers`. `internal/backendapp/async_questions_test.go`: `TestAsyncQuestionParentDeliveryTargetsActiveSession`, `TestAsyncQuestionParentDeliveryWaitsForSession`. |
| `010.1` through `.3` | `internal/sysprompt/sysprompt_test.go`: `TestAsyncQuestionGuidanceByCapability`. `internal/mcp/server/async_questions_test.go`: `TestAsyncQuestionToolDescriptionsByCapability`. |
| `001.2`, `009.1` through `.3` | `hooks/domains/session/use-task-async-questions.test.tsx`, `components/task/chat/async-question-strip.test.tsx`. `internal/task/questions/http_handlers_test.go`: `TestAsyncQuestionsTaskScopeIncludesForwarded`, `TestAsyncQuestionsTaskScopeHidesUnforwarded`. |
| `009.5` | `internal/backendapp/async_questions_test.go`: `TestAsyncQuestionNotificationOncePerRequest`, `TestAsyncQuestionNotificationSkipsUnforwardedParent`. |
| `009.4`, `004.2` | `internal/task/questions/transcript_delivery_test.go`: `TestAsyncQuestionsTranscriptEntryIsIdempotent`. `components/task/chat/messages/agent-question-message.test.tsx`. |
| `009.6` | `internal/task/repository/sqlite/async_question_pending_action_test.go`: `TestAsyncQuestionsParkedGateProjectsPendingAction`, `TestAsyncQuestionsOpenRequestProjectsNoPendingAction`. |
| `007.6`, `007.7` | `internal/clarification/inbox_async_test.go`: `TestAsyncQuestionsInboxWaitingIsPerDependency`. `components/needs-you-inbox/async-question-row.test.tsx`: label and terminal-state cases. |
| `002.2`, `007.4`, `007.5` | `internal/task/questions/http_handlers_test.go`: `TestAsyncQuestionsRespondRetryKeepsWin`, `TestAsyncQuestionsRespondHidesForeignRecord`. `internal/task/repository/sqlite/async_questions_test.go`: `TestAsyncQuestionsWorkspaceCascadeRemovesRows`. |

Criteria in the table abbreviate `AC-TASKS-ASYNC-QUESTIONS-`.
Use the full identifiers in work orders and test annotations.
Mixed-state fixtures must include a working author, an explicit waiter, a terminal session, and an unforwarded child request.
Do not let an existential live request turn every sibling into a blocked request.

## E2E tests

**New** `apps/web/e2e/tests/chat/async-agent-questions.spec.ts`, project `chromium`:

- `asks early and receives a checkpoint answer while work continues`: `001.1`, `001.2`, `003.1`, `003.2`, `007.2`.
- `forwards two children and resolves one canonical request`: `003.4`, `005.1` through `.3`, `007.1`.
- `withdraws an unanswered request and retains history`: `004.1` through `.3`, `007.4`.
- `waits explicitly and resumes after a racing answer`: `006.1` through `.4`.
- `combines blocking and async rows across reload and workspace changes`: `002.1`, `007.1`, `007.4`, `007.5`, `008.1`.

**New** `apps/web/e2e/tests/chat/mobile-async-agent-questions.spec.ts`, project `mobile-chrome`:

- Answer a forwarded child question through UI-02, return to the task group, and observe canonical closure.
- Preserve an unsent answer through Back and desktop/phone transitions.
- Read withdrawn history, handle submission failure, and retry without duplicate delivery.
- Assert internal scrolling, safe-area containment, zero document horizontal overflow, and measured 44px targets.
- Check the phone composition with both touch and a narrow fine pointer, plus widths around the 768px branch.

The existing mock-agent scenario pattern must drive actual MCP async calls and later work markers.
Seed identities through the API, then assert operator outcomes in the rendered Inbox.
Backend integration tests own precise crash/restart and cancellation races. Browser reload alone does not prove backend restart safety.

The flows above are the original plan. The specs on the PR cover a smaller set, so work order 12 owns the remainder.
Remediation flows live in the same two spec files:

- `answers from chat and the agent uses the answer`: `003.2`, `009.1`, `009.2`. The mock agent must emit a marker containing the answer after it resumes.
- `resumes after an explicit wait is answered from chat`: `006.2`, `006.4`, `009.6`.
- `forwards a child question and answers from the parent chat`: `005.1`, `005.2`, `009.3`.
- `withdraws a question and retains the transcript entry`: `004.1`, `004.2`, `009.4`.
- `answers from chat with the Inbox disabled`: `009.1`.
- `mobile-chrome: answers from the phone chat surface`: `009.2`, with containment and 44px checks.

## Work orders

Execute sequentially. No subagents are authorized.

- [x] [Task 01: Persist async requests and delivery intents](task-01-persist-async-questions.md)
- [x] [Task 02: Deliver operator async questions](task-02-operator-question-flow.md)
- [x] [Task 03: Route and forward async parent questions](task-03-parent-question-flow.md)
- [x] [Task 04: Pause only for explicit question dependencies](task-04-explicit-question-waits.md)
- [x] [Task 05: Integrate desktop and phone Inbox flows](task-05-inbox-and-mobile.md)
- [x] [Task 06: Settle waits and close invalid requests](task-06-wait-and-authority-recovery.md)
- [x] [Task 07: Route parent questions by task](task-07-parent-session-routing.md)
- [x] [Task 08: Guide agents on choosing a mode](task-08-agent-mode-guidance.md)
- [x] [Task 09: Answer async questions from the task chat](task-09-chat-question-surface.md)
- [x] [Task 10: Add transcript history and accurate status](task-10-transcript-and-status.md)
- [x] [Task 11: Close remaining API and cleanup gaps](task-11-api-and-cleanup.md)
- [x] [Task 12: Prove answers reach agents end to end](task-12-end-to-end-delivery.md)

Order: 06, then 07, 08, 09, 10, 11, and 12.
Task 08 is independent and can move earlier. Task 10 needs 09, and Task 12 needs all of them.

## Verification results

Work orders 01 to 12 are complete. Each work order records its detailed scope and verification.

- Investigation baseline: the two existing parent-pause tests passed, as recorded above.
- Feature implementation: complete. Targeted Go, web unit, typecheck, i18n, and lint checks passed.
- Backend verification: async question race and SQLite persistence suites, MCP/server/handler, clarification, sysprompt, backendapp, gateway WebSocket, orchestrator, mock-agent, store-conformance race, and SQL guard passed.
- Frontend verification: 217 focused Vitest tests, typecheck, lint, i18n check, and i18n ratchet passed.
- Browser verification: all 7 Chromium async-question E2E tests and both mobile Chrome tests passed. This includes explicit waits, direct-parent forwarding, withdrawal transcript history, and Inbox-disabled task chat.
- Documentation verification: public documentation validation, catalog validation for 425 decisions and 1871 specifications, 36 specification-linter tests, and full specification lint passed.
- Changed-file Go lint passed with 0 issues; `gofmt` and diff whitespace checks passed.
- PostgreSQL race and pending-action coverage is environment-gated and was skipped because `KANDEV_TEST_POSTGRES_DSN` is not configured.
- PR review fixup: fail-closed HTTP authorization for empty workspaces, full-precision `(created_at, kind, id)` Inbox cursors with correct source boundaries, conditional wait acknowledgement that cannot overwrite a fenced release, stale-pause state writes deferred until the pause settles, capture failures propagated instead of widening the pause, current-parent child-list reads, PostgreSQL advisory locks for create/wait boundaries, seeded PostgreSQL tests, hidden full counts, boot combined hasMore/expiry, root child-question reads, readable labels in continuations, UUID helper usage, desktop sizing, a status announcement, idempotent worker lifecycle with goleak coverage, and a root-object MCP output schema.
- Review remediation: queue readiness wakes, captured-turn pause fencing, distinct parent delivery kinds, current task/session/parent authority checks, incarnation preservation and lifecycle cleanup, one bounded Inbox page with async counts/hiding, atomic withdrawal wake, async sidecar authorization, discriminated-union output schema, unsupported-surface gating, child/history cursors, async image route/UI, modal focus lifecycle, `task.question.changed` invalidation, and parser-valid public examples. Remediation regressions cover reparent/transfer/cancel/incarnation, stale pause, pause failure, withdrawal wake atomicity, parent delivery kinds, child/history pagination, async dismissal HTTP routes, schema modes, and blocking item IDs.
- `python3 scripts/list-docs.py validate`: passed. The catalog validated 425 decisions and 1871 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Repository documentation coverage preflight: passed for all twelve work orders, using the in-memory trigger described below.
- Requirement acceptance criteria are mapped to the corresponding implementation work orders. Relative links and changed-document whitespace checks passed.
- The implementation and design artifacts are included in the feature delivery commit.

Each work order owns its exact commands and final results.
Before the first frontend package command in a fresh worktree, run `(cd apps && pnpm install --frozen-lockfile)`.
Real PostgreSQL evidence requires `KANDEV_TEST_POSTGRES_DSN`. A skipped test is not PostgreSQL validation.
Managed E2E commands rebuild changed backend, web, and fixture binaries.

The design-package checks are:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/decisions docs/plans/async-agent-questions
git status --short -- docs/plans/async-agent-questions
```

The documentation coverage preflight validates every work order against the requirement and system-design documents.
The in-memory production-path trigger exercises the work-order validator without modifying source files.

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const packageRoot = 'docs/plans/async-agent-questions';
const paths = fs.readdirSync(packageRoot)
  .filter(name => name.endsWith('.md'))
  .map(name => `${packageRoot}/${name}`);
paths.push(
  'docs/specs/tasks/requirements/async-agent-questions.md',
  'docs/specs/tasks/system-design/async-agent-questions.md',
  'docs/decisions/2026-10-30-async-question-lifecycle.md',
);
const fileContents = Object.fromEntries(
  paths.map(path => [path, fs.readFileSync(path, 'utf8')]),
);
const changedFiles = [...paths, 'apps/backend/internal/mcp/server/server.go'];
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({
  ok: result.ok, status: result.status,
  workOrders: result.workOrders.length, errors: result.errors,
}, null, 2));
if (!result.ok || result.workOrders.length !== 12) process.exit(1);
NODE
```

## Public documentation impact

Public documentation was updated with the implemented behavior:

- `docs/public/automation-and-mcp.md` owns the MCP tool and HTTP API reference.
- `docs/public/agent-communication.md` explains how agents choose and use async questions.
- `docs/public/coordination.md` explains the task and parent behavior.
- `docs/public/websocket-api.md` documents `task.question.changed`.
- `docs/public/coverage.json` maps the added MCP tools and API routes to their documentation.

The root README and screenshot catalog have no additional affected question guidance.

## Related delivery records

The implemented `task-autopilot`, `peer-message-live-clarification`, `user-question-turn-boundary`,
`clarification-active-lifecycle`, and `clarification-response-reliability` packages provide compatibility controls.
Their completed statuses and historical results remain intact.
This package introduces new acceptance IDs and tests, rather than reopening those work orders.
Implementation must preserve their relevant controls while qualifying mode-specific prompt wording.

## Risks

- The new lifecycle spans durable state, hierarchy authorization, queue admission, and guarded cancellation.
  The work orders require failure-injection evidence at each handoff.
- A queued receipt proves admission, not provider execution. Preserve existing uncertainty handling after dispatch.
- Every Inbox count producer must adopt the typed population together. Updating only the mounted list would leave the unopened badge wrong.
- Forwarding must not grant an Autopilot intermediary operator access or permit an unforwarded parent request through an operator route.
- The active user-question and Autopilot documents contain unconditional blocking wording.
  Implementation must scope that wording to omitted/false mode without rewriting current-turn ownership.
- The PR base can advance during implementation. Recheck the authoritative base before pushing and during PR fixup.
- A parked child whose parent task has no active session stays parked until a parent session exists or an operator prompts the child. The request stays readable, but no surface flags this to the operator.
- The parked-gate pending-action projection was traced through the existing composer and clarification readers and covered by SQLite projection tests.
- With the Inbox off, an open question on a running task has no task-list indicator. Discovery relies on the notification and on opening the task.
- The PR already touches 114 files. These work orders add to it, so reviewers may need a walkthrough update.
- Mode guidance can only be checked for presence. Whether agents follow it stays unmeasured until a real-agent evaluation runs.
