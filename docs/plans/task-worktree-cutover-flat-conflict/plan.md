---
spec: docs/specs/tasks/requirements/session-delete-resource-cleanup.md
created: 2026-08-12
status: done
---

# Fix Plan: Recover Divergent Flat Worktree Cutovers

## Overview

Repair the one-time task-worktree ownership cutover for a valid legacy state.
A canonical repository row and deprecated flat fields can name different
worktrees for one repository slot. The cutover will keep the canonical row and
record the flat worktree as history.

## Root Cause

The legacy launch path updated the flat environment before it refreshed the
repository rows. For single-repository launches, it did not refresh those rows
when the launch response omitted the multi-repository list. A later worktree
materialization could therefore update only the flat worktree ID.

The cutover loads canonical repository rows before the deprecated flat fields.
It accepts stale flat path and branch data only when both sources have the same
worktree ID. If the IDs differ, `mergeFlatEnv` reports `worktree A conflicts
with B` before the slot election can classify the weaker source as history.

A throwaway repository test reproduced the reported error. The test used the
reported environment and worktree IDs with a canonical row and a divergent flat
row for the same empty branch slot. The real repository initializer returned
the reported `flat worktree fields` conflict. The throwaway test was removed.

## Backend

### Canonical source precedence

Update
`apps/backend/internal/task/repository/sqlite/worktree_ownership_normalize.go`.
When a non-deleted canonical row already owns the same repository and empty
branch slot, keep that row if the flat source has a different worktree ID.
Classify the flat source as historical evidence instead of adding a conflict.

Use the normalized task, repository, and empty branch slot for this rule.
Canonical rows from environments collapsed into the surviving task environment
are already re-homed into that normalized target and must retain precedence.
Do not suppress a flat worktree when the canonical row belongs to another task,
repository, or non-empty branch slot. Keep the current merge behavior when the
canonical row has no worktree ID.

Do not add the demoted flat ID to the authoritative worktree index. Add one
diagnostic entry to the existing post-commit demotion log. Include the
environment, repository, worktree ID, path, branch, and canonical winner.

### Inventory validation

Update
`apps/backend/internal/task/repository/sqlite/worktree_ownership_targets.go`.
Exclude a demoted flat source from the legacy inventory. This rule keeps the
merge result and inventory validation consistent.

Keep all transaction, backup, shadow-table, rollback, and replay behavior.
The migration must not change the file system or Git state.

## Tests

- **What:** A canonical row wins over divergent flat fields for the same empty
  branch slot.
  **File:**
  `apps/backend/internal/task/repository/sqlite/worktree_ownership_flat_precedence_test.go`.
  **How:** Seed the reported conflict shape. Run the real SQLite repository
  initializer. Make sure that the cutover completes and retains the canonical
  worktree.
- **What:** The migration records the demoted flat worktree only after commit.
  **File:** the same test file.
  **How:** Capture the migration logger. Make sure that a successful cutover
  includes the flat identity and path. Inject a cutover failure and make sure
  that it emits no demotion warning.
- **What:** Source precedence is limited to the exact empty branch slot.
  **File:** the same test file.
  **How:** Give the canonical row a non-empty branch slug. Make sure that the
  normalized environment keeps both repository slots.
- **What:** Duplicate canonical rows preserve precedence regardless of legacy
  row order.
  **File:** the same test file.
  **How:** Give duplicate environments the same canonical slot and divergent
  flat fields on the surviving environment. Run both canonical row orders and
  make sure that the canonical worktree remains authoritative.
- **What:** Re-homed canonical rows retain precedence over surviving flat data.
  **File:** the same test file.
  **How:** Put flat fields on the surviving environment and the only canonical
  row on a collapsed older environment. Make sure that the cutover keeps the
  canonical worktree.
- **What:** PostgreSQL uses the same precedence and transaction behavior.
  **File:**
  `apps/backend/internal/task/repository/sqlite/worktree_ownership_postgres_test.go`.
  **How:** Add an environment-gated PostgreSQL fixture for the reported state.
- **What:** Existing cutover election, conflict, rollback, and replay behavior
  remains valid.
  **File:** the existing SQLite repository test package.
  **How:** Run all cutover tests and the complete repository package.

## Verification Results

- `cd apps/backend && go test ./internal/task/repository/sqlite -run 'TestCutover_(CanonicalRepoSupersedesDivergentFlatOwner|PreservesDivergentFlatOutsideCanonicalSlot|DuplicateCanonicalRowsRetainSurvivingEnvironmentPrecedence|RehomedCanonicalRowSupersedesSurvivingFlatOwner|DoesNotLogRolledBackFlatDemotion)|TestCutoverPostgres_CanonicalRepoSupersedesDivergentFlatOwner' -count=1` — 7 SQLite tests passed; the PostgreSQL test is environment-gated and skipped because `KANDEV_TEST_POSTGRES_DSN` is unset.
- `cd apps/backend && go test ./internal/task/repository/sqlite -run 'TestCutover' -count=1` — 54 tests passed.
- `cd apps/backend && go test ./internal/task/repository/sqlite -count=1` — 421 tests passed.

## Implementation Waves And Parallel Candidates

Wave 1, sequential:

- [x] [task-01-normalize-divergent-flat-owner](task-01-normalize-divergent-flat-owner.md)

No task is parallel-safe. The normalization state, inventory, logs, and tests
share one migration boundary.

## Risks

- A broad precedence rule can hide a worktree from another repository slot.
  The rule must match the environment, repository, and empty branch slug.
- Merge and inventory rules can diverge. Both paths must use the same demotion
  state.
- A warning before commit can describe a rollback as a completed migration.
  Reuse the existing post-commit demotion logger.

## Out of Scope

- Manual edits to an affected database.
- File-system or Git reconciliation during startup.
- Changes to task or session behavior after the one-time cutover.
- Public recovery documentation. The backup and rollback procedure is
  unchanged.
