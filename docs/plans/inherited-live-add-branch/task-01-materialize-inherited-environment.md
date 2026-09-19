---
id: "01-materialize-inherited-environment"
title: "Materialize the inherited live environment"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001
acceptance_criteria:
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.3
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.4
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.6
system_design:
  - ../../specs/tasks/system-design/attach-workspace-sources.md
---

# Task 01: Materialize the Inherited Live Environment

## Summary

Make legacy add-branch resolve and validate the environment bound to an active
inherited child session, create the sibling worktree in that environment, and
roll back the attachment whenever the live operation cannot complete.

## In scope

- Add permanent failing regressions for inherited live classification,
  executor rejection, materialization, and canonical inventory persistence.
- Replace task-owned-only launch and executor checks with effective-environment
  resolution through the selected task session.
- Validate a foreign environment's owner and worktree executor before success.
- Keep the service and materializer on the same selected session/environment
  identity and revalidate the binding at materialization.
- Preserve pre-launch deferral only when no effective environment or eligible
  bound session exists.
- Verify attachment/repository compensation and the absence of a successful
  task update on failure.

## Out of scope

- Database migrations or changes to canonical inventory uniqueness.
- Filesystem-based reconstruction of missing inventory.
- Repairing the incident's existing task, record, or manual worktree.
- Changes to launch/resume behavior, frontend projections, mobile/desktop UI,
  public docs, or non-worktree executor support.

## Acceptance

- A live child session bound to a ready parent-owned or group-owned worktree
  environment is classified as live, materializes a sibling, returns both
  exact paths, and adds one matching `task_environment_repos` row under that
  environment ID.
- A missing, unreadable, archived-owner, unprovisioned, rebound, or
  non-worktree effective environment returns an error and leaves the task's
  attachment count, repository entities, events, and existing inventory
  unchanged.
- A task with no task-owned environment and no eligible bound session retains
  the existing pre-launch deferred-attachment behavior; ordinary task-owned
  live materialization remains unchanged.

## TDD sequence

1. Add `service_branches_inherited_test.go` with a SQLite-backed inherited child
   whose active session points to a parent-owned worktree environment. Use a
   nil or failing materializer to prove the current code incorrectly accepts
   deferral, then assert rollback and no `task.updated` after the repair.
2. Add a service case with an inherited non-worktree environment and assert
   rejection occurs without adding a `task_repositories` row.
3. Extend `branch_materializer_test.go` with a parent owner, inherited child,
   child session binding, and ready parent environment. Assert the current code
   skips materialization, then implement effective resolution and assert the
   sibling path, promoted root, rescan identity, and environment inventory row.
4. Add table coverage for missing/unverifiable/archived owners and a session
   whose binding changes before materialization. Each case must fail closed.
5. Run the focused and complete package checks, then record results in this
   work order and mark the plan complete.

## Verification

```bash
# From the repository root, establish RED before production changes:
(cd apps/backend && go test -tags fts5 ./internal/task/service ./internal/backendapp \
  -run 'Test(AddBranchToTask|BranchMaterializer).*Inherited' -count=1)

# GREEN and affected-package regression checks:
(cd apps/backend && go test -tags fts5 ./internal/task/service ./internal/backendapp \
  -run 'Test(AddBranchToTask|BranchMaterializer).*(Inherited|PreLaunch|LiveTask)' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/task/service ./internal/backendapp -count=1)

# From the repository root:
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/inherited-live-add-branch
```

## Files likely touched

- `apps/backend/internal/task/service/service_branches.go`
- `apps/backend/internal/task/service/service_branches_test.go`
- `apps/backend/internal/task/service/service_branches_inherited_test.go`
- `apps/backend/internal/task/service/service_workspace_sources.go`
- `apps/backend/internal/backendapp/branch_materializer.go`
- `apps/backend/internal/backendapp/branch_materializer_test.go`
- `apps/backend/internal/worktree/store.go`
- `apps/backend/internal/worktree/store_test.go`
- A small shared task helper file only if needed to keep session selection and
  inherited-owner validation from drifting.
- `docs/plans/inherited-live-add-branch/plan.md`
- `docs/plans/inherited-live-add-branch/task-01-materialize-inherited-environment.md`

## Dependencies

- The accepted legacy live-rescan decision in
  `docs/decisions/2026-07-27-legacy-add-branch-live-rescan.md`.
- The inherited environment ownership and inventory contracts in
  `docs/specs/tasks/system-design/additional-session-workspace-reuse.md`.
- Existing `commitWorkspaceSourceBatch` compensation and worktree-store
  persistence through `task_sessions.task_environment_id`.

## Risks

- Re-querying “any session” independently at each layer can select a different
  environment during a state transition. Use the same eligibility ordering and
  verify the exact binding immediately before worktree creation.
- Swallowing repository lookup errors as “not launched” preserves the bug. A
  lookup error is a failed live operation and must compensate.
- A successful Git operation without a matching canonical inventory row still
  leaves resume unsafe. Assert the database row, not only the directory.

## Parallelism

`sequential`

## Inputs

- `REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001` and
  `AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.3`, `.4`, and `.6`.
- `docs/specs/tasks/system-design/attach-workspace-sources.md`, especially
  “Legacy add-branch effective environment.”
- The confirmed diagnostic and temporary reproduction recorded in the current
  Kandev task plan.

## Results

Implemented effective environment resolution for legacy add-branch calls. The
task service now gives the selected eligible session precedence over a stale
task-owned row, validates foreign ownership and executor support, and carries
the exact session/environment identity across attachment persistence. The
service and materializer revalidate that identity before Git creation, and the
worktree store locks the session binding in the same transaction that persists
the inherited environment's canonical inventory.

TDD RED was established with three focused regressions:

- the service accepted deferred materialization for a live inherited child;
- the materializer returned a nil result for a child bound to a parent-owned
  environment;
- the worktree store accepted an expected environment after the session had
  moved to another environment.

PR review fixup added RED coverage proving that:

- a live session binding overrides an older task-owned environment;
- a live target that disappears after preflight rolls back instead of becoming
  a deferred pre-launch success;
- owner lookup errors retain their repository cause; and
- batch workspace sources defer while a host environment is still
  provisioning without suppressing remote Docker, SSH, or Sprites
  materialization.

The shared batch commit wrapper now preserves the materializer's error identity
alongside `ErrWorkspaceSourceMaterialize`, so an owner that disappears after
preflight still reaches the add-branch MCP not-found classifier.

All regressions are GREEN. Verification completed with:

- focused inherited, pre-launch, and live-task service/materializer tests;
- complete `internal/task/service`, `internal/backendapp`, and
  `internal/worktree` package tests;
- the MCP add-branch materialized-path handler test;
- targeted race tests for inherited resolution, materialization, and rebound
  persistence;
- real Docker and SSH workspace-source E2E coverage, including remote batch
  rollback and repeated reconnect reconstruction;
- full backend `golangci-lint`;
- specification catalog validation, specification linter tests, the complete
  specification lint, and documentation diff checks.
