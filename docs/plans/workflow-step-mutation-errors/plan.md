---
created: 2026-11-13
status: implemented
requirements:
  - REQ-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001
system_design:
  - ../../specs/tasks/system-design/workflow-step-mutation-errors.md
legacy_specs: []
---

# Fix plan: Workflow step mutation errors

## Overview

[Issue #5306](https://github.com/kdlbs/kandev/issues/5306) reports a false `404 Step not found` during workflow step creation and update.
Replace text-based missing-resource classification with domain error identity.
One sequential work order delivers the mapper correction, compatible resource responses, and regression coverage.

The single sequential work order delivers the production correction and its
permanent regression coverage, including typed ordinary update/delete misses
discovered during follow-up review.

## Investigation and confirmed cause

- Checkout and remote `main` both resolved to `841d12d1dd06eadf2fe2b066cddd8832ad2e1a25` during investigation on 2026-11-13.
- The issue was open, with no attachments, comments, or assignees.
  Assignment to authenticated user `carlosflorencio` succeeded and was read back on 2026-11-13.
- [PR #5179](https://github.com/kdlbs/kandev/pull/5179) merged on 2026-11-09.
  It corrected retained executor binding eligibility and introduced a safe executor-denial mapping.
  Its merged head was `84df8bb53db6936023e5a951d89af4ed9afa0199`.
- `Handlers.writeStepMutationError` still lowercases every remaining error and matches the substring `not found`.
  That branch replaces any matching cause with the step-specific 404 response.
- Five HTTP mutation handlers share that mapper.
  Missing template application already has a test that expects the incorrect step message.

### Temporary reproduction

A throwaway `handlers/issue_5306_repro_test.go` used the existing `setupStepRouter` harness.
It created a real step and confirmed HTTP 200 on GET before injecting either dependency error:

```go
errors.New("artifact manifest not found")
fmt.Errorf("nested lookup: %w: profile-x", repoerrors.ErrExecutorProfileNotFound)
```

`Service.SetExecutorMutationChecker` returned the injected error before persistence.
Both POST `/api/v1/workflow/steps` and PUT `/api/v1/workflow/steps/<existing-id>` returned:

```json
{"error":"Step not found"}
```

The response status was 404 in all four combinations, instead of the expected server-error status.
The response discarded the dependency identity, while the handler log retained it.
The failed operations published no step events.

The exact diagnostic command was:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/workflow/handlers \
  -run '^TestReproIssue5306NestedNotFound$' -count=1 -v)
```

It failed for the expected status mismatch in both subtests.
The temporary file is removed before handoff.
This reproduction isolates the current classifier defect without replaying the obsolete retained-binding bug from v0.100.0.

## Requirement conformance and assumptions

Tasks owns the failed contract because it owns workflow definition mutation.
Existing ordering and executor requirements constrain adjacent behavior but do not define this classifier.
The new [requirement](../../specs/tasks/requirements/workflow-step-mutation-errors.md) fills that missing contract.
The paired [design](../../specs/tasks/system-design/workflow-step-mutation-errors.md) defines the mapper and its error sources.

The issue establishes that only a genuine step miss warrants a step-specific missing-resource diagnosis.
Concealed access keeps its existing indistinguishable 404 response.
Known domain errors retain safe status/message mappings.
Unexpected errors retain the existing generic 500 policy and diagnostic cause in logs.
This avoids interpreting the issue's example as authorization to disclose private executor identifiers or raw storage errors.

No material product choice remains unresolved.
No new ADR is needed for this local correction to established error and privacy boundaries.
The design preserves the rationale and existing authentication decision.

## Scope

### In scope

- Explicit identities for missing steps, templates, workflows, workspaces, and executor profiles in the shared HTTP mapper.
- A typed missing-template error from the existing repository lookup.
- Existing policy/validation precedence, safe response bodies, and rejected-write behavior.
- A corrected `ErrNotVisible` comment that describes identity-based classification.
- Regression coverage through the real HTTP routes and a table of mapper outcomes.

### Out of scope

- Executor eligibility, profile deletion, or repairs to parked cards.
- Authorization normalization and other HTTP/read, WebSocket, or MCP error classifiers.
- New public error envelopes, automatic retries, schema changes, or transactional changes.
- Browser UI, new localization copy, and additional persistent Kandev tasks or sessions.

## Technical approach

Use `errors.Is` for the step, visibility, workflow, workspace, and executor-profile identities.
Introduce `models.ErrWorkflowTemplateNotFound` and wrap it only for `sql.ErrNoRows` in `Repository.GetTemplate`.
Ordinary `Repository.UpdateStep` and `Repository.DeleteStep` writes wrap `models.ErrWorkflowStepNotFound` when zero rows are affected; version-fenced updates keep their conflict identity.
Keep unknown dependency errors on the existing generic 500 branch.
Keep the read-only, default-executor, executor-denial, and validation branches ahead of resource-miss classification.

The change stays within the workflow model, repository, handler, and one explanatory service comment.
It requires no general error abstraction or changes to policy producers.
The [work order](task-01-classify-step-mutation-errors.md) owns the exact file list and commands.

## Tests

| Acceptance criteria | Planned regression evidence |
| --- | --- |
| `.1`, `.5` | Classifier table plus repository ordinary-write misses and the update/delete race test preserve genuine missing-step identity without matching text |
| `.2`, `.5` | `TestStepMutationDependencyFailureHTTP`: raw, wrapped, and mixed-case non-step failures produce 500 |
| `.3` | `TestGetMissingWorkflowTemplateReturnsTypedNotFound`, `TestStepMutationResourceMissHTTP`, corrected HTTP missing-template assertion, and the safe executor-profile miss response |
| `.4` | Classifier table plus existing executor, default-executor, read-only, validation, and scope tests |
| `.6` | HTTP rejection matrix for all five mapper callers, stored-row assertions, event recording, and deterministic update/delete races |

All criteria use the prefix `AC-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001`.
The new test files are `handlers/step_mutation_errors_test.go`, `repository/template_errors_test.go`, and `repository/step_errors_test.go`.

## E2E tests

Registered Gin HTTP routes exercise the real controller, service, SQLite repository, and event recorder.
This is the end-to-end boundary for the API behavior in `.1` through `.6`.
No rendered UI changes, so no Playwright scenario or mobile preview is required.

## Work orders

- [x] [Task 01: Classify step mutation errors by identity](task-01-classify-step-mutation-errors.md) (`done`, sequential).

## Verification results

- Diagnostic regression: confirmed failure, with four incorrect HTTP 404 responses across two dependency errors and two routes.
- Existing handler validation, read-only, storage-failure, executor allow/default, reorder, and template tests: passed in the targeted baseline command.
- Full HTTP handler baseline: passed (`0.599s`):

  ```bash
  (cd apps/backend && go test -trimpath -tags fts5 ./internal/workflow/handlers -count=1)
  ```

- Existing typed missing-step repository regression: passed (`0.016s`):

  ```bash
  (cd apps/backend && go test -trimpath -tags fts5 ./internal/workflow/repository \
    -run '^TestGetMissingWorkflowStepReturnsTypedNotFound$' -count=1)
  ```

- `python3 scripts/list-docs.py validate`: passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- Documentation coverage preflight from the work order: passed with status `covered` and no reference errors.
  It used the four package documents and an anticipated handler change, without modifying production files.
- Catalog discovery includes the new requirement and design under `tasks`.
- Whitespace checks passed for all four untracked documents and the tracked diff.
- The temporary diagnostic test was removed before handoff; the package contained no permanent implementation or regression tests at that point.
- After implementation, `TestStepMutationDependencyFailureHTTP` failed before the correction with HTTP 404 responses for all five routes and each of its four untyped/wrapped dependency errors. The failure showed the expected false missing-step classification.
- Focused route, classification, resource-miss, template-application, and repository template-identity tests passed.
- The work-order package command passed for workflow models, repository, service, controller, and handlers.
- Changed-file Go lint passed with 0 issues.
- `make -C apps/backend build` passed for the backend and its helper binaries. macOS helper builds emitted the existing no-codesign-tool warning and left Go-signed outputs as-is.
- Final `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check` passed.
- The documentation coverage preflight passed with status `covered` and no reference errors.
- The initial implementation baseline used `origin/main` at `3a96eba9c7220efc0dc35f769f1b17223e9285f6`.
- Follow-up review regression tests confirmed the ordinary update/delete zero-row paths returned untyped errors and HTTP 500 after a step disappeared between read and write.
- Wrapping `models.ErrWorkflowStepNotFound` in those two ordinary write branches fixed both repository identity tests and deterministic HTTP races. The version-fenced `ErrTaskVersionConflict` branch remains unchanged.
- Re-ran the full targeted workflow package command, changed-file Go lint, and `make -C apps/backend build`; all passed.
- PR review found that a direct `repoerrors.ErrExecutorProfileNotFound` still used the generic 500 branch. The mapper now returns the safe `Executor profile not found` 404; a wrapped executor-mutation denial keeps 403 precedence, and unknown dependency failures remain generic 500 responses.
- The new HTTP profile-miss regression failed with the prior 500, then passed with the typed mapping and verified that the response omits the profile ID, stored steps remain unchanged, and no success event is published.
- Rebased onto `origin/main` at `723e237ef749ef7791ec7eb54ec4e4cd32ea8c32`. After the review correction, the full targeted workflow package command, `make -C apps/backend build`, changed-file Go lint (0 issues), documentation catalog validation, all 36 spec-linter tests, full spec lint, documentation coverage preflight, and `git diff --check` passed.

## Documentation impact

Internal docs updated: the requirement and design now describe the current contract, and the plan records completed implementation evidence.
No public docs change was needed: the affected workflow-editor CRUD routes are not a supported external REST contract, and no public page documents their method or error responses. The workflow-authoring process did not change.
The work order includes a documentation coverage preflight, which passed.

## Risks

- A missing template currently uses untyped text. Removing the fallback alone changes its 404 into 500.
  The typed template error prevents that regression and supplies the correct message.
- Missing templates and recognized workflow/workspace misses gain resource-specific response text.
  Clients that compare the old incorrect message must accept the corrected message.
- Reusing `service.IsNotFound` reintroduces the textual fallback through another helper.
- Raw error passthrough exposes private details and bypasses the established safe 500 policy.
- The original v0.100.0 executor scenario is historical context, not the regression fixture for current eligibility behavior.

## Handoff

Task 01 and its review correction are complete in [PR #5316](https://github.com/kdlbs/kandev/pull/5316), alongside the updated requirement, system design, plan, and work order.
