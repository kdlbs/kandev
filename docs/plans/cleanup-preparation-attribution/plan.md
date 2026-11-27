---
created: 2026-09-28
status: done
requirements:
  - REQ-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001
  - REQ-TASKS-RUNTIME-CLEANUP-001
system_design:
  - ../../specs/platform/system-design/runtime-failure-attribution.md
  - ../../specs/tasks/system-design/runtime-cleanup-preparation.md
legacy_specs: []
---

# Implementation Plan: Cleanup Preparation Failure Attribution

## Overview

Make archive preparation and deletion-preview failures identify unavailable Git
repository context while preserving existing cleanup refusal and consent rules.
One sequential work order extends the classification already used by cleanup
workers through the preparation, service, and HTTP boundaries.

Platform owns failure attribution under
[REQ-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001](../../specs/platform/requirements/runtime-failure-attribution.md),
criterion .6. Tasks retains lifecycle safety under
[REQ-TASKS-RUNTIME-CLEANUP-001](../../specs/tasks/requirements/runtime-cleanup.md),
criterion .9. Existing requirements cover this regression; no new product
requirement or automatic recovery policy is introduced.

## Confirmed root cause

A linked checkout survived removal of its source repository. Its `.git` pointer
therefore named a nonexistent administrative directory. `rev-parse` and `status`
both returned exit 128. Unlike the worker audit, `captureCleanupHeadOID` and
`InspectDirtyWorktrees` propagated the unclassified subprocess error.
`TaskDeletePreflight` also formatted its inspection cause with `%v`, losing
the type chain, and the HTTP 503 branch emitted no attributed diagnostic.

The earlier [missing-worktree package](../missing-worktree-cleanup-preparation/plan.md)
handles verified checkout/branch absence in an accessible source repository.
This incident has surviving files and unavailable repository context. Treating
it as that earlier absence case would lose the evidence needed to protect work.
The [worker inspection package](../startup-log-corrections/task-01-cleanup-inspection.md)
already defines the reusable error type. Its completed worker work remains
complete; this package covers preparation and preview callers it did not change.

## Incident repair and evidence

The user explicitly requested repair of the current database. An operator-only
repair on 2026-09-28 retired exactly three stale `task_environment_repos` rows
after establishing absent source clones, terminal sessions, absent/stopped
executors, no recovery claims, and no active cleanup jobs. A consistent online
SQLite backup and verified copies of both surviving directories were retained
privately outside the repository. The third checkout was already absent.

The transaction compared original inventory fields and preserved task/session
rows, repository definitions, healthy sibling worktrees, and historical cleanup
jobs. Original files remain unchanged. Both the backup and repaired database
have zero foreign-key violations. Recovery receipts and original row values
remain beside the private backup; no user data or credentials belong in Git.

A temporary read-only Go test drove the real store and manager against the
pre-repair backup and live repaired inventory. Before repair, all three tasks
failed identity capture; both surviving damaged checkouts also failed dirty
inspection. After repair, all three passed identity capture, dirty inspection,
and archive-source-manifest capture. The reported task retains one healthy
checkout with local changes, so normal discard consent remains necessary.

The live HTTP endpoint required authentication and returned 401 to the
unauthenticated verification request. No login credential was available. The
manager evidence proves the failing inspection boundary is repaired; it is not
claimed as a successful authenticated HTTP archive or deletion.

A separate disposable Git fixture reproduced the permanent code defect:
`TestReproCleanupMissingSourceAttribution` failed for both capture and dirty
inspection because neither error satisfied `errors.As(*CleanupInspectionError)`.
The surviving content remained intact. Temporary tests are removed at handoff.

## Scope

### In scope

- Typed, bounded attribution for identity capture and dirty inspection.
- Preserve the cause through preflight service wrapping.
- Log stage/reason at existing HTTP archive and preflight failure boundaries.
- Real Git, mixed-inventory, service, and HTTP regression evidence.

### Out of scope

- Automatically retiring or deleting unknown workspaces or database inventory.
- Recreating deleted source repositories, commits, indexes, or branches.
- Changing HTTP status/body contracts, frontend copy, retry policy, or schemas.
- Rewriting historical failed cleanup jobs as successful.

## Technical approach

