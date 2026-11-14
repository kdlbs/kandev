---
id: "01-classify-step-mutation-errors"
title: "Classify step mutation errors by identity"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001.1
  - AC-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001.2
  - AC-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001.3
  - AC-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001.4
  - AC-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001.5
  - AC-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001.6
system_design:
  - ../../specs/tasks/system-design/workflow-step-mutation-errors.md
---

# Task 01: Classify step mutation errors by identity

## Summary

Correct the shared workflow HTTP mapper so unrelated missing dependencies cannot masquerade as missing steps.
Preserve recognized policy responses and distinguish missing templates, workflows, workspaces, and executor profiles.
Deliver the error-source change, mapper correction, and tests together.

## In scope

- Add `models.ErrWorkflowTemplateNotFound` and preserve it through `Repository.GetTemplate` wrapping.
- Replace the mapper's arbitrary `not found` text match with the explicit identities in the design table.
- Preserve all earlier policy and validation branches, the existing JSON envelopes, and safe unknown-error response.
- Correct the stale `ErrNotVisible` comment without changing access normalization.
- Cover all HTTP callers, no-write rejection behavior, successful responses, and event publication boundaries.
- Correct the HTTP missing-template assertion in `TestCreateStepsFromTemplate` while preserving its WebSocket assertion.
- Preserve `models.ErrWorkflowStepNotFound` when an ordinary update or delete write affects zero rows after its earlier read.
- Cover repository update/delete misses and HTTP update/delete races that remove a step through `SetExecutorMutationChecker` after the handler reads it.

## Out of scope

- Executor eligibility, task policy producers, access decisions, and `service.IsNotFound` behavior.
- Other HTTP/read handlers, WebSocket/MCP classification, UI changes, and new error codes.
- Transaction redesign, schema changes, or broad cleanup.

## Acceptance

1. The classifier and route tests satisfy `.1`, `.2`, `.4`, and `.5`, including wrapped identities and misleading untyped text.
2. Typed template misses and resource-specific HTTP tests satisfy `.3` without masking genuine repository failures.
3. All five HTTP callers satisfy `.6`, and affected workflow package tests plus changed-file Go lint pass.

The criterion prefix is `AC-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001`.

## TDD sequence

1. Mark this work order `in_progress` and record that state in the plan.
2. Add `TestStepMutationDependencyFailureHTTP` using `setupStepRouter` and the real registered routes.
   Verify a preflight GET succeeds before testing update rejection.
   Inject dependency errors through `SetExecutorMutationChecker` for create, update, delete, and reorder.
   Use `SetWorkflowMutationReadOnlyChecker` for template application.
3. Assert that plain and wrapped non-step misses return the existing generic 500 response.
   Include mixed-case text and an untyped copy of `ErrWorkflowStepNotFound.Error()`.
   Run the red command and record the expected false-404 failures.
4. Add `TestStepMutationErrorClassification` for every row and precedence rule in the design table.
   Add `TestStepMutationResourceMissHTTP` for actual misses and typed workflow/workspace/profile policy errors.
5. Add `TestGetMissingWorkflowTemplateReturnsTypedNotFound` and `TestGetWorkflowTemplateStorageFailureIsNotNotFound` in a new repository test file.
   Reuse `setupTestRepoWithDB` and the existing missing-step sentinel test pattern.
6. Add repository tests for ordinary update/delete misses and HTTP update/delete race tests that delete the target through `SetExecutorMutationChecker` after the read.
   Assert the sanitized 404 and absence of successful mutation events.
7. Implement typed ordinary write-miss errors, leaving version-fenced update conflicts unchanged, then correct the existing HTTP missing-template expectation.
8. Run the green commands and documentation coverage preflight.
   Record actual results and promote the paired draft specs only after their criteria pass.
9. Mark this work order `done` and the plan `implemented` after all listed checks pass.

Dependency failures must occur before any write.
After each rejection, compare stored steps with their initial state and assert no successful mutation events.
Do not treat a template's unrelated partial-write behavior as part of this repair.

## Verification

Run from the repository root.
The red command must fail for the expected HTTP status mismatch before the correction:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/workflow/handlers \
  -run '^TestStepMutationDependencyFailureHTTP$' -count=1 -v)
```

After the correction, run:

```bash
(cd apps/backend && go test -trimpath -tags fts5 \
  ./internal/workflow/models ./internal/workflow/repository \
  ./internal/workflow/service ./internal/workflow/controller \
  ./internal/workflow/handlers -count=1)
