---
created: 2026-11-16
status: completed
requirements:
  - REQ-WORKSPACES-IMPROVE-KANDEV-001
system_design:
  - ../../specs/workspaces/system-design/improve-kandev.md
legacy_specs: []
---

# Fix plan: Improve Kandev template steps

## Overview

[Issue #5328](https://github.com/kdlbs/kandev/issues/5328) has two failures.
Bootstrap creates workflow records but cannot fill them with template steps.
The client then crashes because the empty step list arrives as null.
Retry finds the existing records and never fills them.

The user requested a simple repair. Reuse the current template method and the
existing controller authorization boundary. Add empty-workflow repair and null
normalization. Do not introduce a provisioning subsystem or new repository API.

## Ownership and scope

The workspace-owned
[requirement](../../specs/workspaces/requirements/improve-kandev.md) and
[design](../../specs/workspaces/system-design/improve-kandev.md#template-initialization-and-recovery)
cover this outcome. Existing user permissions, workflow templates, and dialog
composition remain unchanged.

In scope: fresh setup, repair of zero-step managed workflows, error propagation,
and an array at the API/client boundary. A populated workflow stays untouched.
Partial-workflow reconciliation, new SQL/transactions, schema changes, new
permission flags, and credential or launch policy changes are outside this repair.

## Confirmed evidence

The defect was reproduced at `b4c6a197129b5b4a5e99c3e1dab132ada9fb433f`.
A temporary `TestRepro5328BootstrapWithProductionWorkflowPolicy` used the real
handler, task service, workflow service, and production mutation policy callbacks.

```bash
(cd apps/backend && GOMAXPROCS=2 go test -trimpath -tags fts5 -p 2 -run '^TestRepro5328BootstrapWithProductionWorkflowPolicy$' ./internal/integration -v -count=1)
```

Both requests returned HTTP 200. Both workflows retained zero steps and the same
IDs on retry. The controller serialized `{"steps":null}`. Logs showed the reported
read-only error. The temporary test and its own artifacts were removed.

The existing `TestImproveKandevBootstrapCreatesBothHiddenWorkflowsIdempotently`
passed because its fixture omits the production policy callbacks.
Permanent regression coverage must include those callbacks.
Issue assignment to `carlosflorencio` was verified on 2026-11-16.

## Technical approach

1. **Use the existing authorization boundary.**
   `Controller.CreateStepsFromTemplate` already authorizes the user and checks
   read-only policy. In the service's template method, retain access and executor
   checks without repeating the read-only check that blocks internal setup.
   Keep other service mutation methods and their guards unchanged.
2. **Repair empty managed workflows.**
   Add one task-service helper, `EnsureWorkflowTemplateSteps`, using the existing
   creator and lister capabilities. Authorize management, use the stored template,
   reject synced workflows, and create steps only when there are none.
   Use it from the existing bootstrap find/heal path and retain workflow IDs.
   Publish repair inserts through the existing step-event publisher.
3. **Stop reporting failed setup as success.**
   Return template errors from `CreateWorkflow` and bootstrap. Keep a hidden
   bootstrap workflow row available for retry; remove a newly created visible
   workflow if its template initialization fails. Do not let the existing
   fallback swallow these errors.
4. **Prevent overlapping bootstrap repairs.**
   Serialize only the handler's workflow list/find/create/repair section with one
   mutex. Read the list inside that section. Cloning and GitHub calls stay outside.
5. **Normalize empty lists.**
   Return `[]` from the steps endpoint and use `(response.steps ?? []).map(...)`
   in the client. Preserve populated-step normalization and real errors.

No new workflow-service entry point or `WorkflowStepCreator` interface is needed.
Reuse the current template validation, ID remapping, and step inserts.
There is no new database transaction contract or PostgreSQL-specific code.
The controller/task-service split preserves the rationale without a new ADR.

## ASCII UI preview

`UI-01: Bootstrap result`, existing desktop footer and phone Utilities entry.

```text
Before: Improve Kandev -> preparation error (null.map)
After:  Improve Kandev -> report kind -> title/description -> task actions
Error:  preparation error -> submission remains blocked
```

Desktop and phone keep their existing dialog composition, touch targets, focus,
and scroll owner. This is state recovery with no new copy or layout.
The labels are illustrative. Criteria .1, .11, and .13 apply.

## Tests

All criteria use `AC-WORKSPACES-IMPROVE-KANDEV-001`.
New names below are implementation targets.

| Criteria | Focused evidence |
| --- | --- |
| .7-.10 | `internal/integration/improve_kandev_template_steps_test.go`: `TestImproveKandevBootstrapTemplateSteps` covers fresh creation, empty repair, populated siblings, repeat calls, and overlapping bootstrap. Use production policy callbacks. |
| .11 | The same integration file covers provisioning errors and retry. `internal/task/service/service_workflow_template_test.go`: `TestCreateWorkflowTemplateInitialization` covers returned template errors and absence of a success event. |
| .12 | `internal/workflow/controller/template_guard_test.go`: `TestCreateStepsFromTemplateReadOnly` verifies that the public controller still blocks managed/synced workflows and foreign scope. Existing service tests cover executor denials and template remapping. |
| .13 | `internal/workflow/handlers/steps_list_test.go`: `TestListWorkflowStepsEmptyArray` asserts empty-array JSON and preserved error behavior. |
| .13 | `apps/web/lib/api/domains/workflow-api.test.ts`: null, omitted, empty, populated, and rejected-response cases for `listWorkflowSteps`. |

Use existing fixtures. Add focused test files rather than extending an oversized
service test file. A real-service test proves bootstrap correctness.
A mocked bootstrap cannot replace that evidence.

## E2E tests

Run the existing desktop `improve-kandev.spec.ts` and phone
`mobile-improve-kandev.spec.ts` suites with fresh managed builds.
Retain their create, reopen, and issue-only flows.
Shared data normalization changes no viewport-dependent behavior, so unit tests
and these existing rendered flows satisfy mobile parity.
No new mobile interaction or separate browser fixture is required.

## Work orders

- [x] [Task 01: Repair template creation and bootstrap](task-01-template-provisioning.md)
- [x] [Task 02: Normalize workflow step lists](task-02-step-list-compatibility.md)

Both work orders are complete in the primary session.

## Verification results

The original investigation regression failed as expected. The old integration
regression passed, confirming the fixture gap. Issue assignment to
`carlosflorencio` is verified.

Task 01's permanent production-policy and retry regressions pass under the race
detector, including raw-row count and sequenced-overlap assertions. Task 02's
HTTP and client compatibility regressions also pass under the race detector
where applicable. Changed Go files pass
`bash scripts/lint-go-changed` with zero issues. The focused web suite passes
22 tests; typecheck and lint pass. Desktop E2E passes 17 tests and phone E2E
passes one test. Both E2E runs complete the backend and Vite builds.
Documentation catalog validation, full specification lint, documentation
coverage, and `git diff --check` pass.

### Documentation preflight

Run this command from the repository root. It calls the repository's coverage
validator against the implementation paths and package documents. This proves
reference coverage; implementation correctness is established by the tests
above.

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const dir = 'docs/plans/improve-kandev-template-steps';
const docs = [
  ...fs.readdirSync(dir).filter(name => name.endsWith('.md')).map(name => `${dir}/${name}`),
  'docs/specs/workspaces/requirements/improve-kandev.md',
  'docs/specs/workspaces/system-design/improve-kandev.md',
];
const production = [
  'apps/backend/internal/task/service/service_resources.go',
  'apps/backend/internal/workflow/controller/controller.go',
  'apps/web/lib/api/domains/workflow-api.ts',
];
const result = validateCoverage({
  changedFiles: [...production, ...docs].map(filename => ({ filename, status: 'modified' })),
  fileContents: Object.fromEntries(docs.map(path => [path, fs.readFileSync(path, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
```

## Documentation impact

This turn updates internal requirements, design, and delivery records only.
The fix restores existing setup and dialog behavior. No public command, setting,
label, screenshot, or user procedure changes, so no public-doc edit is planned.

## Risks

- Keep the public controller's read-only and management checks in place.
  Internal repair must authorize scope and retain executor eligibility.
- The bootstrap mutex must cover the initial list read and both workflow repairs.
  Otherwise concurrent requests can act on the same stale empty state.
- Existing partial workflows stay untouched. This repair keeps current step-write
  semantics and reports errors without introducing batch rollback.
- A hidden bootstrap workflow stays available for retry after template failure.
  Failed visible template creations are deleted to avoid leaving a workflow the
  caller was told had failed.
- Returning template errors exposes failures that ordinary workflow creation
  previously logged. Cover a normal successful creation and its error path.