Extend the existing `CleanupInspectionError` path in
`manager_cleanup_audit.go` to preparation sites in `manager_cleanup.go`,
`manager_cleanup_capture.go`, and `manager_cleanup_dirty.go`. Add only the
`working_tree_status` stage; reuse current reasons and subprocess bounds.
Use recorded source identity and existing linked-worktree inspection evidence
to distinguish unavailable repository context from a generic Git failure.

Retain cancellation/deadline precedence and the wrapped cause. Neither a bare
128 nor an arbitrary filesystem ENOENT is proof of absent tracked work. Do not
globally change `runBoundedGitInspect` to append raw output to errors.

Use error wrapping in `task_delete_preflight.go` that preserves both the
preflight sentinel and the inspection cause. Keep the authorization path and
confirmation-ticket rules unchanged. The HTTP handler logs closed stage/reason
fields and a bounded selection count. `errors.go` attaches those same fields to
the existing archive error log when available. No raw paths, stderr, request
bodies, or credentials are added to logs or HTTP responses.

This is a conformance correction to existing diagnostic and fail-closed
contracts, so no new ADR is needed. Public documentation has no new product
operation to describe; the incident-specific SQL repair is not a public recipe.

## Tests

| Evidence | Acceptance criteria |
| --- | --- |
| `TestCleanupPreparationInspectionAttribution` with real Git and mixed inventory | Platform .6; Tasks .9 |
| `TestTaskDeletePreflightPreservesInspectionCause` | Platform .6; Tasks .9 |
| `TestHTTPTaskDeletePreflightLogsInspectionFailure` | Platform .6 |
| `TestHandleNotFoundLogsCleanupInspectionFailure` | Platform .6 |
| Existing missing-worktree, replacement-worktree, and dirty-inspection regressions | Tasks .9 |

Platform refers to `AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001`; Tasks refers
to `AC-TASKS-RUNTIME-CLEANUP-001`.

## End-to-end boundary

HTTP handler integration tests cover an authorized preflight request through
the service, its unchanged 503 response, and the emitted bounded diagnostic.
The manager's real-Git tests independently prove the repository failure. No
rendered interaction changes, so no new browser or mobile suite is required.

## Work orders

- [done] [Task 01: Preserve cleanup preparation attribution](task-01-preserve-attribution.md)

## Verification results

Diagnostic commands from `apps/backend`:

- `go test -tags fts5 ./internal/worktree -run '^TestIncidentCleanupInventoryReadOnly$' -count=1 -v`: passed against original and repaired inventory using read-only connections.
- `go test -tags fts5 ./internal/worktree -run '^TestReproCleanupMissingSourceAttribution$' -count=1 -v`: failed for the expected missing typed attribution at both sites.

The user authorized implementation in a later turn. The permanent tests
reproduced the missing manager attribution, service error chain, and HTTP log
fields before production edits. After the correction, the focused regression
command and all three race-test commands in the work order passed. Scoped
`golangci-lint run --new-from-rev=HEAD` passed with zero issues for the worktree,
task service, and task handler packages. No HTTP contract, schema, cleanup
decision, or automatic repair policy changed.

Design validation passed:

- `python3 scripts/list-docs.py validate`: 326 decisions and 1240 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: all 36 tests passed.
- `.github/scripts/pr-docs.cjs` local `validateCoverage`: design diff exempt;
  planned implementation covered by this work order with no errors.
- Work-order requirements have referenced design owners; all work-order designs
  are declared by the plan.
- `git diff --check`: passed.

Final specification catalog/lint and diff validation also passed. The database
repair was applied separately from the code change. Delivery uses the normal
commit hooks and a pull request; the running backend has not been rebuilt or
restarted with this code.

## Risks

- A missing executable and missing working directory can both surface as
  ENOENT; classification must use actual context rather than the exit alone.
- Error formatting can accidentally discard either the unavailable sentinel or
  context/cause identity; assert both with `errors.Is` and `errors.As`.
- Restoring the removed source repository requires separate recovery work if
  the operator wants to resume these plugin tasks. This repair preserves files
  and enables task retirement; it does not reconstruct unavailable Git history.
- Future missing-source incidents still require deliberate preservation and
  repair. Better attribution does not grant automatic deletion authority.
