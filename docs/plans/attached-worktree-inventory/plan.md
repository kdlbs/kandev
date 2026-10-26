---
created: 2026-10-26
status: implemented
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-003
system_design:
  - ../../specs/tasks/system-design/attach-workspace-sources.md
  - ../../specs/tasks/system-design/workspace-repository-placement.md
legacy_specs: []
---

# Implementation Plan: Attached worktree inventory

## Overview

Fix [issue #4797](https://github.com/kdlbs/kandev/issues/4797) at attachment production so a newly
attached Worktree repository retains its placement in canonical inventory after restart. One
sequential work order adds permanent regressions, carries placement into the physical writer,
and verifies persistence, compensation, and exact retries. ROOT released implementation in a
later turn after design review and granted the exclusive heavy-command lease.

Tasks owns durable attachment inventory. Reuse the active
[attachment requirements](../../specs/tasks/requirements/attach-workspace-sources.md), specifically
AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.2/001.4 and
AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.6/003.7. They already define the failed contract; no new
requirement or ADR is needed. The existing attachment design and its placement extension remain
the owning technical sources. Do not promote unrelated draft placement scope in this repair.

## Confirmed cause and evidence

Baseline: `b4e9cf5ede3545ca6a8e58165e580ad7c4988afa`. ROOT independently reproduced a successful
`MaterializeWorkspaceSources` call with a real SQLite worktree store and a real nested worktree:
the new `task_repositories` row recorded `kandev/added`, position 1, while its matching
`task_environment_repos` row recorded an empty path, position 0. ROOT's causal run exited 1;
nil-store nested and parent-root controls passed. The earlier cold-build timeout was incomplete,
not product RED. This package does not read, copy, run, or modify ROOT's private reproduction.

Independent source inspection establishes the chain:

1. `branchMaterializer.prepareMaterializeRequest` carries the task-relative path but omits position.
2. `Manager.buildWorktreeRecord` drops the path before persistence.
3. `SQLiteStore.CreateWorktree` inserts a pathless row with hardcoded position 0.
4. `workspaceSourceInventoryRows` skips the already present repository/branch key.
5. Ordinary executor reuse requires exact recorded-path agreement and rejects the row.

## Scope

In scope: complete new-slot placement publication, SQLite-backed real-Git regression coverage,
parent-root and both nested placements, same-repository branch separation, reopen persistence,
post-create failures, exact retries, and preservation of existing canonical placement.

Out of scope: historical automatic repair, resume guard changes, migrations, timestamps,
dialog/send-error symptoms without causal evidence, new UI or localization, lifecycle redesign,
root expansion, retained-checkout work, and unrelated documentation corrections.

## Technical approach

Carry server-recorded placement through the existing physical creation path:

- Add `Position` to `worktree.CreateRequest`; retain its existing `WorkspaceRelativePath` semantics.
- Add internal `WorkspaceRelativePath` and `Position` fields to `Worktree`, both excluded from JSON.
- Populate request position from `TaskRepository.Position` in `branch_materializer.go` and copy
  both fields into new worktree records in `manager_state.go`.
- In `store.go`, include both fields in the INSERT, replacing hardcoded position 0. Include them
  in the shared read projection/scan so a stored worktree round-trips consistently.
- Preserve the existing ON CONFLICT placement: update physical metadata as today, without
  overwriting canonical path/position. This protects startup/recreate callers with omitted values
  and avoids converting worktree creation into legacy inventory repair.

Do not change `workspace_source_materializer_sources.go`: its skip remains correct once physical
creation inserts complete placement. Retain session/environment revalidation, physical identity
checks, cleanup barrier, recovery claim, and the current transaction. No schema or public wire
change is required. New metadata is an internal producer input.

Failure handling remains the existing compensated batch. A persistence error rolls back the
store transaction and removes its newly created physical directory. A later-source or rescan
failure restores runtime roots/exclusions and cleans only worktrees absent from the pre-batch
snapshot. The service compensates new source rows. Existing canonical slots are not rewritten,
so no updated-row undo journal is needed. Cleanup may retain inactive history under existing
policy; it must leave no active failed attachment. Retry must restore one active slot at the
same recorded destination. Do not claim a rollback result from a nil-store fixture.

## Tests

All new tests are independently authored after implementation release. New files avoid growing
the already large shared test files.

- `workspace_source_materializer_inventory_test.go`:
  `TestWorkspaceSourceMaterializer_PersistedAttachmentPlacement` covers AC-001.2/003.7 with real
  SQLite + `newPersistingMaterializerWorktreeMgr`, actual Git worktrees, and table cases for
  `kandev_directory`, `current_root`, and an already parent-rooted task. Use nonzero positions.
  Include a new repository and two branch slots of one repository with distinct placement and
  identity; assert every new row, not merely that one row exists. Reopen the database and use
  a fresh manager to prove paths/positions/IDs and physical reuse without cached authority.
  Existing nil-store tests remain controls. This is the expected causal RED.
- In the same file, `TestWorkspaceSourceMaterializer_PersistedAttachmentRollbackAndRetry`
  covers AC-001.4/003.6 with a first worktree successfully persisted before a later-source
  failure, and a failure after creation during rescan. Assert prior inventory/physical IDs,
  root/layout/exclusions, and unrelated files remain; no new active slot or directory survives.
  Drive the public `taskservice.Service.AttachWorkspaceSources` boundary for source-row
  compensation and exact committed retry (`Changed=false`, no extra rescan/rebind). Retry after
  manager reconstruction against the same persisted database preserves selected
  placement and exactly one active slot. The two-session rollback assertions remain intact.
- `store_placement_test.go`: `TestSQLiteStore_CreateWorktreePlacement` covers insert/readback,
  two branch slots, and conflict updates with omitted or different placement inputs. Assert
  existing row ID/path/position remain intact while allowed physical metadata updates succeed.
  Include unrelated failed/deleted inventory so it cannot masquerade as a required live slot.
- `store_placement_postgres_test.go`: `TestSQLiteStore_CreateWorktreePlacementOnPostgres`
  runs the same store contract through `testutil.OpenIsolatedPostgres`, gated by the existing
  `KANDEV_TEST_POSTGRES_DSN` convention. Report a skip honestly if no DSN is supplied.

## E2E evidence and presentation impact

The backend integration crosses the public attachment service, real materializer, real worktree
store, filesystem, rollback, and reopened persistence. It supplies the end-to-end evidence for
this persistence defect. Existing resume guard tests remain consumer protection; no consumer
test edits or browser suite are required. No claim is made that unproven dialog symptoms are fixed.

No rendered UI changes or phone composition changes are planned, so mobile-parity implementation,
ASCII previews, and Playwright additions are unnecessary. Desktop and phone share the repaired
backend. Public documentation assessment: `docs/public/tasks-and-workflows.md`,
`docs/public/coordination.md`, `docs/public/developer-tools.md`, root README and screenshot catalog
already describe attachment/placement. Their contract and instructions remain correct; no public
page, screenshot, API, or terminology update is needed. Internal design and delivery records change.

## Work orders

- [x] [Task 01: Publish complete attachment inventory](task-01-publish-placement.md)

## Verification results

Design checks passed on 2026-10-26:

- `python3 scripts/list-docs.py validate`: exit 0, 403 decisions and 1740 specifications.
- `python3 scripts/lint-spec-files.test.py`: exit 0, 36 specification-linter tests.
- `python3 scripts/lint-spec-files.py --all`: exit 0, all specification files passed.
- Working-file `.github/scripts/pr-docs.cjs` `validateCoverage`: exit 0, `covered`, no errors;
  both REQ-001/003 resolve through the two existing designs. The source trigger is prospective
  during this design-only preflight.
- `git diff --check -- docs/specs docs/plans/attached-worktree-inventory`: exit 0.
- `git status --short -- docs/plans/attached-worktree-inventory`: the single new package is untracked;
  it contains exactly one manifest and one pending work order.

ROOT reviewed the producer design in substance and requested cache-compatible flags, finite
compile/test bounds, separate test/lint budgets and external per-invocation logs. Task 01 now
records `GOTOOLCHAIN=local`, `-trimpath -race -tags fts5 -p 1`, GNU outer timeouts, and fresh
child-owned evidence directories. This review is not implementation release.

Implementation checks completed on 2026-10-26, sequentially under the ROOT heavy lease:

- Focused regression RED: exit 1, all three placements exposed empty canonical paths after
  successful materialization; no production edits preceded it. Focused GREEN: exit 0.
- Revised real-store matrix: exit 0, including strong two-session rollback and public service
  retry after manager reconstruction. The original same-manager failure is preserved separately.
- The exact affected-test command in Task 01: exit 0, backendapp 13.839s, worktree 4.204s,
  task/service 4.406s. Every new regression ran after the final production edit.
- The exact SQL guard command in Task 01: exit 0.
- The exact race-enabled store-conformance command in Task 01: exit 0, 52.313s.
- The exact canonical changed-file lint command in Task 01: exit 0 with its default 3GiB budget;
  available memory was 15439MiB at launch. New paths were intent-to-add for diff coverage.
- PostgreSQL cases skipped at the existing DSN gate; no PostgreSQL runtime coverage is claimed.
- Catalog validation, full specification lint, actual added/modified-path `validateCoverage`
  (`covered`, no errors, ten owned paths), and diff/working-file whitespace checks passed.

Command startup/join receipts and full logs are retained in task-owned external evidence under
`/home/jcfs/.kandev/loop-evidence/1a588848-c703-480a-bc22-1eb3f18f8b10/` in `red.ga1roypz`,
`focused-green.tiiu39k_`, `reconstructed-retry-2.aie_eh_m`, `affected-green.x78egpk6`,
`sqlguard.7l8qolah`, `conformance.7biuzk1n`, and `scoped-lint.b7kc0nav`.
All these commands actually joined, and their known owned process groups were absent.
The transient incomplete test fixture/import failures remain in their original receipts;
they are not product RED evidence. Normal frozen dependency install, commit hooks and hosted
validation are delivery evidence recorded in the Kandev task plan, not another product test gate.


## Risks and overlap

- Omitted creation metadata must never reset an existing canonical slot. Test the conflict path.
- Real-store rollback can retain inactive history; assert active inventory and exact prior slots,
  rather than requiring all historical rows to disappear.
- PR #4722 (OPEN, `84250d9ab08149f18683671272611df8bc52a5a2`) owns retained-checkout consumers
  and workspace-inventory-repair docs. This package avoids those files. PR #4798 (OPEN,
  `aced58fb7338877cd77ee0127c9afa8c50018270`) has no proposed producer-file overlap.
- Recheck relevant ownership before implementation; main drift alone does not justify rebasing.
- Existing damaged tasks are deliberately not healed by this producer repair.

## Known deferred limitation

The two-session real-store fixture exposed an independent same-manager live retry failure: task-level
cleanup can project a different session than the creation cache key, leaving the creator session
with stale worktree cache state after cleanup. The next live retry can enter recreation of a deleted
generated branch and fail with `ErrBranchUnrecoverable`. ROOT approved preserving both-session
rollback assertions and proving retry after manager reconstruction against the same persisted DB,
which models the restart boundary relevant to #4797. Committed exact retry remains a no-op.
This package does not fix or claim coverage of successful same-manager multi-session retry.
The original fixture and actual native62766 exit1 log/receipt are retained in child-owned external
evidence; no persistent follow-up task or issue is created.
