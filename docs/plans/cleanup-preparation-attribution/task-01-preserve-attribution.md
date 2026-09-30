---
id: "01-preserve-attribution"
title: "Preserve cleanup preparation attribution"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001
  - REQ-TASKS-RUNTIME-CLEANUP-001
acceptance_criteria:
  - AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.6
  - AC-TASKS-RUNTIME-CLEANUP-001.9
system_design:
  - ../../specs/platform/system-design/runtime-failure-attribution.md
  - ../../specs/tasks/system-design/runtime-cleanup-preparation.md
---

# Task 01: Preserve Cleanup Preparation Attribution

## Summary

Extend existing cleanup inspection attribution through preparation and the
delete-preview request. Unavailable repositories produce bounded stage/reason
diagnostics while files, task state, HTTP responses, and cleanup safety retain
their current behavior.

## In scope

- Identity capture, exact-ref capture, and dirty-status inspection errors.
- Existing source-repository and linked-worktree classification evidence.
- Typed service wrapping and attributed archive/preflight request logs.
- Real-Git and HTTP/service regression tests before implementation.

## Out of scope

- New recovery endpoints, inventory mutation, or automatic repository repair.
- Raw Git-output logging, global subprocess error-format changes, UI changes.
- Migrations, new runtime flags, retry changes, or historical-job edits.

## Acceptance

1. Missing source or proven missing administrative metadata is distinguishable
   from command/start failure, cancellation, and verified ref absence in both
   capture and dirty inspection. Generic errors never authorize cleanup.
2. Preflight retains its unavailable sentinel and typed cause; HTTP preflight
   and archive failures log stage/reason without paths, raw stderr, or payloads.
   Their existing status and response-body contracts remain unchanged.
3. Healthy/broken mixed inventories fail before task mutation or consent-ticket
   issuance. Files, branch state, and inventory remain intact, and existing
   verified-absence and changed-identity protections pass.

## Implementation sequence

1. After an explicit implementation request, mark this order `in_progress`.
2. Create the focused tests below and record the expected red run.
3. Apply classification at the manager boundaries, preserving underlying errors.
4. Preserve the preflight error chain and add the bounded handler diagnostics.
5. Run every verification command and record results here and in `plan.md`.
6. Mark the order `done` only when the required checks pass.

## Regression matrix

Use `newReferenceCleanupTestManager` and disposable Git repositories. Add
`manager_cleanup_preparation_inspection_test.go` instead of growing large test
files. `TestCleanupPreparationInspectionAttribution` must cover:

- A real linked checkout whose source clone is removed while files survive.
- An absent checkout and absent source clone, which remains unknown.
- A present source with the linked administrative directory removed.
- A healthy sibling before and after the broken slot, preserving both slots.
- Verified absent branch in an accessible source as the successful control.
- Git start failure, generic exit 128, cancellation, and deadline; inspect the
  cause identity and preserve existing reasons without parsing localized stderr.
- Intact sentinel files, inventory, and healthy refs after each refusal.

`TestTaskDeletePreflightPreservesInspectionCause` injects the typed inspection
error through the existing service fixture. Assert the unavailable sentinel,
stage/reason, underlying cancellation where applicable, and no confirmation ID.

`TestHTTPTaskDeletePreflightLogsInspectionFailure` uses the existing authorized
handler fixture with an observed logger. Assert one diagnostic, closed fields,
no raw path/payload values, HTTP 503, and the current no-store generic body.
`TestHandleNotFoundLogsCleanupInspectionFailure` covers the wrapped archive
error path and unchanged HTTP 500 behavior. Retain authorization/not-found and
dirty-consent tests as controls.

## Verification

Run new tests against unchanged production code first:

```bash
(cd apps/backend && go test -tags fts5 ./internal/worktree ./internal/task/service ./internal/task/handlers -run '^(TestCleanupPreparationInspectionAttribution|TestTaskDeletePreflightPreservesInspectionCause|TestHTTPTaskDeletePreflightLogsInspectionFailure|TestHandleNotFoundLogsCleanupInspectionFailure)$' -count=1 -v)
```

