---
created: 2026-09-25
status: done
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
system_design:
  - ../../specs/tasks/system-design/workflow-profile-readiness.md
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
  - ../../specs/agents/system-design/dynamic-agent-routing-01.md
legacy_specs: []
---

# Implementation plan: Dynamic workflow completion

## Overview

Restore workflow advancement for Dynamic profiles and promptability after rejected completion transitions.
One work order owns the regression tests, correction, and browser evidence. Implementation is complete.

## Evidence and root cause

The supplied bundle records build `25271b6e7`, macOS arm64, on 2026-09-25.
Task `d6b9d34e-4129-40fe-8528-90dec2caa281` uses session `48bfdbf8-689e-4523-93bd-f7fa20981f69`.
Four successful turns encounter the same rejection at 10:12:42, 10:54:10, 11:00:43, and 11:40:20 (UTC+01:00).
The error ends with `agent profile belongs to a virtual execution family`.

At 11:40:13, Cursor emits the reported preparation thought. It returns `end_turn` at 11:40:17.
READY completes the turn at 11:40:20, but the destination preflight rejects the Dynamic profile.
The complete-stream path then defers readiness to an already-handled READY event.
The backend retains `RUNNING` with no active turn and rejects follow-up prompts.
Cancellation restores readiness. One captured occurrence persists for approximately 27 minutes.

`sessionHasUnauthorizedExactModelDrift` resolves a logical Dynamic profile through the concrete lifecycle resolver.
The resolver correctly rejects it. The completion preflight failure path then returns without restoring session readiness.
Both paths exist in the reporter commit and investigated checkout `3aa3233c78`.
The bundle contains partial retained browser history. Backend and ACP evidence establish this sequence without a live reproduction.
The archive is task-local evidence, not an implementation prerequisite. Do not copy private transcript content into tests.

## Scope

### In scope

- Existing reuse criteria 001.1 and 001.5 for Dynamic logical profiles.
- Existing recoverability criterion 001.11 and the precise readiness outcome in new criterion 001.14.
- Current-session and parked-session checks, completion error settlement, and follow-up acceptance.

### Out of scope

- Provider routing policies, model fallback, new flags, schema changes, and stall timers.
- UI layout, copy, controls, Office behavior, and broad transition refactoring.
- Changing concrete exact-model enforcement or permitting virtual CLI launches.

## Technical approach

Follow [profile readiness](../../specs/tasks/system-design/workflow-profile-readiness.md).
Handle only wrapped `agent/runtime.ErrVirtualProfile` in the shared exact-model drift helper; the runtime facade re-exports the lifecycle sentinel.
Keep ordinary lookup errors fatal and retain concrete exact-model checks.
At rejected completion preflight, settle the owned completed source through existing state publication.
Do not apply that settlement to live manual moves, turn-start transitions, or guarded decision calls.

Use `newProfileSwitchFixture` and real session rows for orchestration assertions.
Use the real settings-backed lifecycle resolver in one case to prevent permissive mocks from hiding the bug.
Retain session guards, route identities, transition markers, managed credential checks, and existing queue behavior.
No public documentation change is needed: this repair restores documented session reuse and recovery.

## Tests

Planned tests belong in `apps/backend/internal/orchestrator/event_handlers_workflow_dynamic_completion_test.go`.

| Test                                               | Evidence                                                                                                    | Criteria                                  |
| -------------------------------------------------- | ----------------------------------------------------------------------------------------------------------- | ----------------------------------------- |
| `TestWorkflowDynamicProfileReuse`                  | Default/explicit reuse, current/parked sessions, real virtual resolver, no route reset                      | 001.1, 001.5; Dynamic 001.1, 001.3, 001.7 |
| `TestWorkflowDynamicCompletionAdvances`            | READY completes one turn, advances once, preserves logical session, reaches waiting or next legitimate turn | 001.5                                     |
| `TestWorkflowProfileLookupFailureRecoversCompletedSession` | Profile lookup failure preserves the source and publishes waiting                                        | 001.11, 001.14                            |
| `TestWorkflowCredentialPreflightFailureDispatchesFollowUp` | READY/complete-stream rejection preserves the source and dispatches one follow-up prompt and turn         | 001.14                                     |
| `TestWorkflowPreflightFailurePreservesLiveTurn`    | Manual, turn-start, and guarded-decision failures cannot settle active work                                 | 001.14                                    |
| `TestWorkflowCompletionRecoveryPreservesNewerTurn` | Duplicate/delayed completion cannot settle a newer turn or reopen a terminal session                        | 001.14                                    |

