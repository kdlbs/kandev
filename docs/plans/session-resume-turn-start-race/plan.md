---
created: 2026-10-03
status: completed
requirements:
  - REQ-TASKS-RESUME-PROMPT-QUEUE-001
system_design:
  - ../../specs/tasks/system-design/resume-prompt-queue.md
legacy_specs: []
---

# Implementation Plan: Preserve resume during message-triggered transitions

## Overview

Preserve a session's startup claim when a user message triggers a workflow
transition during resume. First correct state preparation with deterministic
backend regressions. Then verify direct and queued delivery through the web
application's existing actions on desktop and mobile.

The task system owns session admission and workflow transitions. Extend the
[resume prompt requirement](../../specs/tasks/requirements/resume-prompt-queue.md)
and its [design](../../specs/tasks/system-design/resume-prompt-queue.md).
The existing `.3` through `.5` define readiness and queue barriers.
New `.10` and `.11` define the workflow/startup interaction explicitly.

## Confirmed defect and reproduction

The retained trace concerns task `ae874f35-2ac6-4894-b8d3-aca380292a8b`,
session `39afdcd1-ee2b-4dd3-8ec7-914c68eadb53`, and deployed commit
`59687fafa4f151512e666dc8f762f5c2fb10cc89`.
The selected provider was Codex ACP with model `gpt-6-luna`.

Times below use UTC on 2026-10-03:

| Time | Evidence |
| --- | --- |
| 22:19:24.088 | Automatic session resume begins after opening the task |
| 22:19:26.170 | The user message triggers `on_turn_start` and a step transition |
| 22:19:26.190 | The task projection includes a `STARTING` primary session |
| 22:19:26.203 | Workflow preparation changes that session to `WAITING_FOR_INPUT` |
| 22:19:26.255 | Kandev records the user message and starts its turn |
| 22:19:26.268 | Dispatch rejects the now-`FAILED` session |
| 22:19:26.288 | Resume reports a state conflict before runtime persistence |
| 22:19:38.185 | Later manual resume reaches `WAITING_FOR_INPUT` |

The message was `fix conflicts, make the ci green`.
Its ID was `50114592-6a54-4395-b266-1a356274a0ff`.
Its turn was `ba43692c-2afc-4141-9005-4b553fed452a`.
The recorded error was `session not promptable: session is in FAILED state`.
The launch error was `state changed from STARTING to WAITING_FOR_INPUT before runtime persistence`.

Source inspection confirms the cause. The engine turn-start branch preserves
`RUNNING`, but otherwise calls `setSessionWaitingForInput` after recipient
selection. That call invalidates the resume's `STARTING` persistence guard.
The message then encounters the launch-failure state before agent dispatch.

The diagnostic ZIP was partial because of its byte limit.
The investigation used the retained backend file and exact session conversation
to recover the complete incident window. No live-state repair occurred.

The smallest regression starts an eligible session in a step with an
`on_turn_start` transition. Claim `STARTING`, pause before credential-snapshot
persistence, process the user transition, then resume persistence.
The current implementation demotes startup and rejects the persistence write.

## Scope

### In scope

- Engine and legacy turn-start preparation for the resolved session recipient.
- The same state rule for WIP deferral during turn-start.
- A stale input-mode direct submission and normal startup queue admission.
- Startup, admitted-turn, terminal-state, error, queue-policy, and identity preservation.
- Deterministic backend regressions and focused desktop/mobile E2E.

### Out of scope

- Replaying historical failed messages, including the incident message.
- Provider protocol changes, credential authorization changes, and relaxed resume guards.
- New recovery controls, layout, copy, queue policy, or workflow definitions.
- New schema, runtime flag, broad lifecycle refactor, or additional agent sessions.
- Changes to genuine boot-ready, turn-completion, or cancellation settlement.

## Technical approach

Add a narrow workflow state-preparation helper in a small companion file.
Read the resolved recipient's current state and preserve `STARTING` and
`RUNNING`. Preserve ready and terminal rows. Use strict conditional state
publication for the existing `CREATED`/`IDLE` waiting-state preparation.
A lost write must preserve its successor and must not publish false Review.

