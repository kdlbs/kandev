---
created: 2026-09-18
status: complete
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-004
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-006
system_design:
  - ../../specs/tasks/system-design/workflow-step-agent-start-ownership.md
legacy_specs: []
---

# Implementation Plan: Initial creation prompt workflow transition

## Overview

Process the explicit starting step's `on_turn_start` before an immediate REST or MCP creation prompt reaches the agent.
One sequential work order owns the transport wiring, orchestration, regression tests, and public documentation.
Implementation is complete.

## Evidence and root cause

Source: [issue #3804](https://github.com/kdlbs/kandev/issues/3804), open with no comments or image attachments during investigation.
Investigated checkout: `33b7bd9bf0` on 2026-09-18.
The authenticated GitHub user is `carlosflorencio`; the issue was assigned to that account after reproduction.

The current source confirms both missing paths:

1. REST `prepareStartAgentSession` prepares a session. After settlement, `dispatchTaskSession` sends `IntentStartCreated` without turn-start processing.
2. MCP captures an explicit destination in `handleCreateTask`. `launchAutoStartTask` sends `IntentStart`, whose `startTask` path omits turn-start processing.
3. `StartCreatedSession` deliberately omits this trigger because ordinary messages already process it and workflow automatic starts must not cascade.
4. `handleAgentRunning` returns early for ACP sessions. No startup event repairs the missing transition.
5. Ordinary `wsAddMessage` calls `ProcessOnTurnStart` before composition and resolves the resulting session before dispatch.

A temporary `TestReproIssue3804` exercised real `LaunchSession(IntentStartCreated)` with SQLite, workflow evaluation, and a mocked agent manager.
It prepared a task on Backlog with a move-to-Spec turn-start action.
Creation recorded one user message while the task remained on Backlog.
The positive control called `ProcessOnTurnStart` and observed Spec.

```text
after creation dispatch: step=backlog user_messages=1
after explicit turn-start: step=spec
PASS (package 0.086s)
```

Command: `(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run '^TestReproIssue3804$' -count=1 -v)`.
The temporary test was removed after diagnosis.
This proves the orchestration omission; it does not reproduce the reporter's macOS runtime or provider transport.
MCP has a source trace at this stage, not an executed transport reproduction.

## Scope

### In scope

- Explicit-step, immediate REST and MCP creation with non-empty text.
- One transition before first-prompt composition and dispatch.
- Destination session/profile/settings, WIP queue admission, and duplicate-trigger protection.
- Negative controls for ordinary messages, workflow automatic starts, omitted steps, and empty text.

### Out of scope

- Historical task repair, dependency-deferred creation, attachment-only creation, and new workflow actions.
- Provider changes, new UI, schema changes, and broader launch refactoring.
- Executing production changes during this design turn.

## Requirement conformance

Requirement `004`, especially `AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-004.3`, already preserves explicit initial placement.
The missing creation-prompt semantics are added as draft requirement `006` in the existing owner.
An explicit placement and a subsequent configured transition are separate operations; the fix preserves both.
No new architectural owner or ADR is required. The existing task-system trigger and prompt-admission boundaries remain authoritative.

Companion packages were inventoried: start ownership, reset quiescence/failure containment, empty-prompt deduplication, initial task brief, and asynchronous prompt preservation.
Their existing scope and results remain unchanged. This work adds creation admission before their launch behavior.

## Technical approach

Follow [Initial creation prompt admission](../../specs/tasks/system-design/workflow-step-agent-start-ownership.md#initial-creation-prompt-admission).
Capture caller provenance before MCP resolves a destination.
Use an internal `InitialCreatePrompt` marker and a shared prepared-session helper.
For marked MCP requests, prepare without eager passthrough launch before processing the trigger.
For REST, use the prepared session already returned by creation.

Reuse `ProcessOnTurnStart`, destination-session resolution, queue metadata, and first-prompt composition.
Do not add an unconditional trigger to either generic launch method.
The helper must finish transition and WIP admission decisions before provider dispatch.
Passthrough duplicate suppression belongs to the initial execution/turn, not the whole session.

## Tests

| Criteria | Permanent regression evidence |
| --- | --- |
| `006.1`, `006.2` | `task_create_prompt_test.go` and `workflow_e2e_test.go`: transition before dispatch, resulting step, one prompt, and completion transition |
| `006.3` | `TestInitialCreatePrompt_PassthroughRunningDoesNotRepeatTurnStart`, `TestInitialCreatePrompt_PassthroughEvidenceSurvivesPredecessorTerminalEvents`, and `TestInitialCreatePrompt_QueueReplayTransfersPassthroughEvidence`; later passthrough input still transitions |
| `006.4` | `TestInitialCreatePrompt_QueuesAfterTurnStartAdmission`; the initial prompt remains queued with processed-trigger metadata, then releases through the admission-gated queue path with one delivery and no second transition |
| `006.5`, `004.1` through `004.3` | REST provenance and MCP prepared-start tests cover explicit versus inferred destinations; existing unmarked launch and workflow suites remain unchanged |
| `006.6` | `TestInitialCreatePrompt_TransitionFailurePreventsLaunch`, `TestInitialCreatePrompt_AdmissionFailureDoesNotFailSupersededSuccessor`, `TestInitialCreatePrompt_LaunchFailureUsesReplacementSession`, and `TestInitialCreatePrompt_QueueAdmissionFailurePersistsLaunchError` cover durable errors, correct replacement ownership, and zero prompt dispatch on admission failure |
| `006.1`, `006.5`, `006.6` | REST and MCP handler tests prove original explicit-step provenance and no dispatch before settlement or while blocked |

The primary regression must fail against the current implementation because the step remains Backlog at dispatch.
The temporary diagnosis asserted the defective behavior; it is not the permanent regression test.

## E2E tests

Use Go orchestration integration tests with the real workflow engine and SQLite plus an executor dispatch observer.
Extend `workflow_e2e_test.go` with `TestWorkflowE2E_InitialCreatePrompt` for Backlog -> Spec -> Spec Review.
Assert the destination step, replacement session profile, inherited executor settings, and one prompt-bearing process dispatch at provider launch, then Spec Review after eligible completion (`006.1` through `006.3`).
Adapter tests must connect both REST and MCP creation to the marked launch request.
An engine-only trigger table row is insufficient because it bypasses the missing creation call site.
No Playwright case is required: no rendered interface or browser interaction changes.

## Work orders

- [x] [Task 01: Admit the initial creation prompt through turn-start](task-01-initial-create-prompt.md)

## Verification results

- Diagnostic reproduction: passed, with the defective behavior and positive control recorded above.
- Permanent regression and implementation checks: passed. REST and MCP adapters, initial admission, WIP queueing, passthrough replay, transition failure containment, and workflow E2E coverage are implemented.
- A first combined race run exposed a transient suite failure; the isolated full orchestrator race run and the exact combined race command both passed on rerun.
- Backend build: passed with `make -C apps/backend build`.
- Backend lint: passed with `make -C apps/backend lint`.
- Public documentation checks: passed with the work order's Node validators.
- `python3 scripts/list-docs.py validate`: passed, 291 decisions and 1032 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- Package status inspection confirmed both new plan files. The temporary reproduction was removed.

## Risks

- A global trigger call can skip the target step by cascading its automatic start.
- MCP's resolved step does not retain whether the caller explicitly selected it.
- Profile switching can replace the prepared session; stale session dispatch can fail or revive old work.
- WIP queue drain and passthrough running events can evaluate the trigger twice without scoped evidence.
- Engine helpers currently log some internal failures without returning them. Keep any error propagation change confined to the creation admission path.
- Concurrent cancellation or replacement must retire any initial-turn suppression evidence.
