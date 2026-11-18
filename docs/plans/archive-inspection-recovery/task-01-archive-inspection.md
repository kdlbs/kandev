---
id: "01-archive-inspection"
title: "Separate archive admission from physical inspection"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-005
acceptance_criteria:
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-005.1
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-005.2
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-005.3
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-005.4
system_design:
  - ../../specs/tasks/system-design/runtime-cleanup-preparation.md
---

# Task 01: Separate archive admission from physical inspection

## Summary

Archive the selected tasks when physical inspection fails. Persist blocked
cleanup evidence and preserve uncertain workspaces without blocking healthy ones.

## In scope

Implement both preparation paths and archive worker partitioning. Cover all
regressions named in the plan. Update the archive section of public task docs
when implementation ships. Keep the original snapshot across restart and retry.

## Out of scope

Live data repair, new controls, delete consent changes, and new schema.

## Acceptance

1. Typed physical inspection errors allow archive to commit, including a mixed
   parent/child tree. The API returns success and publishes normal archive state.
2. Unknown resources and enclosing paths survive cleanup and restart. Healthy
   siblings progress. Retries cannot replace missing original identity.
3. Authorization, inventory, persistence, parent cancellation, and mutation
   fences retain their existing behavior. Delete recovery tests still pass.

## Verification

Run from the repository root. First run the named new tests before production
changes and record their expected failure. Then run this complete block:

```bash
(cd apps/backend && go test -trimpath ./internal/task/service -run 'TestArchiveInspectionFailure|TestArchiveTaskTree|TestRetainDeletion|TestTaskDelete' -count=1)
(cd apps/backend && go test -trimpath ./internal/worktree -run 'Test.*Cleanup.*(Capture|Inspection|Preparation)' -count=1)
(cd apps/backend && go test -trimpath ./internal/task/handlers -run TestArchiveInspectionFailureHTTPCommitsTree -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Run `bash scripts/lint-go-changed` with the exact changed Go paths after the
implementation file set is known. Record that concrete command in Results.

## Files likely touched

- `apps/backend/internal/task/service/resource_cleanup_retained.go`
- `apps/backend/internal/task/service/resource_cleanup_preparation.go`
- `apps/backend/internal/task/service/resource_cleanup_jobs.go`
- New `apps/backend/internal/task/service/resource_cleanup_archive_inspection.go`
- `apps/backend/internal/task/service/handoff_cascade.go` only if response integration requires it
- The four test files named in the plan
- `apps/backend/internal/task/service/service_tasks.go` (shared-resource completion guard)
- `docs/public/tasks-and-workflows.md`

## Dependencies

None.

## Risks

A blocked resource must fence all ancestor removal routes, including environment
and SSH reclamation. Do not enable delete-only unverified cleanup for archive.
No inspection-error classification may swallow a database error.

## Parallelism

`sequential`

## Inputs

- Archive requirement 005 criteria .1 through .4.
- Archive admission amendment in the referenced design.
- `resource_cleanup_unverified.go` and `task_delete_unavailable_test.go` as
  evidence of existing snapshot patterns, not archive policy.
- `handoff_cascade_absent_environment_test.go` for cascade test setup.
- ADR 0009: fail-closed GC semantics.

## Results

- RED: direct and cascade archive rejected typed commit inspection errors.
  Independent healthy cleanup also failed. The shared-resource completion test
  later reproduced a false completed result.
- All three Go commands in Verification passed. The expanded service selection
  `Archive|Cleanup|RetainDeletion|TaskDelete` also passed with `-race`.
- Catalog validation, specification lint, and `git diff --check` passed.
- Public docs: 63 validator tests passed and 55 pages validated.
- Go lint passed with zero issues. The host lacked `golangci-lint`.
  Installed repository CI version v2.9.0 into `/tmp/kandev-archive-tools`.
  Passed the lint base as a command-local Git config value because origin/HEAD
  was absent. No repository Git configuration was changed.

Exact lint command:

```bash
PATH=/tmp/kandev-archive-tools:$PATH GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=kandev.lintBaseRef GIT_CONFIG_VALUE_0=origin/main bash scripts/lint-go-changed apps/backend/internal/task/service/resource_cleanup_archive_inspection.go apps/backend/internal/task/service/resource_cleanup_archive_inspection_test.go apps/backend/internal/task/service/resource_cleanup_archive_progress_test.go apps/backend/internal/task/service/resource_cleanup_archive_reclaim_test.go apps/backend/internal/task/service/resource_cleanup_archive_runtime_test.go apps/backend/internal/task/service/handoff_cascade_archive_inspection_test.go apps/backend/internal/task/service/resource_cleanup_jobs.go apps/backend/internal/task/service/resource_cleanup_preparation.go apps/backend/internal/task/service/service_tasks.go apps/backend/internal/task/handlers/task_archive_inspection_test.go apps/backend/internal/worktree/manager_cleanup_preparation_inspection_test.go
```

### PR review remediation

- Added regressions for retained orphan-reaper roots after partial cleanup,
  stopped runtime-row removal with mixed stop outcomes, nil worktree manifest
  accounting, and the reclaim source-manifest guard with legacy compatibility.
- The first regression runs reproduced lost path evidence, retained stopped
  rows, and incomplete manifest accounting. Production repository wiring uses
  `sqlite.Repository`, which implements `ListTaskResourceCleanupJobs` for both
  supported database dialects.
- Source-manifest enforcement uses an additive snapshot marker. Existing
  snapshots keep their previous reclamation behavior.

Post-review validation:

```bash
(cd apps/backend && go test -trimpath -race ./internal/task/service -run 'Archive|Cleanup|RetainDeletion|TaskDelete' -count=1)
(cd apps/backend && go test -trimpath ./internal/task/handlers -run TestArchiveInspectionFailureHTTPCommitsTree -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

All passed. The race suite completed in 27.996s and the HTTP regression in
0.247s. The reclaim regression reproduced removal without manifest evidence
before the guard was added. Updated two existing reclaim fixtures to supply
manifest evidence or retry the original archive after a successful stop.

Changed-file Go lint also passed with zero issues after review remediation.
