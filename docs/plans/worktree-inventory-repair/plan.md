---
created: 2026-09-28
status: complete
requirements:
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-001
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-002
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-003
system_design:
  - ../../specs/tasks/system-design/worktree-inventory-repair.md
legacy_specs: []
---

# Implementation plan: Repair legacy worktree inventory

## Outcome

Provide an explicit repair for legacy worktree inventory that disagrees with
healthy Git checkouts. Preserve the ownership and audited-commit checks that
prevent unsafe deletion and cross-task reuse. Correct absent-execution logging
without suppressing real runtime failures.

## Reproduced failures

| Failure | Cause | Repair |
| --- | --- | --- |
| Cleanup reports a branch advanced from its audited commit | Inventory names a different branch from the checkout's attached branch. | Correct the exact saved branch after independently validating Git registration and commit; preserve both refs. |
| Archive source manifest has incomplete worktree identity | A repository ID is absent from the canonical slot and durable cleanup snapshot, which also lacks HEAD evidence. | Repair the slot and create a linked successor with current evidence; keep the predecessor snapshot unchanged. |
| Shared environment resume fails ownership validation | A selected repository checkout is under the child's marked root while the environment projects the parent's root. | Explicitly move the checkout into its canonical shared root and update exact workspace references, preserving both markers. |
| Cleanup logs a failed stop for an already-absent execution | Warning precedes typed absence classification. | Classify typed lifecycle/public-runtime absence before logging WARN. |

Historical corruption is not attributed to a particular migration without
proof. Ordinary launch and cleanup never invoke inventory repair automatically.
The package supports SQLite host worktrees; unknown liveness and unsupported
stores or executors refuse application. Live installation maintenance and
deployment are outside this code-change delivery.

## Work orders

Execute sequentially; there is no delegation authorization.

- [x] [01: Preview and validate exact inventory repairs](task-01-preview-repair.md)
- [x] [02: Apply and recover journaled inventory repairs](task-02-apply-repair.md)
- [x] [03: Classify absent-execution diagnostics](task-03-stop-diagnostics.md)
- [x] [04: Open the repair database at native Windows paths](task-04-windows-database-path.md)

## Verification mapping

| Criteria | Required implementation evidence |
| --- | --- |
| 001.1-.2, 002.1 | New `internal/task/inventoryrepair/inspect_test.go`: exact branch mismatch, missing repository identity, registration/backlink mismatch, duplicate candidate, unknown liveness, remote/store rejection, read-only preview |
| 001.3-.5 | New `apply_test.go` and `recovery_test.go`: preserved content/index/refs, move and SQL failpoints, busy backend, stale compare-and-set, unresolved-journal startup gate |
| 002.2-.3 | New `cleanup_test.go`: predecessor snapshot unchanged, missing legacy OID, successor idempotency, already-removed sibling, changed archive generation, dirty and borrowed worktrees |
| 003.1-.2 | New `executor_stop_diagnostics_test.go`: typed runtime absence has no WARN, real failure remains WARN and cannot unlock destructive cleanup |
| 001.6 | New `internal/task/inventoryrepair/read_test.go`: native absolute database path with a space, `#`, `%`, or (POSIX only) `?` in read-only and read-write mode; no other file created; journal backup check |

The numeric prefixes above refer to `AC-TASKS-WORKTREE-INVENTORY-REPAIR-*`.
Work orders provide exact commands. Existing tests remain the baseline for
ownership and source-manifest refusal behavior.

## End-to-end evidence

Task 02 includes real SQLite and Git fixtures through the cleanup worker and
the existing attach-only worktree admission boundary. Runtime execution is not
started by these fixtures; the full executor package is tested separately. No rendered UI changes or browser layout tests are required.

## Risks and operational boundary

- Restarting the active backend interrupts this session and other work. Prepare
  and test everything before arranging a separate maintenance boundary.
- A clean checkout can still contain ignored files. Relocation must preserve
  them; normal archive policy is a separate later decision.
- The old cleanup job cannot be fixed by updating only its live inventory row
  or by silently replacing its audit hash. Preserve old evidence and explicitly
  supersede its incomplete attempt.
- Existing archived-worktree and metadata-recovery packages retain their own
  statuses. This package references their contracts without claiming that their
  outstanding validation or deployment work is complete.

## Verification results

Implementation validation on 2026-09-28:

- Race tests passed for `internal/task/inventoryrepair`, the maintenance CLI,
  `internal/worktree`, `internal/task/service`, `internal/orchestrator/executor`,
  and `internal/backendapp/ownershiplock`.
- The focused backendapp `InventoryRepair` startup tests and
  `internal/persistence/storeconformance` race tests passed.
- Scoped Go lint, SQL guard, catalog validation (321 decisions / 1,224 specs),
  specification lint, 47-page public-doc validation, changed/new-file whitespace,
  and actual source-change documentation-coverage preflight passed.
- A real hotfix process refused an unresolved repair fence before creating a
  database; the standalone command built successfully.

Implementation work orders 01-03 are complete. Live installation repair
is a separate operator action and was not performed as part of this change.

Review validation on 2026-09-29:

- Reproduced database-file aliases bypassing startup fences and a different
  installation UID hiding a live checkout consumer before correcting both.
- Startup uses its already locked canonical paths for fence checks. Process
  inspection covers every UID, refuses unknown liveness, and parses exact
  status fields when excluding zombies and kernel threads.
- Repair, CLI, ownership-lock, and focused startup race tests passed; regressions
  cover database and parent-directory aliases, lock release, foreign-owner
  process census, and misleading process names. Specification lint passed.
- Current-head CI, final review disposition, and merge validation remain external
  PR gates; earlier validation does not substitute for those checks.
