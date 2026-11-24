---
id: "01-compose-current-step"
title: "Compose the current workflow step on ordinary starts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-007
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-007.1
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-007.2
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-007.3
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-007.4
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-007.5
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-007.6
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-007.7
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-007.8
system_design:
  - ../../specs/tasks/system-design/workflow-task-start-context.md
---

# Task 01: Compose the current workflow step on ordinary starts

## Summary

Ordinary omitted-ID starts must use the resolved current step for first-prompt composition.
One work order owns the source correction, regression coverage, and API reference clarification.

## Scope

In scope: all cases in the [plan test matrix](plan.md#tests), including stored/dispatched content, profile selection, and configuration before launch.
Out of scope: task movement, workflow triggers, admission, replay payloads, recovery, provider transport, UI, and historical transcript repair.
Office/helper prompt contracts and explicit `start_created` input preservation remain separate.

## Acceptance

1. The omitted-ID regression fails before the correction, then passes with the explicit-current-step control and matching launch configuration.
2. All referenced criteria have behavior assertions, including explicit-step precedence, separate Office/helper negative controls, and the accepted degraded path when the fallback task read fails.
3. Exact checks pass, the API reference describes the resulting behavior, and both delivery records contain actual results.

## Implementation sequence

1. Read the linked requirement/design, scoped backend guidance, and backend test reference.
2. Reconcile requirement IDs and caller contracts with the current base.
3. Mark this work order `in_progress` and update the plan.
4. Write `TestStartTask_CurrentStepPrompt` before the source change.
5. Run the first verification command and retain the expected missing-sentinel failure.
6. Add a local prompt-step value at the existing `applyWorkflowAndPlanMode` call in `startTask`.
7. Retain the incoming step for Office and `PreserveWorkflowSourceBinding` callers. Use the existing resolved fallback for other omitted-ID starts.
8. Preserve incoming step uses elsewhere and explicit profile precedence.
9. Complete the test matrix and API reference update.
10. Run every verification command and record results before marking the work order done.

## Fixture guidance

Reuse `setupTestRepo`, `newMockStepGetter`, `newMockTaskRepo`, `mockAgentManager`, and `createTestServiceWithScheduler`.
Use `configureSessionStep` for a matching original-session model/option rule.
Create an existing task bound to a review step with `OnEnterAutoStartAgent` and a distinctive template containing `{{task_prompt}}`.
Keep the description distinct from the step sentinel.
In `launchAgentFunc`, capture `TaskDescription`, `AgentProfileID`, and persisted `SessionRuntimeConfig` overrides before launch returns.
Use `mockMessageCreator` to compare stored and dispatched instructions.

For helper coverage, set `ProfileExplicit` and `PreserveWorkflowSourceBinding` and use a distinct direct prompt.
For Office coverage, use the Office task marker and `validOfficeRuntimeEnv`, with `KANDEV_TASK_ID` matching the fixture task.
Conditional configuration still requires the original session. A replacement session must not gain configuration solely for this test.
Use distinct templates for stored and explicitly selected steps.
Assert content sentinels and context-block counts without snapshots of the full MCP tool list.

## Verification

Run from the repository root. The first command is the required red/green regression command.

```bash
(cd apps/backend && go test -trimpath ./internal/orchestrator -run '^TestStartTask_CurrentStepPrompt' -count=1 -v)
(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run '^TestStartTask_CurrentStepPrompt' -count=1)
(cd apps/backend && go test -trimpath ./internal/orchestrator -run '^(TestApplyWorkflowAndPlanMode_|TestLaunchSession_ProfileExplicit|TestStartTask_PreservesOnlyResolvedWorkflowPromptExpansion|TestLaunchSpawnPreservesWorkflowSourceBinding|TestStartTask_Office)' -count=1)
bash scripts/lint-go-changed apps/backend/internal/orchestrator/task_operations.go apps/backend/internal/orchestrator/task_start_workflow_prompt_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Run this PR-documentation coverage preflight from the repository root:

```bash
node <<'JS'
const fs = require('node:fs');
const cp = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'apps/backend/internal/orchestrator/task_operations.go',
  'docs/specs/tasks/requirements/workflow-step-agent-start-ownership.md',
  'docs/specs/tasks/system-design/workflow-task-start-context.md',
  'docs/plans/task-start-current-step-prompt/plan.md',
  'docs/plans/task-start-current-step-prompt/task-01-compose-current-step.md',
];
const tracked = cp.execFileSync('git', ['diff', '--name-only', '-z', 'HEAD'], {encoding: 'utf8'});
const untracked = cp.execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], {encoding: 'utf8'});
const changed = [...new Set((tracked + untracked).split('\0').filter(Boolean))];
const fileContents = Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')]));
const result = validateCoverage({
  changedFiles: changed.map(filename => ({filename, status: 'modified'})),
  fileContents,
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
JS
```

## Files likely touched

- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/task_start_workflow_prompt_test.go` (new)
- `docs/public/websocket-api.md`
- This work order and `plan.md` (status/results).

## Dependencies and parallelism

None. `sequential`. No subagent delegation is authorized.

## Risks

- Unconditional fallback can replace Office/helper instructions with a workflow template.
- Skip overrides suppress the description fallback as well as the step template.
- The existing task operations test file exceeds the effective-line budget. New coverage belongs in the separate file named here.

## Inputs

- [Requirement 007](../../specs/tasks/requirements/workflow-step-agent-start-ownership.md#req-tasks-workflow-step-agent-start-ownership-007-current-step-context-on-ordinary-starts)
- [Step identity and caller compatibility](../../specs/tasks/system-design/workflow-task-start-context.md)
- `TestStartTask_PreservesOnlyResolvedWorkflowPromptExpansion` in `task_operations_test.go`.
- `TestApplyWorkflowSessionConfigBeforeLaunchPersistsRuntimeLayer` in `event_handlers_workflow_session_config_test.go`.
- `session_launch_profile_explicit_test.go` and `session_launch_profile_explicit_preflight_test.go`.
- `workflow_prompt_test.go` and `prompt_launch_fallback_test.go`.

## Results

Completed. The permanent 14-case `TestStartTask_CurrentStepPrompt` regression reproduced the omitted-ID omission before the production change and passes after a prompt-only fallback to the resolved current step. The stored task step remains unchanged when the caller omits an ID. Explicit steps, explicit profiles, skip overrides, template semantics, plan mode, saved references, and Office/helper/ephemeral/unbound controls pass. A separate lookup-failure regression records the accepted degraded behavior for a later failed task read: log the failure and continue with the caller prompt without step-template composition.

Verification passed:

- `(cd apps/backend && go test -trimpath ./internal/orchestrator -run '^TestStartTask_CurrentStepPrompt' -count=1 -v)`
- `(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run '^TestStartTask_CurrentStepPrompt' -count=1)`
- `(cd apps/backend && go test -trimpath ./internal/orchestrator -run '^(TestApplyWorkflowAndPlanMode_|TestLaunchSession_ProfileExplicit|TestStartTask_PreservesOnlyResolvedWorkflowPromptExpansion|TestLaunchSpawnPreservesWorkflowSourceBinding|TestStartTask_Office)' -count=1)`
- `bash scripts/lint-go-changed apps/backend/internal/orchestrator/task_operations.go apps/backend/internal/orchestrator/task_start_workflow_prompt_test.go`
- `make -C apps/backend build`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.test.py`
- `python3 scripts/lint-spec-files.py --all`
- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`
- PR-docs coverage preflight and `git diff --check`

The PR review follow-up also verified criterion `007.8`: when the later omitted-step task read fails after classification succeeds, the start logs the warning and preserves the direct prompt in both stored and dispatched content without step-template text.

The public API reference now describes `workflow_step_id` fallback for ordinary starts.