Wire the helper into `applyEngineTransitionWithCommitMode` for turn-start,
including its WIP branch, and the legacy `executeStepTransition` turn-start
branch. Keep profile selection and existing route ownership unchanged.
Propagate preparation errors through `recordWorkflowTransitionError` where
available. Remove the legacy read-error fallback that blindly settles waiting.

Keep `persistResumeStateWithOptions`, `persistSessionFullRowIfCurrentState`,
credential issuance, attempt-fenced rollback, and launch-failure handling strict.
The correction belongs at the workflow writer that violates startup ownership.

Preserve ordinary message-handler fallback and the existing
`MetaKeyTurnStartAlreadyProcessed` marker. Change handler code only if regression
evidence exposes a separate delivery gap necessary for this repair.

| Path | Identity/capability | Intended behavior | Evidence |
| --- | --- | --- | --- |
| Codex and other ACP sessions | Existing session, startup attempt, saved conversation | Preserve resume and deliver once after readiness | Credential-boundary Go test and mock ACP E2E |
| Passthrough sessions | Existing session and transport-specific admission | Same state rule, existing terminal input delivery | Workflow state matrix and existing passthrough tests |
| Same or different profile recipient | Existing resolved destination | Use that destination's current state | Profile-switch state matrix |
| Missing, foreign, terminal, or unreadable destination | No eligible admission | Preserve state and existing error/barrier behavior | Negative Go regressions |

No claim of provider-by-provider live testing is made. The repair uses the
shared task lifecycle boundary and the existing mock ACP transport.

## Tests

All suffixes refer to `AC-TASKS-RESUME-PROMPT-QUEUE-001`.

| Criteria | Planned executable evidence |
| --- | --- |
| `.4`, `.10` | `TestOnTurnStartDuringResumePreservesStarting`, `TestResumeCredentialSnapshotSurvivesTurnStart` |
| `.4`, `.10` | `TestTurnStartPreparationPreservesConcurrentStartup`: stale snapshot and lost conditional write |
| `.5`, `.11` | `TestTurnStartPreparationPreservesTerminalState`: cancellation, failure, completion, errors |
| `.7`, `.11` | `TestResumeTurnStartGenuineFailureRetainsRecovery`: accepted input retained after a real launch failure |
| `.8`, `.10` | `TestTurnStartPreparationRecipientState`: same/new recipient and current identity |
| `.3`, `.4`, `.5`, `.10` | Existing Send Now/FIFO tests plus legacy and WIP startup cases |

Use new small test files. Do not append to oversized workflow or resume tests.
Channel barriers control the credential boundary. Count launch and dispatch
calls, transition records, user messages, and state events. Do not use sleeps
as proof of ordering. Tests must fail on the current writer before correction.

## E2E tests

Task 02 adds `session-resume-turn-start.spec.ts` in the `chromium` project
and `mobile-session-resume-turn-start.spec.ts` in `mobile-chrome`.
Both live under `apps/web/e2e/tests/session/`.
Use a shared helper in `apps/web/e2e/helpers/session-resume-turn-start.ts`.

Desktop submits through `ApiClient.addUserMessage` while an actual resume is
held by the existing mock-agent resume delay. This models the valid direct
request from a browser that has not received the startup event. Assert that
the workflow moves once and startup remains intact until genuine readiness.
After readiness, assert one response, one user message, retained conversation,
and no new prompt or startup error (`.3`, `.4`, `.8`, `.10`, `.11`).

Mobile uses the existing composer and a real tap with startup queue admission.
Use a workflow whose later drain evaluates `on_turn_start`. Prove one transition
and one response. Retain paused-queue coverage, touch reachability, and no
horizontal overflow (`.3`, `.5`, `.9`, `.10`).

The mock delay runs after credential-snapshot persistence. Browser tests do
not replace the earlier-boundary regression in Task 01. No production test hook
or new delay setting is required. Disable retries for the new regressions.

