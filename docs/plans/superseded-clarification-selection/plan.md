---
created: 2026-11-02
status: completed
requirements:
  - REQ-TASKS-CLARIFICATION-LIFECYCLE-001
  - REQ-TASKS-CLARIFICATION-SCENARIOS-001
system_design:
  - ../../specs/tasks/system-design/clarification-active-lifecycle.md
legacy_specs: []
---

# Implementation Plan: Keep superseded questions out of active clarification

## Overview

Repair [issue #5002](https://github.com/kdlbs/kandev/issues/5002).
After a newer question resolves, chat must close its active panel without
selecting a detached question from an older turn. One sequential work order
covers the shared selector, regression coverage, and desktop/phone checks.

This package is implemented. The selector no longer grants detached questions
turn authority, and desktop/phone regressions cover the resolved state. Issue
#5002 is assigned to `carlosflorencio`.

## Evidence and root cause

Investigation base: `f648e79f9ecc7d5db0db301f20d0b23c7cb44d97`, matching
remote `main` on 2026-11-02. The issue has no image attachments or comments.

The synthetic reproduction uses an older completed turn with a detached pending
question A and a newer open turn with question B. Its actual selector output was:

| Input state | Selected IDs | Required IDs |
| --- | --- | --- |
| B pending, projection `clarification` | `Q-new` | `Q-new` |
| B answered, projection null | `Q-old` | None |
| B rejected, projection null | `Q-old` | None |
| B answered, A not detached | None | None |

In `clarificationMessagesInScope`, null projection enables `allowDetached`.
The function returns all messages before applying the durable-turn filter.
`findPendingClarification` then exempts detached rows from its newest-message
boundary. Clearing B's projection therefore exposes A. The explicit
`allowDetached` option can produce the same bypass without a null projection.

`use-chat-panel-state.ts` already supplies the current durable turn, completion
state, and session action. `useProcessedMessages` forwards that scope. The
incorrect selected row drives the overlay and pending-question composer state.

Existing selector suites passed: two files, 48 tests. One test explicitly
expects an older detached question after a newer turn starts. That expectation
conflicts with the active lifecycle requirement and must change.

A temporary diagnostic suite produced five expected failures and one passing
control. Failures covered answer, rejection, deleted newer-turn messages, clean
projection over a detached predecessor, and the explicit detached override.
The control preserved a current detached question with `clarification`
authority, including a completed durable turn. The temporary file was removed.
No browser or running-instance reproduction was performed during planning.

## Requirement conformance and ownership

The task system owns bundle authority and supersession. UI rendering consumes
that contract. The repair reuses these active documents without adding a requirement:

- [Lifecycle requirements](../../specs/tasks/requirements/clarification-active-lifecycle.md):
  `AC-TASKS-CLARIFICATION-LIFECYCLE-001.1` and `.2` define current ownership.
  Criteria `.4` and `.9` preserve historical answers as ordinary messages.
- [Lifecycle scenarios](../../specs/tasks/requirements/clarification-active-lifecycle-scenarios.md):
  `AC-TASKS-CLARIFICATION-SCENARIOS-001.1` covers supersession, explicit clean
  projections, and deletion of newer-turn messages.
- [System design](../../specs/tasks/system-design/clarification-active-lifecycle.md#frontend-clarification-selection)
  now makes frontend authority precedence explicit.
- [Current-turn decision](../../decisions/2026-08-14-current-turn-clarification-ownership.md)
  and [late-answer decision](../../decisions/2026-09-18-late-clarification-messages.md)
  already settle the behavior. No new ADR is needed.

Confirmed scope is investigation, assignment, and a fix package. Source and
active contracts establish intended behavior. No material product choice remains unresolved.

## Scope

### In scope

- Enforce projection and durable-turn authority in every shared clarification selector.
- Replace the conflicting expectation and add focused transition regressions.
- Preserve current detached bundles, legacy fallback, and bundle completeness.
- Prove that history and late-answer controls survive on desktop and phone.

### Out of scope

- Backend, agent adapter, response endpoint, persistence, or schema changes.
- Rewriting historical metadata or modifying a developer's running instance.
- New question-history discovery, including the separate proposal in issue #4820.
- Changes to layout, dismissal, permission requests, or ordinary message admission.

## Technical approach

### Shared selector

In `apps/web/lib/utils/pending-clarification.ts`, remove the detached exceptions
from both `clarificationMessagesInScope` and `findPendingClarification`.
Apply projection rejection before turn filtering. Null means explicitly clean.
Undefined means that projection evidence is unavailable.

A string turn ID restricts every candidate to that turn. Null turn authority
keeps discovery inert. Undefined turn authority permits the existing fallback
only with a `clarification` projection. Preserve unscoped legacy callers.

Remove `hasDetachedPendingClarification` and the unused `allowDetached` escape
hatch after confirming its caller inventory remains empty. Correct the scope
comments. Keep completed-turn and terminal-session meanings distinct. Avoid
unrelated changes to turn ranking or timestamp normalization.

`findPendingClarification`, `findPendingClarificationGroup`, and the boolean
helpers must agree. Bundle construction remains limited to eligible messages
and the selected `pending_id`. Preserve partial-arrival gating and mixed
terminal/pending siblings. Another eligible same-turn bundle must not disappear
when the latest bundle resolves and projection still advertises clarification.

### Chat and pending indicators

Keep the existing scope wiring in `use-chat-panel-state.ts` and
`use-processed-messages.ts`. Add coverage beside the filtering tests in
`hooks/use-processed-messages.test.ts` and the pending-input tests in
`hooks/use-task-pending-input.test.tsx`.

After B resolves, A remains transcript history. The active result is null,
the bundle is empty, and loaded-session pending flags are false. Existing
historical controls retain their separate message-delivery behavior.

### Rendered regression fixture

Reuse `ApiClient.seedTaskSession` and `seedSessionMessage` through
`e2e/helpers/clarification.ts`. Seed an isolated `RUNNING` session containing:

1. A completed older turn with A pending and `agent_disconnected=true`.
2. A newer open turn with B already answered or rejected.
3. Ordinary activity on the newer turn, using deterministic timestamps.

The normal backend projection must report null. Open the task through its
task route and verify the rendered result, then reload and repeat. Parameterize
answered and rejected outcomes. This fixture proves the state after resolution
without a two-hour timeout, a response mock, or a new agent scenario.

Keep the newer durable turn open throughout the assertion. The standard mock
clarification scenario finishes soon after answering. Waiting for that completion
can hide this bug because the current selector disables its exception afterward.

Existing real answer, Skip, timeout, and late-answer tests remain part of the
targeted browser commands. The synthetic fixture proves rendering and hydration,
not successful live-waiter delivery.

## ASCII UI preview

UI-01: Task chat after B resolves. Source inspection and the selector reproduction
support the before state. Desktop and phone share the affected region:

```text
Before                            After
Transcript: A, B resolved          Transcript: A, B resolved
Question A (agent moved on)        A: [Answer as new message]
Answer choices                    No active question panel
Composer: question pending         Composer: ordinary running-session mode
```

Phone entry remains the task's focused Chat surface. Desktop retains its chat
panel. Both use the existing transcript scroll owner and inline composer.
The required change is selection, not geometry. Wording and spacing are illustrative.

UI-02: A remains current after timeout, with projection `clarification`.

```text
Transcript: current question A
Question A (detached)
Answer choices / custom answer
Existing answer and Skip actions
```

This control remains available on desktop and phone, including reload. A
completed durable turn alone does not make its input-capable session terminal.

Nearest shipped examples are `ClarificationInputOverlay`, the shared
`clarification-input-overlay.tsx`, and `mobile-clarification.spec.ts`. Retain
their focus behavior, internal scrolling, safe-area clearance, and touch targets.
No new sheet, breakpoint, navigation action, or copy is required.

UI-01 maps to lifecycle AC `.2`, `.4`, `.9` and scenarios AC `.1`.
UI-02 maps to lifecycle AC `.1` and scenarios AC `.1`.

## Tests

| Acceptance | Regression evidence |
| --- | --- |
| Lifecycle `.1`, `.2` | `pending-clarification-scope.test.ts`: answer/rejection transitions, exact current turn, missing newer messages, current detached control |
| Scenarios `.1` | Scope tests: explicit clean projection, null/undefined turn history, terminal sessions, legacy status, partial bundles |
| Lifecycle `.2`, `.4` | `use-processed-messages.test.ts`: superseded A stays in visible transcript while active selectors are empty |
| Lifecycle `.2` | `use-task-pending-input.test.tsx`: task/session flags remain clean beside detached history |
| Lifecycle `.4`, `.9` | Existing `use-late-clarification-message.test.tsx` and desktop/phone late-answer browser tests |

The primary permanent regression name is
`does not reactivate an older detached bundle after the current bundle is answered`.
Its assertion must fail before production changes with actual `[Q-old]`
instead of `[]`. Add the equivalent rejected-bundle case. Replace
`keeps a detached clarification answerable after a newer turn starts` with
the active-contract expectation and a distinct current-turn positive control.

## E2E tests

- Extend `e2e/tests/chat/clarification-lifecycle-turn-shadow.spec.ts`, project
  `chromium`, with the seeded answered/rejected matrix. Assert no active panel,
  no pending-question composer wording, retained historical action, and clean
  session projection before and after reload. Preserve its existing turn-ranking tests.
- Extend `e2e/tests/chat/mobile-clarification.spec.ts`, project `mobile-chrome`,
  with the same matrix. Use `.tap()` to open and close A's historical answer
  surface. It must not reopen an active panel or submit a tool response.
- Run `clarification.spec.ts` as a compatibility guard for real answer, Skip,
  current detached recovery, and historical new-message delivery.
- Observe HTTP or WebSocket causes before interaction when applicable. Do not
  replace synchronization with fixed sleeps or larger UI timeouts.

## Work orders

- [x] [Task 01: Enforce current-turn clarification selection](task-01-enforce-current-turn-selection.md)

The work order is sequential and has no dependency. The package does not authorize delegation.

## Verification

The [work order](task-01-enforce-current-turn-selection.md#verification) owns
the exact implementation commands. It covers focused Vitest, ESLint, formatting,
typecheck, the copy ratchet, and separate managed desktop/phone E2E runs.
Managed runners rebuild the production assets and enforce the worker budget.

Document gates, from repository root:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/tasks/system-design/clarification-active-lifecycle.md docs/plans/superseded-clarification-selection
git status --short -- docs/specs/tasks/system-design/clarification-active-lifecycle.md docs/plans/superseded-clarification-selection
```

Run the repository's `validateCoverage` preflight with the changed work order,
its plan, both referenced requirements, and the referenced design. Include
the planned selector path when exercising coverage before implementation.
Documentation-only classification by itself does not prove delivery coverage.

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const files = [
  'docs/plans/superseded-clarification-selection/plan.md',
  'docs/plans/superseded-clarification-selection/task-01-enforce-current-turn-selection.md',
  'docs/specs/tasks/requirements/clarification-active-lifecycle.md',
  'docs/specs/tasks/requirements/clarification-active-lifecycle-scenarios.md',
  'docs/specs/tasks/system-design/clarification-active-lifecycle.md',
];
const changedFiles = [
  'apps/web/lib/utils/pending-clarification.ts', files[0], files[1], files[4],
].map(filename => ({
  filename,
  status: filename.includes('superseded-clarification-selection') ? 'added' : 'modified',
}));
const fileContents = Object.fromEntries(files.map(file => [file, fs.readFileSync(file, 'utf8')]));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({
  ok: result.ok,
  status: result.status,
  requiresCoverage: result.requiresCoverage,
  workOrders: result.workOrders,
  errors: result.errors,
}, null, 2));
if (!result.ok || !result.requiresCoverage) process.exitCode = 1;
NODE
```

## Verification results

Investigation completed on 2026-11-02:

- Synthetic selector reproduction confirmed both answer and rejection failures.
- Existing selector suites passed: two files, 48 tests.
- Temporary diagnostic suite: five expected failures and one passing control.
  The temporary source file was removed.
- GitHub identity and assignment verified: issue #5002 is open and assigned
  to `carlosflorencio`.
- Catalog validation passed: 425 decisions and 1,885 specifications.
- Specification-linter tests passed: 36 tests. All specification files passed lint.
- Delivery-coverage preflight passed with the planned selector path as a trigger.
  One changed work order had complete requirement, acceptance, plan, and design references.
- Relative Markdown link targets exist. The design remains within its 32 KiB limit.
- Package diff checks passed. Only the design and two delivery files were part
  of that documentation package.

Implementation completed on 2026-11-02:

- The shared selector now applies projection rejection before turn filtering and
  no longer grants detached metadata ownership. `allowDetached` and
  `hasDetachedPendingClarification` were removed after confirming no production
  callers.
- The permanent regression
  `does not reactivate an older detached bundle after the current bundle is answered`
  failed before the fix with `old-question` selected and passed afterward. The
  rejected equivalent and sparse-history matrix also pass.
- Focused selector and chat-state suites passed: 7 files, 132 tests. Broader
  `lib/utils` and `hooks` suites passed: 585 files, 5,695 tests.
- Web typecheck passed. ESLint reported no errors on the changed files, and
  Prettier passed.
- The rendered matrix passed in the managed Docker E2E runner: desktop 32 tests
  across the lifecycle-shadow and clarification specs, phone 15 tests in the
  mobile clarification spec (managed `mobile-chrome` emulation only; no
  physical-device validation was available, so physical-device issue closure
  remains a separate gate). Both answered and rejected fixtures passed before
  and after reload.
- `pnpm run i18n:ratchet` passed. Catalog validation, all 36 specification-linter
  tests, and full specification lint passed.
- Implementation is committed in this PR.

## Related delivery records

[Active lifecycle](../clarification-active-lifecycle/plan.md),
[inactive dismissal and late answers](../clarification-inactive-dismiss/plan.md),
[watchdog recovery](../clarification-watchdog-race-recovery/plan.md), and
[peer messages](../peer-message-live-clarification/plan.md) are completed companion
records. This repair preserves their behavior and recorded results. It adds a
new selector correction and browser matrix rather than reopening their work orders.

## Documentation impact

Internal design and delivery records change. Public
[clarification guidance](../../public/tasks-and-workflows.md#answer-clarification-questions)
already describes the required current-question and historical-answer behavior.
No public copy, locale, screenshot, API, or configuration update is required.

## Risks

- Removing all detached behavior would break valid current-turn late answers.
- Dropping only the null-projection exception leaves the explicit override unsafe.
- Inferring authority from surviving messages can reactivate history after deletion.
- A browser assertion after turn completion can pass without exercising the defect.
- An answer-looking transcript row does not prove live-waiter delivery. Keep
  that claim separate from the seeded rendering regression.
