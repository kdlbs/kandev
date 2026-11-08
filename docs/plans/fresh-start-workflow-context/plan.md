---
created: 2026-11-07
status: completed
requirements:
  - REQ-TASKS-PROMPT-ATTACHMENTS-002
system_design:
  - ../../specs/tasks/system-design/prompt-attachments.md
legacy_specs: []
---

# Fix plan: Preserve workflow context during fresh recovery

## Overview

Fresh recovery must apply the workflow instructions that constrain the first agent turn.
One sequential work order repairs both initial-submission paths and adds service-level regression coverage.
The package is implemented and verified.

Issue: [#5137](https://github.com/kdlbs/kandev/issues/5137).
GitHub assignment to `carlosflorencio` is complete and verified.
Investigation snapshot: `52309e613fc9a76d38a9f891b74b4d8b32bff265`.

## Evidence and root cause

The issue contains no comments or image attachments.
The reporter observed a failed first launch followed by fresh recovery without its workflow instructions.
This investigation reproduced the recovery defect with persisted test state and a controllable agent manager.
It did not reproduce the original provider bootstrap error or inspect the reporter's live instance.

The initial launch composes workflow instructions through `applyWorkflowAndPlanMode` before recording and dispatch.
Fresh recovery omits that composition in two places:

1. Without an eligible receipt, `RecoverSessionWithOptions` launches `IntentResume`.
   `newResumeLaunchRequest` assigns `task.Description` to `TaskDescription`.
   The applicable workflow template never reaches this request.
2. With a pending receipt, recovery starts without a prompt and calls `replayInitialPromptSubmission` after readiness.
   `wrapInitialSubmissionReplayPrompt` restores current MCP policy through `wrapCreatedSessionPrompt`.
   It does not apply the workflow or step template to the raw submission.

A disposable `TestRepro5137FreshStartWorkflowPrompt` entered the real `RecoverSession` path with SQLite task/session state.
The first stored user message retained the workflow constraint in both cases.
The test failed for the intended reason in both subtests:

- `legacy_text_fallback`: launch input contained only `Report the requested change`.
- `pending_initial_submission`: provider input contained the MCP block and that description.

Both omitted `[REPORT ISSUE PHASE] Only report a GitHub issue. Do not implement.`
The test also observed one recovery input and one description occurrence.
The temporary fixture is removed at the design handoff.

The existing `TestFreshStartReplayUsesCurrentFirstLaunchInstructions` passed.
Its assertions cover tools and signal policy, without a nonempty workflow template.
The existing `TestWorkflowAsyncStartFailure_PreservesPromptThroughRecovery` also passed.
That separate path preserves an automatic workflow prompt in the queue.

## Requirement and design conformance

[Requirement 002](../../specs/tasks/requirements/prompt-attachments.md#req-tasks-prompt-attachments-002-initial-submission-recovery)
already owns the initial submission during fresh recovery.
Criteria 002.1, 002.3, 002.5, 002.6, and 002.7 remain applicable.
Draft criteria 002.9 through 002.11 make workflow composition and failure containment explicit.

The [owning design](../../specs/tasks/system-design/prompt-attachments.md#workflow-context-during-initial-recovery)
now defines the shared composition and the internal executor handoff.
Tasks owns this correction because it owns the pending submission and workflow instructions.
The provider runtime does not choose workflow policy.

Current trusted policy remains authoritative, as the existing recovery design requires.
An unchanged workflow therefore supplies the same constraints as the original launch.
Recovery does not replay an old `raw_content` block.
No new ADR is required for this implementation correction.

## Scope

### In scope

- Compose current workflow-level and step instructions for eligible initial fresh recovery.
- Cover pending receipts, including attachments, and the existing legacy text fallback.
- Preserve template order, replacement semantics, saved-prompt provenance, and effective plan mode.
- Preserve task/session identity, transcript rows, receipt transitions, readiness, and cancellation ownership.
- Stop before dispatch when required workflow policy is unavailable.
- Clarify the existing public recovery guidance during implementation.

### Out of scope

- The provider bootstrap cause in #5128 and delivery-checkpoint repair in #5129.
- Changes to automatic retry, Office scheduling, native resume, or history continuation.
- Replay of accepted messages, stored hidden blocks, one-time move instructions, or task plans.
- Receipt migrations, new persistence records, public request fields, runtime flags, and rendered UI changes.
- Production implementation, permanent tests, commits, PRs, and delegation during this design turn.

## Technical approach

Add a focused helper in `apps/backend/internal/orchestrator/initial_recovery_prompt.go`.
It resolves authorized task/session policy and current templates for the owned recovery attempt.
Strict policy reads precede token reset. Empty or excluded inputs retain their current eligibility.
Capture prior provider identity and receipt state before reset to distinguish initial fallback from recovery after accepted work.
An accepted receipt or existing provider-conversation evidence prevents this new initial-workflow override.

Reuse the existing workflow composer instead of duplicating template rules.
Carry its generated reference context through canonical first-launch wrapping.
Keep the effective plan mode from the request or the current step's `enable_plan_mode` action, apply the mode transform once, and leave the raw receipt value unchanged.
Apply configuration and plan-mode transforms once, after raw workflow composition.
Capture the workflow entry binding with the same policy snapshot and carry it through resume and prompt admission.
A move or leave-and-return supersedes the prepared instructions before dispatch.
Do not call workflow actions to recover instructions.

Receipt replay uses the prepared effective content through `initial_submission_recovery.go` and the first-launch seam in `task_operations.go`.
The raw receipt still owns transcript matching and acceptance transitions.
The executor starts without a prompt, and the owned continuation dispatches after readiness.

Legacy fallback uses an internal optional `InitialPromptOverride` on `LaunchSessionRequest` and `executor.ResumeOptions`.
`RecoverSessionWithOptions` supplies it only for the affected bootstrap recovery.
`newResumeLaunchRequest` applies it before existing prompt-suppression rules.
No public JSON caller can supply this value.

An automatic workflow queue entry already owns composed input.
The correction must not compose another copy of that entry or bypass a paused queue.
Unrelated queued input cannot authorize replay of stored history.
The existing initial-submission hold remains authoritative for receipt replay.

### Compatibility matrix

| Path | Input and identity | Result | Evidence |
| --- | --- | --- | --- |
| Pending receipt, structured ACP | Same task/session, fresh provider context | Workflow instructions plus the admitted submission and descriptors | Real recovery service and captured provider prompt |
| Legacy text fallback | Existing task-description fallback, same session | Normal first-launch workflow composition | Captured launch request and unchanged transcript |
| Automatic workflow recovery queue | Existing queue entry and incarnation | Existing input and pause policy, with one template application | Existing asynchronous recovery regression and negative fixture |
| Ordinary resume or read-only restore | Existing native or workspace identity | Existing prompt eligibility and suppression | Negative service and executor regressions |
| Passthrough, configuration, or ephemeral session | Existing mode contract | Existing visible-text and hidden-context exclusions | Mode fixtures with existing provider capabilities |
| Office session | Scheduler-owned recovery | Existing scheduler admission | Existing Office recovery regression |

Shared ACP composition does not establish live compatibility with every provider.
The reported Claude launch uses the shared path, which the controllable manager exercises without provider authentication.

## Tests

New service suite: `apps/backend/internal/orchestrator/fresh_start_workflow_context_test.go`.
Primary regression: `TestFreshStartWorkflowContext`.

| Criteria | Coverage |
| --- | --- |
| 002.1, 002.3, 002.9 | Receipt and legacy recovery, workflow-level instructions, step templates, order, and modes |
| 002.9, 002.10 | Saved `@prompt` expansion, current definition after an edit, and one effective instruction block |
| 002.5, 002.6, 002.10 | Retained original row and raw receipt, including cancelled and superseded attempts |
| 002.7 | Ordinary resume, workspace-only restoration, existing queue ownership, and Office exclusion |
| 002.11 | Missing step and failed step/workflow reads before reset or dispatch |

The executor adds `TestBuildResumeRequestWithOptions_FreshStartPromptOverride`.
It proves the optional handoff and retention of promptless suppression.
The work order defines the full matrix and exact commands.

## End-to-end evidence

The primary service test uses the real recovery action, persisted state, executor request construction, and an observable readiness event.
It captures the first agent input and reloads the stored message and receipt.
This covers the reported user outcome without an artificial browser test.
Desktop and phone use that same backend action. No interface composition changes.

## Work orders

- [x] [Task 01: Preserve workflow context in initial fresh recovery](task-01-preserve-workflow-context.md)

Task 01 is sequential and has no implementation dependency.
The existing fresh-start and asynchronous prompt packages are implemented in this snapshot.
Their recorded completion results remain unchanged.

## Verification results

Investigation on 2026-11-07:

- Disposable regression: failed in both recovery paths with the missing workflow constraint.
- Existing tool-policy and asynchronous-queue regressions: passed.
- GitHub assignment: `carlosflorencio`, verified from the issue response.
- Catalog validation: passed for 437 decisions and 1937 specifications.
- Specification linter tests: all 36 passed. Full specification lint passed.
- Documentation coverage preflight: passed with status `covered`, including the planned production path.
- Diff whitespace and new-file whitespace checks: passed.
- Temporary reproduction source: removed. No production or permanent test changes remain.

Implementation on 2026-11-08:

- Regression matrix: `TestFreshStartWorkflowContext` failed before the fix in both source paths and passes afterward.
- Focused race suite: passed for both orchestrator packages, including existing fresh-start, queue-preservation, Office, and resume-cancellation regressions.
- Full orchestrator and executor package suites: passed.
- Message queue package suite: passed.
- Backend build: `go build -trimpath ./...` passed.
- Executor handoff: `TestBuildResumeRequestWithOptions_FreshStartPromptOverride` passes for absent, nonempty, explicit-empty, promptless, continuation, and native-resume suppression.
- Go lint: passed for the changed backend files.
- Public recovery guidance: updated for current workflow and step instructions.
- Review follow-up: step-only plan mode, workflow-entry supersession, and one-shot provider/queue read failures are covered by the new regressions.
- PR fixup follow-up: saved-prompt read failures fail closed, unfenced routes stop recovery, legacy initial-prompt admission keeps the entry fence, and dynamic retries cache the raw receipt. A persisted-transcript regression covers row, turn, attachment, and failure retention.
- Specification, public documentation, link, and whitespace checks: passed.

Investigation commands:

```bash
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run '^TestRepro5137FreshStartWorkflowPrompt$' -count=1 -v)
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run '^Test(FreshStartReplayUsesCurrentFirstLaunchInstructions|WorkflowAsyncStartFailure_PreservesPromptThroughRecovery)$' -count=1)
```

The first command records investigation evidence. Its temporary source is not part of this package.

## Risks

- Generic prompt transforms can duplicate plan instructions if workflow composition occurs too late.
- Canonicalization removes saved instructions unless the exact generated reference context accompanies the prompt.
- A replacing step template can discard submitted text by existing policy. Recovery must retain that policy.
- A preserved queue entry can duplicate workflow input if fallback also applies its template.
- A workflow edit during recovery requires current policy and existing attempt/entry fencing, rather than old hidden transcript content.
- The original startup error remains a separate defect and can recur after this repair.

## Public documentation

Internal docs are updated in this package.
The implementation extends the existing recovery bullet in `docs/public/sessions-and-review.md`.
It states that the replacement receives current workflow and step instructions and retains its acceptance, attachment, and history limits.
The page remains a how-to guide. No screenshot or navigation update is required.
