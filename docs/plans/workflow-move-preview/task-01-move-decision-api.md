---
id: "01-move-decision-api"
title: "Read-only move decisions and API"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-MOVE-PREVIEW-001
  - REQ-TASKS-WORKFLOW-MOVE-PREVIEW-002
acceptance_criteria:
  - AC-TASKS-WORKFLOW-MOVE-PREVIEW-001.1
  - AC-TASKS-WORKFLOW-MOVE-PREVIEW-001.2
  - AC-TASKS-WORKFLOW-MOVE-PREVIEW-001.3
  - AC-TASKS-WORKFLOW-MOVE-PREVIEW-001.4
  - AC-TASKS-WORKFLOW-MOVE-PREVIEW-001.5
  - AC-TASKS-WORKFLOW-MOVE-PREVIEW-001.6
  - AC-TASKS-WORKFLOW-MOVE-PREVIEW-001.7
system_design:
  - ../../specs/tasks/system-design/workflow-move-preview.md
---

# Task 01: Read-only move decisions and API

## Summary

Provide a task-specific preview of recipient and expected configuration without
executing routing or ACP operations. Reuse decisions in execution paths so the
endpoint and actual moves remain consistent.

## In scope

Shared recipient decision extraction, effective configuration and conditional
rule projection, normalized one-shot reset/dispatch effects, DTO, authorized
HTTP endpoint, dependency wiring, and focused backend regression fixtures.

## Out of scope

Rendered UI, schema migrations, changed lifecycle policy, provider probes,
and caching or reserving preview selections.

## Acceptance

1. Real fixture moves match their previews for same/other/new session cases,
   explicit targets, overrides, and conditional settings; terminal exclusion holds.
2. Repeated previews leave session/task metadata, bindings, messages, and runtime
   invocation counters unchanged. Authorization and safe DTO filtering hold.
3. Unknown/skipped/planned configuration is explicit; existing atomic route,
   retry, deferred-move, and best-effort provider semantics remain intact.

## Verification

Run from repository root. Add named test files before running these commands;
use TDD to demonstrate the missing preview behavior first.

```bash
(cd apps/backend && go test ./internal/orchestrator ./internal/task/handlers ./internal/task/service ./internal/workflow/...)
(cd apps/backend && go test -race ./internal/orchestrator ./internal/task/handlers -run 'Preview|WorkflowStepSession|WorkflowSessionConfig|WorkflowSessionTarget|SameProfile')
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/orchestrator/workflow_move_preview.go`,
  `workflow_move_preview_settings.go`, and `_test.go` (new).
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`.
- `apps/backend/internal/orchestrator/workflow_session_target.go`.
- `apps/backend/internal/orchestrator/workflow_session_config.go`.
- `apps/backend/internal/task/handlers/workflow_move_preview_http.go` and `_test.go` (new).
- `apps/backend/internal/task/handlers/task_handlers.go`, `task_http_handlers.go`.
- `apps/backend/internal/task/dto/` and existing backendapp dependency wiring.

## Dependencies

None. Read both paired specifications and current lifecycle/conditional-settings
contracts. Existing same-profile policy and session-target tests are exemplars.

## Risks

A helper named resolve can mutate state. Review every reachable call; extract
pure inputs/outputs and keep warning publication in execution only.

## Parallelism

`sequential`

## Inputs

[Design](../../specs/tasks/system-design/workflow-move-preview.md), especially
Resolution, API, and Freshness and failures. [Plan test matrix](plan.md#tests).

## Results

Implemented the read-only workflow move decision layer and the authorized
`POST /api/v1/tasks/:id/move-preview` endpoint. Recipient selection shares the
pure reusable-session selector with execution, preserves explicit targets and
terminal-session exclusion, projects runtime configuration and conditional
rules without writes, and reports unknown or deferred outcomes explicitly.

Verification passed:

- `cd apps/backend && go test ./internal/orchestrator ./internal/task/handlers ./internal/task/service ./internal/workflow/...`
- `cd apps/backend && go test -race ./internal/orchestrator ./internal/task/handlers -run 'Preview|WorkflowStepSession|WorkflowSessionConfig|WorkflowSessionTarget|SameProfile'`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`

## Review remediation results

The preview now shares `workflowmove.ShouldAutoStartAgent` with the actual
no-session launch handler. A destination without `auto_start_agent`, or a
`skip_step_prompt` move without instructions, returns `no_session` with no
recipient. A permitted sessionless launch still resolves the task-level agent
profile when the destination has no step profile. Provider option labels are
bounded display labels, while Kandev-owned setting fields keep stable keys for
localization. The endpoint authorizes the task before repository access, keeps
current-session routing when only the task profile fallback exists, predicts
profile switches from an existing source session, and projects applicable
configure-session rules for a fresh launch. Busy reusable targets report
deferred dispatch, while fresh context and passthrough mode changes report
skipped applicability.

Regression coverage compares both idle launch gates with the actual move path,
checks task-profile fallback for an allowed launch, and verifies the setting
label DTO shape, task authorization ordering, typed configuration notices, and
busy-target dispatch.

Remediation verification passed on 2026-09-15:

- `cd apps/backend && go test ./internal/workflow/move ./internal/orchestrator`
- `cd apps/backend && make lint`

Final post-remediation verification also passed for the targeted backend race
suite and the managed backend build:

- `cd apps/backend && go test -race ./internal/orchestrator ./internal/task/handlers -run 'Preview|WorkflowStepSession|WorkflowSessionConfig|WorkflowSessionTarget|SameProfile'`
- `cd apps/backend && go test -race ./internal/orchestrator -count=1`
- `cd apps/backend && go test ./internal/orchestrator -run '^TestCompletedTaskFollowUpAdmissionIsConversationalOnly$' -count=100 -failfast`
- `cd apps/backend && make build`