bash scripts/lint-go-changed \
  apps/backend/internal/workflow/models/errors.go \
  apps/backend/internal/workflow/repository/sqlite.go \
  apps/backend/internal/workflow/repository/template_errors_test.go \
  apps/backend/internal/workflow/repository/step_errors_test.go \
  apps/backend/internal/workflow/service/access.go \
  apps/backend/internal/workflow/handlers/handlers.go \
  apps/backend/internal/workflow/handlers/step_mutation_errors_test.go \
  apps/backend/internal/workflow/handlers/ws_handlers_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/workflow-step-mutation-errors
```

Run the repository documentation coverage validator against the four package files and the intended implementation diff:

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const documents = [
  'docs/plans/workflow-step-mutation-errors/plan.md',
  'docs/plans/workflow-step-mutation-errors/task-01-classify-step-mutation-errors.md',
  'docs/specs/tasks/requirements/workflow-step-mutation-errors.md',
  'docs/specs/tasks/system-design/workflow-step-mutation-errors.md',
];
const fileContents = Object.fromEntries(documents.map(file => [file, fs.readFileSync(file, 'utf8')]));
const changedFiles = documents.map(filename => ({ filename, status: 'added' }));
changedFiles.push({ filename: 'apps/backend/internal/workflow/handlers/handlers.go', status: 'modified' });
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.status !== 'covered') process.exitCode = 1;
NODE
```

This preflight validates package traceability with an anticipated production change.
It does not claim that implementation exists or that a future live PR check passed.

## Files likely touched

- `apps/backend/internal/workflow/models/errors.go`
- `apps/backend/internal/workflow/repository/sqlite.go`
- `apps/backend/internal/workflow/repository/template_errors_test.go` (new)
- `apps/backend/internal/workflow/repository/step_errors_test.go` (new)
- `apps/backend/internal/workflow/service/access.go` (comment only)
- `apps/backend/internal/workflow/handlers/handlers.go`
- `apps/backend/internal/workflow/handlers/step_mutation_errors_test.go` (new)
- `apps/backend/internal/workflow/handlers/ws_handlers_test.go` (HTTP assertion only)
- The paired requirement, design, this work order, and `plan.md` for lifecycle/results updates.

## Dependencies

None. PR #5179 is merged and its retained-binding behavior is the compatibility baseline.

## Risks

- Removing the text match without typing the template miss breaks its existing HTTP 404 status.
- A broad helper such as `service.IsNotFound` preserves the defect through its text fallback.
- Raw dependency messages can disclose data. Preserve the safe default and existing policy messages.

## Parallelism

`sequential`

## Inputs

- [Requirement and acceptance criteria](../../specs/tasks/requirements/workflow-step-mutation-errors.md)
- [Design and ordered classification table](../../specs/tasks/system-design/workflow-step-mutation-errors.md)
- [Investigation and reproduction](plan.md#investigation-and-confirmed-cause)
- `apps/backend/AGENTS.md` and `.agents/skills/tdd/references/backend-tests.md`
- `handlers/executor_mutation_denial_test.go`, `handlers/workflow_scope_test.go`, and `repository/sqlite_test.go`

## Results

Implemented and verified, including the follow-up repository miss correction.

- The red handler regression reproduced false 404 responses for plain, wrapped, mixed-case, and untyped step-sentinel text across create, update, delete, reorder, and template application.
- Added identity-based HTTP classification and a typed missing-template repository error. The specific step-missing, template-missing, workflow-missing, workspace-missing, policy, and validation responses are covered.
- The route regressions verify successful preflight GET for update, unchanged stored steps after rejection, no successful mutation events, and safe generic responses.
- Follow-up RED tests reproduced untyped ordinary update/delete misses and HTTP 500 responses when a step disappeared after the handler read it. Typed repository errors and HTTP update/delete race tests now preserve the sanitized 404 and publish no successful mutation event.
- PR review reconciliation: a direct `repoerrors.ErrExecutorProfileNotFound` maps to `Executor profile not found`; the existing wrapped executor-mutation denial remains 403, and untyped dependency failures remain generic 500 responses without raw details.
- The profile-miss HTTP regression failed with 500 before the mapper change, then passed with 404 while proving the profile ID is not returned and no write or success event occurs.
- `go test -trimpath -tags fts5 ./internal/workflow/models ./internal/workflow/repository ./internal/workflow/service ./internal/workflow/controller ./internal/workflow/handlers -count=1` passed.
- After the review correction, changed-file Go lint passed with 0 issues; `make -C apps/backend build` passed.
- Documentation catalog validation, all 36 spec-linter tests, full spec lint, the package coverage preflight, and `git diff --check` passed after the spec and plan updates.

The work order is complete. The implementation and review correction are recorded in [PR #5316](https://github.com/kdlbs/kandev/pull/5316).
