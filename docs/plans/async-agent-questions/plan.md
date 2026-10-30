---
created: 2026-10-30
status: implemented
requirements:
  - REQ-TASKS-ASYNC-QUESTIONS-001
  - REQ-TASKS-ASYNC-QUESTIONS-002
  - REQ-TASKS-ASYNC-QUESTIONS-003
  - REQ-TASKS-ASYNC-QUESTIONS-004
  - REQ-TASKS-ASYNC-QUESTIONS-005
  - REQ-TASKS-ASYNC-QUESTIONS-006
  - REQ-TASKS-ASYNC-QUESTIONS-007
  - REQ-TASKS-ASYNC-QUESTIONS-008
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
All work orders remain pending. This design turn authorizes no production or permanent test edits.

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

- The [task-owned requirements](../../specs/tasks/requirements/async-agent-questions.md), `REQ-TASKS-ASYNC-QUESTIONS-001` through `008`.
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

## Technical approach

Use the [system design](../../specs/tasks/system-design/async-agent-questions.md) as the technical source.
The [proposed ADR](../../decisions/2026-10-30-async-question-lifecycle.md) records the lifecycle and delivery boundary.

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

## Work orders

Execute sequentially. No subagents are authorized.

- [x] [Task 01: Persist async requests and delivery intents](task-01-persist-async-questions.md)
- [x] [Task 02: Deliver operator async questions](task-02-operator-question-flow.md)
- [x] [Task 03: Route and forward async parent questions](task-03-parent-question-flow.md)
- [x] [Task 04: Pause only for explicit question dependencies](task-04-explicit-question-waits.md)
- [x] [Task 05: Integrate desktop and phone Inbox flows](task-05-inbox-and-mobile.md)

## Verification results

- Investigation baseline: the two existing parent-pause tests passed, as recorded above.
- Feature implementation: complete. Targeted Go, web unit, typecheck, i18n, and lint checks passed.
- Real PostgreSQL race coverage: test file added and gated on `KANDEV_TEST_POSTGRES_DSN`; skipped locally because the DSN was not set.
- Desktop and phone browser checks: E2E specs and mock scenarios were added but not executed in this environment.
- Review remediation: queue readiness wakes, captured-turn pause fencing, distinct parent delivery kinds, current task/session/parent authority checks, incarnation preservation and lifecycle cleanup, one bounded Inbox page with async counts/hiding, atomic withdrawal wake, async sidecar authorization, discriminated-union output schema, unsupported-surface gating, child/history cursors, async image route/UI, modal focus lifecycle, `task.question.changed` invalidation, and parser-valid public examples. Remediation regressions cover reparent/transfer/cancel/incarnation, stale pause, pause failure, withdrawal wake atomicity, parent delivery kinds, child/history pagination, async dismissal HTTP routes, schema modes, and blocking item IDs.
- `python3 scripts/list-docs.py validate`: passed. The catalog validated 425 decisions and 1863 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Repository documentation coverage preflight: passed for all five work orders, using the in-memory trigger described below.
- Artifact audit: all 30 acceptance criteria appear in work-order frontmatter. All nine new documents have valid relative file links and clean whitespace.
- `git diff --check` and package status inspection: passed. All nine package files remain unstaged and uncommitted.

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

The documentation coverage preflight validates every new work order against the new requirement/design pair.
The in-memory production-path trigger exercises work-order validation for this docs-only draft.
It does not modify or stage that source path.

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
if (!result.ok || result.workOrders.length !== 5) process.exit(1);
NODE
```

## Public documentation impact

This turn changes internal documents only.
At implementation, update `docs/public/automation-and-mcp.md` as reference, `agent-communication.md` as a how-to guide, and `coordination.md` as explanation.
Update `websocket-api.md` for the new event contract, and `coverage.json` only if its published-source mapping changes.
Searches of the root README and screenshot catalog found no additional affected question guidance.
Do not publish proposed APIs as shipped before implementation.

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
- Source and remote main can advance after this snapshot. Recheck the affected boundaries before Task 01.
