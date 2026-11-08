---
id: "01-preserve-workflow-context"
title: "Preserve workflow context in initial fresh recovery"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-PROMPT-ATTACHMENTS-002
acceptance_criteria:
  - AC-TASKS-PROMPT-ATTACHMENTS-002.1
  - AC-TASKS-PROMPT-ATTACHMENTS-002.3
  - AC-TASKS-PROMPT-ATTACHMENTS-002.5
  - AC-TASKS-PROMPT-ATTACHMENTS-002.6
  - AC-TASKS-PROMPT-ATTACHMENTS-002.7
  - AC-TASKS-PROMPT-ATTACHMENTS-002.9
  - AC-TASKS-PROMPT-ATTACHMENTS-002.10
  - AC-TASKS-PROMPT-ATTACHMENTS-002.11
system_design:
  - ../../specs/tasks/system-design/prompt-attachments.md
---

# Task 01: Preserve workflow context in initial fresh recovery

## Summary

Repair the two recovery paths identified in [issue #5137](https://github.com/kdlbs/kandev/issues/5137).
Reuse first-launch workflow composition for initial receipt replay and eligible legacy text fallback.
Keep dispatch ownership, raw submission data, and existing recovery exclusions intact.

## In scope

- Add a focused recovery composer with strict task, step, and workflow policy reads.
- Carry trusted saved-prompt context through canonical first-launch wrapping.
- Compose before configuration and plan-mode transforms, with one copy of each instruction block.
- Add an internal optional prompt override for legacy fallback, subject to existing executor suppression.
- Cover the full recovery action with persisted state and a controllable provider boundary.
- Clarify public recovery guidance after the implementation passes its checks.
- Record actual results here and in the plan.

## Out of scope

- Startup error classification, provider installation, delivery-journal repair, and new retry policy.
- Workflow actions, session creation, task movement, and Office admission changes.
- Raw-history replay, receipt migration, new public fields, frontend code, and rendered UI changes.

## Acceptance

1. Both source paths deliver the applicable workflow instructions once under normal first-launch rules.
   Required policy-read failures prevent token reset and provider delivery.
2. Receipt state, attachments, original transcript rows, readiness, cancellation, and queue ownership retain their existing behavior.
   Excluded recovery paths gain no new prompt.
3. The regression matrix and exact checks pass. Public guidance and delivery records describe the resulting behavior.

## Implementation notes

Capture the current authorized task/session policy in `RecoverSessionWithOptions` before `applyRecoveryActionWithSettlement`.
Use the raw pending submission where available, or the description already eligible for legacy fallback.
Do not infer a new replay source from `raw_content`, the first message, or session expansion metadata.
Read provider-conversation evidence before reset and preserve existing behavior for accepted or uncertain submissions.
An already cleared token is not evidence that a session never accepted work.

Reuse `buildWorkflowPromptWithContextOptions` with a validated step and workflow snapshot.
Preserve empty-step, `{{task_prompt}}`, replacing-template, and workflow-level instruction behavior.
Carry the generated reference context with its composed content.
Keep existing first-launch title, question, completion-signal, autopilot, and built-in package policy.

For receipts, compose raw workflow content before `effectivePromptForSessionWithConfigMode`.
The existing first-launch wrapping seam still creates the canonical MCP context.
Keep `submission.Content` separate from dispatch content so receipt comparison and transcript matching remain valid.
Do not apply workflow entry actions or repeat saved-prompt expansion after preparation.

For legacy fallback, transport the effective prompt with the proposed optional `InitialPromptOverride`.
Keep the launch request field internal with `json:"-"`.
The executor consumes prepared content and performs no workflow lookups.
Existing `NoInitialPrompt` and resume suppression take precedence over the override.
An absent override retains current request construction.

A preserved automatic workflow queue entry already owns its template.
Do not add a second composition or drain it through a new recovery route.
Retain the receipt hold ahead of unrelated queued work.
Use existing resume-attempt and workflow-entry guards at dispatch.

## Regression matrix

Create `fresh_start_workflow_context_test.go` with the table suite `TestFreshStartWorkflowContext`.

The smallest fixture contains a workflow-bound task, a FAILED session, a ready task environment, and a current bootstrap error.
Persist the original first user row and its turn.
Use an actual pending submission receipt for one source and no receipt/preview for legacy text fallback.
Keep the task description nonempty so fallback cannot hide the defect.
Call `RecoverSession(..., "fresh_start")` and observe the agent launch or post-readiness prompt.
Use channels or observable state for readiness and completion.

Cover these cases:

- The reported constraint appears before the task input in both source paths.
- A real saved prompt referenced by the step produces trusted hidden expansion in the first agent input.
- A current saved definition replaces an older stored expansion without copying the old hidden block.
- Workflow-level instructions, `{{task_prompt}}`, replacing templates, and empty step prompts retain their first-launch behavior.
- Plan-mode and configuration transforms appear once, including a step that enables plan mode.
- Raw receipt content remains unchanged in meaning, apart from its existing delivery-state transition.
- Attachment descriptors retain order and delivery modes. The transcript retains its original row and failure history.
- A missing step or failed step/workflow read leaves provider identity and pending input intact, with no provider dispatch.
- Cancelled and superseded recovery attempts cannot send into a successor.
- A preserved automatic workflow entry is not composed twice, including while the queue is paused.
- Ordinary resume, read-only restore, empty fallback, ephemeral/configuration exclusions, and Office recovery retain their existing prompt policy.
- An accepted receipt or prior provider-conversation identity prevents the new initial-workflow fallback, including after token reset.
- Passthrough retains its visible-text policy and receives no newly introduced hidden expansion.

Add `TestBuildResumeRequestWithOptions_FreshStartPromptOverride` in the executor suite.
Cover absent, nonempty, and explicit-empty overrides, plus `NoInitialPrompt` and native-resume suppression.
Keep existing completed-session and cancelled-session suppression regressions in the targeted check.

## Verification

Run the first command after the regression exists and before production changes.
Record the expected missing-constraint failure for both source paths.

```bash
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run '^TestFreshStartWorkflowContext$' -count=1 -v)
```

After implementation, run this block from the repository root:

```bash
(cd apps/backend && go test -race -tags fts5 ./internal/orchestrator ./internal/orchestrator/executor -run '^Test(FreshStartWorkflowContext|FreshStartSubmission|FreshStartReplay|FreshStartPendingReceipt|FreshStartRecovery|WorkflowAsyncStartFailure|RecoverSession_Office|ApplyRunningRecordToResumeRequest|BuildResumeRequestWithOptions|ResumeSession_ArchiveCancelled|ResumeSession_OrphanCancelled|ResumeSession_UserCancelled)' -count=1)
bash scripts/lint-go-changed apps/backend/internal/orchestrator/initial_recovery_prompt.go apps/backend/internal/orchestrator/initial_submission_recovery.go apps/backend/internal/orchestrator/messagequeue/service.go apps/backend/internal/orchestrator/session_launch.go apps/backend/internal/orchestrator/task_operations.go apps/backend/internal/orchestrator/executor/executor_resume.go apps/backend/internal/orchestrator/fresh_start_workflow_context_test.go apps/backend/internal/orchestrator/fresh_start_workflow_fixture_test.go apps/backend/internal/orchestrator/executor/executor_resume_fresh_start_override_test.go apps/backend/internal/prompts/service/expansion.go
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/fresh-start-workflow-context
```

Run this documentation coverage preflight from the repository root:

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const plan = 'docs/plans/fresh-start-workflow-context/plan.md';
const work = 'docs/plans/fresh-start-workflow-context/task-01-preserve-workflow-context.md';
const design = 'docs/specs/tasks/system-design/prompt-attachments.md';
const requirement = 'docs/specs/tasks/requirements/prompt-attachments.md';
const source = 'apps/backend/internal/orchestrator/session_launch.go';
const paths = [plan, work, design, requirement, source];
const result = validateCoverage({
  changedFiles: [
    { filename: plan, status: 'added' },
    { filename: work, status: 'added' },
    { filename: design, status: 'modified' },
    { filename: requirement, status: 'modified' },
    { filename: source, status: 'modified' },
  ],
  fileContents: Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(JSON.stringify({ ok: result.ok, status: result.status, errors: result.errors }, null, 2));
if (!result.ok) process.exit(1);
NODE
```

This preflight includes a planned production path to exercise the full reference chain.
There is no new SQL query or schema change, so this package requires no new database-dialect suite.
The real backend recovery action supplies end-to-end evidence for the shared desktop and phone controls.

## Files likely touched

- `apps/backend/internal/orchestrator/initial_recovery_prompt.go` (new)
- `apps/backend/internal/orchestrator/initial_submission_recovery.go`
- `apps/backend/internal/orchestrator/messagequeue/service.go`
- `apps/backend/internal/orchestrator/session_launch.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/fresh_start_workflow_context_test.go` (new)
- `apps/backend/internal/orchestrator/fresh_start_workflow_fixture_test.go` (new test support)
- `apps/backend/internal/orchestrator/executor/executor_resume_fresh_start_override_test.go` (new)
- `docs/public/sessions-and-review.md`
- This work order and its sibling plan for execution status and results.

## Dependencies

None. Read the completed companion packages listed in the plan before changing their shared seams.
Execution is sequential. No subagents are authorized.

## Risks

- Late composition can duplicate plan-mode instructions or erase saved-prompt provenance.
- A transcript row proves storage, without proving provider acceptance.
- A broad override can accidentally prompt an ordinary resume or a promptless recovery.
- Queue ownership and cancellation can change while the provider starts.
- Template replacement rules can intentionally omit the raw task description.

## Parallelism

`sequential`

## Inputs

- [Plan](plan.md), including reproduction evidence and the compatibility matrix.
- [Requirement 002](../../specs/tasks/requirements/prompt-attachments.md#req-tasks-prompt-attachments-002-initial-submission-recovery).
- [Workflow recovery design](../../specs/tasks/system-design/prompt-attachments.md#workflow-context-during-initial-recovery).
- [Fresh-start recovery](../fresh-start-recovery/plan.md), especially its completed policy and admission follow-up.
- [Automatic workflow preservation](../workflow-async-start-prompt-preservation/plan.md).
- Existing `fresh_start_submission_test.go`, `workflow_start_prompt_test.go`, `workflow_entry_saved_prompt_test.go`, and executor resume regressions.
- `apps/backend/AGENTS.md` and `.agents/skills/tdd/references/backend-tests.md`.

## Results

Implemented after the explicit implementation request. The new regression failed
before the fix in both source paths with the missing workflow constraint. It now
passes with current policy reads, one composition per path, and receipt content
left raw. The focused race suite, full orchestrator and executor package suites,
and Go lint pass. Public recovery guidance now describes the workflow and step
instructions. The investigation's temporary fixture is not part of this package.

Review follow-up corrected three gaps. The prepared recovery now carries the
effective plan mode from the request and the current step's `enable_plan_mode`
action, applies the existing transform once for receipt and legacy recovery, and
leaves the raw receipt value unchanged. It also captures the workflow entry
binding from the same policy snapshot and carries it through resume and prompt
admission, so a move or leave-and-return supersedes the prepared instructions
before dispatch. Provider-identity and preserved-queue read failures now stop
recovery before reset instead of silently dropping composition. The added
regressions cover step-only plan mode, leave-and-return supersession, and
one-shot identity or queue read failures.

PR fixup closed the remaining review findings. Saved-prompt resolution now uses
an error-returning expansion path and stops both receipt and legacy recovery
before reset. A workflow step that cannot produce a valid entry binding fails
closed instead of running unfenced, and the legacy launch carries the entry
binding into initial-prompt admission while holding the dispatch fence through
acceptance or failure. Dynamic fallback retries now cache the raw receipt rather
than the composed prompt so workflow instructions are not applied twice. A
persisted-transcript regression covers the original user row, turn, attachment,
and failure history without adding another user message.
