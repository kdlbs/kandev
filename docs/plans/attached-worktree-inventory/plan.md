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
and verifies persistence, compensation, and exact retries.

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
- Preserve existing ON CONFLICT placement for active or partially deleted slots. For a fully
  deleted old row (`status = deleted` and non-null `deleted_at`) reactivated by incoming active,
  non-deleted materialization, replace path and position together from creation inputs in the
  same transaction as physical tuple/status. Empty path and zero position remain legitimate
  inputs; do not infer authority from non-empty metadata. Keep row identity and all current locks.
  Ordinary resume still cannot repair missing/inconsistent inventory.

Do not change `workspace_source_materializer_sources.go`: its skip remains correct once physical
creation inserts complete placement. Retain session/environment revalidation, physical identity
checks, cleanup barrier, recovery claim, and the current transaction. No schema or public wire
change is required. New metadata is an internal producer input.

Failure handling remains the existing compensated batch. A persistence error rolls back the
store transaction and removes its newly created physical directory. A later-source or rescan
failure restores runtime roots/exclusions and cleans only worktrees absent from the pre-batch
snapshot. The service compensates new source rows. Prior live canonical slots are not rewritten,
so no live-slot undo journal is needed. Cleanup may retain inactive history under existing policy;
it must leave no active failed attachment. A new admitted retry may select a different destination
and order when replacing a fully deleted slot. Do not claim a rollback result from a nil-store fixture.

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

## Corrective amendment

After compensated attachment leaves a deleted row, a new attachment at another placement can
revive its physical tuple while retaining the prior path/order. The independently authored
changed-placement regression demonstrated that stale path after reopening SQLite before correction.

The strict reactivation boundary above supersedes unconditional conflict-placement preservation.
Retain the existing active REQ-001/003 and AC-001.2/001.4/003.6/003.7. No new requirement, ADR,
production boundary, schema, migration, file or consumer/guard change is introduced.

Task 01 independently adds
`TestWorkspaceSourceMaterializer_PersistedAttachmentChangedPlacementRetry` before source correction.
Use the real SQLite/Git public-service fixture and post-create rescan
failure, retain two-session rollback and prior/foreign preservation assertions, reconstruct the
manager against the same persisted DB, select another valid placement, and reverse the two new
source rows so nonzero order changes as well. After successful attachment, reopen SQLite and
assert every new durable association matches canonical path/order, exact physical ID/path, one
active slot per branch, and preserved prior/foreign state. The old destination must remain absent.
A same-manager multi-session retry remains explicitly deferred.

Extend the shared store contract in the two existing store test files with
`TestSQLiteStore_CreateWorktreeReactivationPlacement` and its `OnPostgres` counterpart. Cover
fully deleted reactivation with changed values and legitimate empty/zero values; active rows,
deleted status without deletion time, active status with deletion time, and failed status retain
placement. Fully deleted rows also retain placement for inactive or deletion-marked incoming
writes. Assert row ID preservation and physical/status updates under the existing identity rules.

The reopen test now invokes real branch materialization/manager reuse with the identical admitted
placement. Assert unchanged worktree ID/path, canonical placement/order, slot cardinality and
preserved checkout files. Do not expand into the executor's attach-and-resume flow.

Corrective ownership is limited to `store.go`, the three existing test files, and this package's
three documents. The other three producer files are unchanged by this amendment. Task 01's
corrective verification block defines focused RED/GREEN and store
controls, changed-SQL guard/conformance, canonical lint over actual changed Go paths, and document
checks. Do not replay the initial broad service families or install dependencies again.

## Verification results

Corrective design checks passed: catalog validation (403 decisions, 1740 specifications), full
specification lint, diff whitespace, and actual cumulative ten-path documentation coverage
(`covered`, no errors). Exactly one manifest and one work order remain.

Corrective implementation checks completed sequentially on 2026-10-26:

- Changed-placement/reordered public-service retry RED: exit 1 after reopening SQLite, expected
  `kandev/kandev/repo-3-main` but stored `kandev/repo-3-main`. No producer edit preceded RED.
  Focused GREEN: exit 0 after correcting strict deleted-slot reactivation.
- Corrective targeted command in Task 01: exit 0, backendapp 6.472s and worktree 2.351s.
  Includes actual fresh-manager physical reuse for all three placement cases, exact committed
  retry no-op, two-session rollback, ten-case store predicate matrix, and attachment controls.
- Corrective SQL guard: exit 0. Race-enabled store conformance: exit 0, 51.910s.
- Canonical lint over the four actually changed corrective Go paths: exit 0, zero issues,
  default 3GiB budget after checking available memory (15598304kB).
- The shared placement fixture now rebinds parameterized seed SQL for PostgreSQL. Local
  PostgreSQL cases skipped because the existing DSN gate is unset; no runtime coverage is claimed.
- The transient corrective fixture cardinality/layout/closed-service failures are retained
  externally and are not product RED evidence. No production expansion was needed.

Corrective command receipts are retained externally in `corrective-red-2.559i412y`,
`corrective-focused-green-3.rhnhll2z`, `corrective-targeted.d4uz0wzo`,
`corrective-sqlguard.3x9h_9xv`, `corrective-conformance.2hpyndqa`, and
`corrective-lint.yawrlgut`. All commands joined and their owned process groups were absent.

The following receipts cover the initial implementation.

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

Initial implementation checks completed sequentially on 2026-10-26:

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

- Omitted creation metadata must never reset a live canonical slot. Test strict reactivation separately.
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