After the correction, from the repository root:

```bash
(cd apps/backend && go test -race -tags fts5 ./internal/worktree -run 'Test(CleanupPreparationInspectionAttribution|CaptureCleanupHeadOIDs_|InspectDirtyWorktrees|CleanupInspection|BranchExistsDistinguishes)' -count=1)
(cd apps/backend && go test -race -tags fts5 ./internal/task/service -run 'Test(TaskDeletePreflight|PrepareTaskResourceCleanup_MissingWorktree|TaskLifecycleCleanup_MissingWorktree|ResourceCleanupInspection)' -count=1)
(cd apps/backend && go test -race -tags fts5 ./internal/task/handlers -run 'Test(HTTPTaskDeletePreflight|HandleNotFoundLogsCleanupInspectionFailure)' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/worktree/manager_cleanup.go`
- `apps/backend/internal/worktree/manager_cleanup_capture.go`
- `apps/backend/internal/worktree/manager_cleanup_dirty.go`
- `apps/backend/internal/worktree/manager_cleanup_audit.go`
- `apps/backend/internal/worktree/manager_cleanup_preparation_inspection_test.go` (new)
- `apps/backend/internal/task/service/task_delete_preflight.go`
- `apps/backend/internal/task/service/task_delete_preflight_test.go`
- `apps/backend/internal/task/handlers/task_delete_preflight.go`
- `apps/backend/internal/task/handlers/task_delete_preflight_test.go`
- `apps/backend/internal/task/handlers/errors.go`
- `apps/backend/internal/task/handlers/cleanup_inspection_error_test.go` (new)
- This work order and its plan.

## Dependencies

None. Existing worker attribution is already present. Execute sequentially in
the primary session; there is no delegation authorization.

## Risks

The live repair retired three specific records under explicit user authority.
It is not permission to add that mutation to automatic inspection. Preserve the
fail-closed contract and do not manufacture missing commits or clean status.

## Parallelism

`sequential`

## Inputs

- [Incident evidence and repair scope](plan.md).
- [Failure attribution requirements](../../specs/platform/requirements/runtime-failure-attribution.md), criterion .6.
- [Task cleanup requirements](../../specs/tasks/requirements/runtime-cleanup.md), criterion .9.
- [Attribution design](../../specs/platform/system-design/runtime-failure-attribution.md#cleanup-preparation-and-deletion-preview).
- [Preparation design](../../specs/tasks/system-design/runtime-cleanup-preparation.md).
- `manager_cleanup_inspection_test.go` and `resource_cleanup_inspection_test.go` for existing classification and observed-log patterns.
- [Fail-closed cleanup decision](../../decisions/0009-fail-closed-gc-semantics.md).

## Results

Implemented on 2026-09-29 after the user's explicit implementation request.
Preparation and dirty-status errors now retain the existing typed classifier,
using absent source and linked-metadata evidence without changing cleanup
decisions. The service preserves both error chains, and the HTTP boundaries
emit stage/reason diagnostics with the existing generic response bodies.

- Red: the four permanent regression entry points above failed against
  unchanged production code. The manager errors lacked typed attribution,
  preflight discarded the cause, the 503 handler emitted no diagnostic, and
  archive logging lacked stage/reason fields.
- Green: the same focused command passed for all three packages after the
  correction.
- All three targeted `go test -race -tags fts5` commands above passed, including
  verified absence, concurrent removal, dirty-consent, authorization, mixed
  inventory, cancellation, deadline, and HTTP safety controls.
- `golangci-lint run --new-from-rev=HEAD ./internal/worktree ./internal/task/service ./internal/task/handlers`
  from `apps/backend`: passed with zero issues.
- `python3 scripts/list-docs.py validate`: passed, covering 326 decisions and
  1240 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.

No live database changes were needed during implementation. The incident
repair and its evidence remain recorded in the plan. The running backend has
not been rebuilt or restarted with this code.
