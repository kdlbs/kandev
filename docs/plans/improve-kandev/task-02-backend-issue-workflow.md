---
id: "02-backend-issue-workflow"
title: "Backend issue workflow and bootstrap"
status: done
wave: 1
depends_on: ["01-harness-planning-gate"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/improve-kandev.md"
---

# Task 02: Backend issue workflow and bootstrap

## Acceptance

- `report-kandev-issue` is a hidden, single-step, auto-starting system workflow
  whose prompt gathers and confirms every required repository-template field
  before publishing an issue without changing code.
- Improve Kandev bootstrap idempotently returns stable `workflow_id` and
  `issue_workflow_id` values for distinct hidden workspace workflows.
- A real-handler integration test proves the response and persisted workflow
  state across two bootstrap calls.

## Verification

```bash
cd apps/backend && go test ./config/workflows -run 'TestLoadTemplates_(ExpectedTemplateIDs|HiddenFlag|ReportKandevIssuePromptContract)' -count=1
cd apps/backend && go test ./internal/integration -run TestImproveKandevBootstrapCreatesBothHiddenWorkflowsIdempotently -count=1
```

## Files likely touched

- `apps/backend/config/workflows/report-kandev-issue.yml`
- `apps/backend/config/workflows/loader_test.go`
- `apps/backend/internal/improvekandev/handler.go`
- `apps/backend/internal/integration/improve_kandev_test.go`

## Dependencies

Task 01.

## Parallelism

Sequential. The response contract is consumed by Tasks 03 and 04.

## Inputs

- Spec: **What**, **API surface**, **Failure modes**, and issue-reporting
  scenarios.
- Plan: **Backend** and **Continuation Snapshot**.
- Pattern: existing `improve-kandev.yml`, `ensureWorkflow`, and integration test
  repository setup in `apps/backend/internal/integration/test_server_test.go`.

## Existing partial work to resume

- The new YAML template, generalized bootstrap workflow creation, response
  field, and loader contract tests are already present.
- The targeted workflow/config and improvekandev package tests passed.
- The missing real-handler integration assertion was added in the integration
  package so it can reuse the existing SQLite test server and workflow service.
  It covers two bootstrap calls, stable IDs, hidden persisted workflows, and
  the one-step issue workflow.

## Risks

- Test setup must use real task and workflow repositories on the same SQLite
  database so template steps are materialized exactly as production does.
- Preserve existing EMU/fork metadata; frontend decides which report kinds it
  blocks.

## Output contract

The bootstrap response contains stable, distinct implementation and issue
workflow IDs; both persisted workflows are hidden and the issue workflow has a
single auto-starting step.

## Recorded verification

- `cd apps/backend && go test ./config/workflows -run 'TestLoadTemplates_(ExpectedTemplateIDs|HiddenFlag|ReportKandevIssuePromptContract)' -count=1` — passed (3 tests).
- `cd apps/backend && go test ./internal/improvekandev ./config/workflows -count=1` — passed (43 tests).
- `cd apps/backend && go test ./internal/integration -run TestImproveKandevBootstrapCreatesBothHiddenWorkflowsIdempotently -count=1` — passed.

Task 02 is complete. Continue with Task 03.