There is no rendered UI change, so no new layout preview is required.
Existing task-chat composition and recovery presentation remain the exemplars.

## Work orders

- [x] [Task 01: Preserve startup during workflow turn-start](task-01-preserve-startup.md) (done)
- [x] [Task 02: Verify resumed message delivery](task-02-verify-delivery.md) (done)

Order: Task 01, then Task 02. Execution is sequential.

## Verification results

Design validation on 2026-10-03:

- `python3 scripts/list-docs.py validate`: passed, 347 decisions and 1331 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- PR-documentation coverage preflight: passed, both work orders have complete references.
- `git diff --check -- docs/specs docs/plans`: passed.

Task 01 implementation and backend checks: passed. Task 02 desktop and mobile
E2E checks and frontend typecheck: passed. Final validation on 2026-10-04:

- `python3 scripts/list-docs.py validate`: passed, 347 decisions and 1331 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- PR-documentation coverage preflight with the implementation paths and both changed work orders: passed, `covered` with no errors.
- `git diff --check`: passed.

Review follow-up validation on 2026-10-04:

- The focused route-persistence regression was red before the correction in both legacy and engine modes, then passed after the correction.
- The race-enabled orchestrator selector passed with the new regression, resume/cancellation coverage, WIP, and Send Now cases.
- The focused executor and task-handler selectors passed.
- `make -C apps/backend build`: passed.
- Desktop managed E2E selector with retries disabled: 4 passed.
- Mobile managed E2E selector with retries disabled: 3 passed.
- `pnpm run typecheck`: passed.

The requirement is active and its system design is current after conformance.
Each work order records its exact commands and results.

## Verification notes

Run this documentation coverage preflight from the repository root:

```bash
node <<'JS'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const root = 'docs/plans/session-resume-turn-start-race/';
const tasks = ['task-01-preserve-startup.md', 'task-02-verify-delivery.md']
  .map(name => root + name);
const docs = [
  ...tasks,
  root + 'plan.md',
  'docs/specs/tasks/requirements/resume-prompt-queue.md',
  'docs/specs/tasks/system-design/resume-prompt-queue.md',
];
const fileContents = Object.fromEntries(docs.map(name => [
  name, fs.readFileSync(name, 'utf8'),
]));
const changedFiles = [
  { filename: 'apps/backend/internal/orchestrator/event_handlers_workflow.go', status: 'modified' },
  { filename: 'apps/backend/internal/orchestrator/workflow_turn_start_profile_error_test.go', status: 'added' },
  ...tasks.map(filename => ({ filename, status: 'added' })),
];
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({ ok: result.ok, status: result.status, errors: result.errors }, null, 2));
if (!result.ok) process.exitCode = 1;
JS
```

The production path is a planned trigger for this local coverage check.
Package creation changes documentation only. This preflight validates the
reference chain without staging files or pretending production changed.
At delivery, use the actual changed production paths and work-order statuses.

## Risks

- A source snapshot can differ from the resolved recipient's current row.
- A read-only state check can still race unless the subsequent write is conditional.
- Broad waiting-state changes can break completion or cancellation settlement.
- Holding a lifecycle lock across credential issuance can deadlock another admission path.
- A browser-only delay cannot reproduce the credential-persistence boundary.
- Suppressing the persistence error conceals the defect and weakens startup authorization.

## Related records

- [Earlier completed queue package](../resume-prompt-queue/plan.md).
- [Server-owned Auto-run](../../decisions/2026-08-16-server-owned-queue-auto-run.md).
- [Session opening resumes conversation](../../decisions/2026-09-18-session-open-resumes-conversation.md).
- [Agent recovery](../../specs/agents/requirements/agent-resume-runtime-recovery.md), especially `.1.3` through `.1.5`.

The earlier package retains its completed tasks and results. This package owns
the added regression matrix. Public documentation needs no update because
the repair restores already-documented behavior without a new user action.
