---
id: "01-template-provisioning"
title: "Repair template creation and bootstrap"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-IMPROVE-KANDEV-001
acceptance_criteria:
  - AC-WORKSPACES-IMPROVE-KANDEV-001.7
  - AC-WORKSPACES-IMPROVE-KANDEV-001.8
  - AC-WORKSPACES-IMPROVE-KANDEV-001.9
  - AC-WORKSPACES-IMPROVE-KANDEV-001.10
  - AC-WORKSPACES-IMPROVE-KANDEV-001.11
  - AC-WORKSPACES-IMPROVE-KANDEV-001.12
system_design:
  - ../../specs/workspaces/system-design/improve-kandev.md
---

# Task 01: Repair template creation and bootstrap

## Summary

Reuse the existing template creator under the existing authorization boundary.
Make bootstrap fill empty managed workflows and report setup errors.

## In scope

- In `workflow.Service.CreateStepsFromTemplate`, retain access and executor
  checks without repeating the controller's user-facing read-only restriction.
  Keep the shared `authorizeExecutorMutation` helper unchanged for other methods.
- Keep `Controller.CreateStepsFromTemplate` authorization and mutability checks.
- Add `task.Service.EnsureWorkflowTemplateSteps` using the existing
  `WorkflowStepCreator` and `workflowStepLister` capabilities.
  Authorize management and validate the stored bootstrap template binding.
  Reject synced workflows. Create steps only when the workflow has zero steps.
- Use this helper in `ensureWorkflow`/`findAndHealWorkflow` and publish repair
  inserts with `stepevents.NewPublisher`. Preserve the hidden-flag repair.
- Return template errors from `CreateWorkflow` and bootstrap. Do not publish a
  workflow success event or accept a fallback row after a provisioning error.
- Serialize the handler's workflow list/find/create/repair section with one mutex.
  Read the list after acquiring it. Leave cloning and GitHub calls outside.
- Add the focused regressions in the [test matrix](plan.md#tests), including
  production policy wiring in the bootstrap fixture.

## Out of scope

New repository methods, SQL, batch transactions, migrations, provisioning
interfaces, privileged flags, and reconciliation of populated partial workflows.
Task 02 owns list compatibility. There are no rendered UI changes in this task.

## Acceptance

1. Fresh bootstrap creates all four expected steps across the two workflows.
   Empty repair retains workflow IDs. Populated siblings keep their steps and
   settings. Repeated or overlapping bootstrap creates no duplicate steps.
2. Template and repair errors reach the caller and keep bootstrap from reporting
   readiness. A later request can repair an empty hidden bootstrap row left by
   failed setup. Failed visible workflow creation removes its new row.
3. Public managed/synced workflow mutations and foreign-scope requests remain
   denied. Normal template creation, template remapping, and executor checks work.

## Implementation sequence

1. Add `TestImproveKandevBootstrapTemplateSteps` with production policy callbacks.
2. Run it before production edits and require failure on the zero-step defect.
3. Add controller guard and task-service error tests from the matrix.
4. Apply the small service, helper, and bootstrap changes.
5. Run the commands below and record red/green results.

## Verification

Run from the repository root.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/workflow/service ./internal/workflow/controller ./internal/task/service ./internal/improvekandev ./internal/integration -run 'Test(CreateStepsFromTemplate|EnsureWorkflowMutable|CreateWorkflowTemplateInitialization|EnsureWorkflowTemplateSteps|ImproveKandevBootstrap)' -count=1)
mapfile -t repair_go_files < <(git ls-files --modified --others --exclude-standard -- 'apps/backend/*.go' 'apps/backend/**/*.go')
bash scripts/lint-go-changed "${repair_go_files[@]}"
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

No repository or PostgreSQL-specific test matrix is required because SQL does
not change. Use existing fixtures and channel-based overlap tests.

## Files likely touched

- `apps/backend/internal/workflow/service/service.go`
- `apps/backend/internal/workflow/controller/template_guard_test.go` (new)
- `apps/backend/internal/task/service/service_resources.go`
- `apps/backend/internal/task/service/service_workflow_template.go` (new)
- `apps/backend/internal/task/service/service_workflow_template_test.go` (new)
- `apps/backend/internal/improvekandev/handler.go`
- `apps/backend/internal/integration/improve_kandev_template_steps_test.go` (new)

## Dependencies

None. Execute before Task 02.

## Risks

- The template service method serves two contexts. Public writes must continue
  through the guarded controller. Internal repair must authorize management.
- Hold the bootstrap lock across both the fresh list and repair decisions.
- Preserve existing per-step persistence. Partial-write reconciliation is outside scope.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/workspaces/requirements/improve-kandev.md), criteria .7-.12.
- [Design](../../specs/workspaces/system-design/improve-kandev.md#template-initialization-and-recovery).
- Existing controller guard, template builder, step lister, and step-event publisher.
- `wireTaskWorkflowCrossReferences` policy wiring and the existing integration fixture.
- Backend guidance and `.agents/skills/tdd/references/backend-tests.md`.

## Results

The production-policy regression failed before the repair because bootstrap
returned workflows with zero steps. The template-initialization regression
also failed before the error-propagation fix because the template error was
swallowed. Both regressions pass with the implementation. Bootstrap scenarios
assert raw row counts before name-map checks; the overlap case holds the first
injected creator until a second request enters the router.

The focused race-enabled workflow, task-service, Improve Kandev, and integration
suite passed. `bash scripts/lint-go-changed` reported zero issues. Documentation
catalog validation and specification lint passed, and `git diff --check` passed.
The temporary investigation reproduction remains removed.

PR fixup moved the new template authorization tests into
`service_template_authorization_test.go`. Regression tests prove visible
workflow rows are removed when the template creator is missing or fails, while
hidden bootstrap rows remain available for retry. The read-only service test
documents the controller/service boundary; controller tests still reject
public read-only mutations. The red test failed on the visible orphan before
the cleanup change. The focused race suite passed:

```bash
go test -trimpath -tags fts5 -race ./internal/workflow/service ./internal/workflow/controller ./internal/task/service ./internal/improvekandev ./internal/integration -run 'Test(CreateStepsFromTemplate|EnsureWorkflowMutable|CreateWorkflowTemplateInitialization|EnsureWorkflowTemplateSteps|ImproveKandevBootstrap)' -count=1
```

`bash scripts/lint-go-changed` reported zero issues for the four changed Go
files. Documentation catalog validation and specification lint passed after
the review follow-up.