Run existing profile-policy tests as controls for concrete exact-model mismatch, missing models, lookup errors, and same-profile `new`.
Tests must assert persisted state, session identity, event payloads, and prompt recipients, not only helper calls.

## E2E tests

Add `apps/web/e2e/tests/workflow/dynamic-workflow-completion.spec.ts`, project `chromium`.
Use the isolated backend and `backend.useEnv` to enable the existing Dynamic flag for this test only.
Restore the environment in `finally`, as the Dynamic settings tests do.
Seed one Dynamic profile with a mock concrete candidate and consecutive steps using that profile with `reuse`.
Let the first turn complete and advance automatically to a step without automatic launch.
Assert the same primary session, destination step, completed response, and ready composer.
Send a second unique prompt through the composer and assert one accepted turn and its response without cancellation.
Reload before the follow-up in one case to prove persisted readiness. This covers 001.5 and Dynamic profile compatibility.
Failure recovery criterion 001.14 uses backend integration evidence through state publication and prompt admission.
Do not inject browser store state to simulate successful backend settlement.

This repair changes shared backend state only. Desktop and phone consume the same session events.
Mobile parity review requires no new layout or touch behavior. No ASCII preview is needed.
Existing mobile workflow targeting coverage remains applicable. The new backend event assertions cover the recovery state for both clients.

## Work orders

- [x] [Task 01: Restore Dynamic workflow completion](task-01-restore-completion.md) (done)

## Related delivery records

`workflow-session-targeting` and `workflow-session-focus` record completed recipient and presentation work.
`workflow-same-profile-new-session` records the completed `new` policy repair.
`queued-session-ownership` remains in progress and owns inspection/admission behavior.
This package does not reopen their tasks or claim their results as new evidence.
It adds the missing Dynamic completion cases and retains their recipient, queue, and stop-intent contracts.

## Verification results

Implementation and design checks on 2026-09-25 passed:

- Red regressions reproduced Dynamic virtual-profile rejection and the stranded `RUNNING` session before the production fix.
- `(cd apps/backend && go test ./internal/orchestrator -run '^TestWorkflow(Dynamic|ProfileLookupFailure|CredentialPreflightFailure|Completion|PreflightFailure)' -count=1)`
- `(cd apps/backend && go test -race ./internal/orchestrator -run 'TestWorkflow(Dynamic|ProfileLookupFailure|CredentialPreflightFailure|Completion|PreflightFailure)|TestPrepareWorkflowStepSession|TestPreflightWorkflowStepCredentials|TestExactModelWorkflowStartPolicy|TestHandleAgentReady|TestHandleCompleteStreamEvent' -count=1)`
- `(cd apps/backend && go test ./internal/agent/runtime/lifecycle -run 'Test.*Profile' -count=1)`
- `make -C apps/backend lint` (0 issues)
- `(cd apps/web && pnpm e2e:run --project chromium tests/workflow/dynamic-workflow-completion.spec.ts tests/workflow/workflow-session-targeting.spec.ts)` (6 tests passed, including both specs)
- `(cd apps/web && pnpm exec eslint --max-warnings 0 e2e/tests/workflow/dynamic-workflow-completion.spec.ts)`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`
- `pnpm exec prettier --check` on the new E2E test and formatted plan/spec files.
- `pnpm exec eslint --max-warnings 0` on the new E2E test.
- Managed E2E build completed for the backend and Vite web assets.
- Frontend dependency installation was unnecessary because `apps/node_modules` was present.

PR fixup verification on 2026-09-25 also passed: targeted orchestrator race tests, changed-code `golangci-lint` (0 issues), and the managed Chromium workflow scenarios with retries disabled (6 passed). The local PR documentation coverage evaluator reported `covered` after the work order linked the Dynamic routing design. Documentation validation and specification lint passed.

The new browser regression cleans up its Dynamic profile while the feature flag is enabled.

## Risks

- Ignoring all resolver errors can reuse deleted or unavailable profiles.
- Settling every failed transition can interrupt a live manual-move source or a newer turn.
- A helper-only test can miss the READY/complete-stream ordering and prompt admission failure.
- Concurrent queue changes can affect the guard boundary. Preserve existing ownership tests.
