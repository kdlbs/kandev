---
id: "02-retarget-worktree-reuse"
title: "Preserve checkout identity after base retargeting"
status: done
wave: 2
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002
acceptance_criteria:
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002.4
system_design:
  - ../../specs/tasks/system-design/additional-session-workspace-reuse.md
---

# Task 02: Preserve checkout identity after base retargeting

## Summary

Keep the existing canonical checkout selected when only the comparison base
changes. This repairs the reproduced stacked-task reuse failure without live
slug edits or state resets.

## In scope

- Regression tests through resume preparation and canonical launch admission.
- Shared canonical slot selection before inventory matching and worktree-ID reuse.
- Single and multiple repositories, primary and guest sessions, explicit branch
  selection, ambiguous slots, invalid rows, placements, and supplied-ID conflicts.
- Preserve current comparison base, canonical inventory, checkout, and provider identity.

## Out of scope

- Remote/local executor changes, Git or schema mutations, blanket slug updates,
  automatic task/session state resets, UI changes, or live repair.

## Acceptance

1. A parent-keyed canonical Worktree row remains reusable after its logical base
   changes to main, with its original worktree ID and slug and the updated base.
   Cover both resume and additional-session launch, including multiple repositories.
2. Explicit checkout selection, multiple logical or physical candidates,
   mismatched placement, conflicting requested IDs, missing/failed/deleted rows,
   and an invalid selected sibling keep their existing refusal semantics.
3. Actual manager reuse validates the selected ID/key without changing HEAD,
   index, files, inventory, task state, or provider metadata as a repair effect.

## Verification

From the repository root (resolve temporary-path aliases for Git fixtures):

```bash
export TMPDIR="$(python3 -c 'import os,tempfile; print(os.path.realpath(tempfile.gettempdir()))')"
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator/executor -run 'Retarget|CanonicalWorktreeIdentity|ReuseEnvironmentInventory|PrepareResumeRepositorySettings|MultiBranch|InventoryAdmission|WorkspaceInventory' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/worktree -run 'ReuseRequired|ReuseWorktree' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -run 'TestInventoryRepairDoesNotRunMutatingRecoveryPreflight|TestInventoryRepairRejectsProviderRestoredSettingsBeforeRepositoryAccess' -count=1)
bash scripts/lint-go-changed apps/backend/internal/orchestrator/executor/executor_worktree_reuse_identity.go apps/backend/internal/orchestrator/executor/executor_worktree_reuse_identity_test.go apps/backend/internal/orchestrator/executor/executor_environment_reuse.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_worktree_reuse_identity.go`
- `apps/backend/internal/orchestrator/executor/executor_worktree_reuse_identity_test.go`
- `apps/backend/internal/orchestrator/executor/executor_environment_reuse.go`
- `apps/backend/internal/orchestrator/executor/executor_inventory_admission.go`
- `docs/specs/tasks/requirements/additional-session-workspace-reuse.md`
- `docs/specs/tasks/system-design/additional-session-workspace-reuse.md`
- This work order and its plan.

## Dependencies

Independent of task 01; execute sequentially in this session.

## Risks

A repository-only fallback can choose another checkout. Require unambiguous
logical and canonical slot identity, preserve explicit branch selection, and
retain the existing physical admission checks. Do not trust session projections.

## Parallelism

`sequential`

## Inputs

- [Canonical reuse requirements](../../specs/tasks/requirements/additional-session-workspace-reuse.md).
- [Canonical reuse design](../../specs/tasks/system-design/additional-session-workspace-reuse.md).
- Existing resume preparation, canonical inventory, and manager reuse tests.

## Results

Completed on October 22, 2026. The regressions failed before the relevant
production correction and passed afterward. Final combined Worktree/executor
verification covered both work orders, including real Git and SQLite-backed
preflight/reuse with unchanged inventory, HEAD, file bytes, and provider metadata.

- Casing and retargeting/selection suites: passed (Worktree 19.052s,
  executor 11.861s).
- Managed clone/relocation compatibility suite: passed (50.540s) with
  canonical `TMPDIR=/private/tmp` on macOS.
- Manager required-reuse suite: passed (5.017s).
- Catalog/specification lint, 36 specification-linter tests, 63 public-doc tests,
  54 public-page validations, documentation coverage preflight, and diff checks
  passed. Changed-package Go lint is recorded in the plan after its final run.

Default macOS temporary paths exposed existing `/var` versus `/private/var`
fixture failures. Three representative failures reproduced with an overlay
restoring the original production file. Resolving temporary-path aliases made
the full compatibility scope pass; no production workaround was added for those
fixtures. No live task, database, Git metadata, runtime, or provider token changed.

## PR remediation

The first published head exposed inventory-repair receipt tests that the initial
focused scope omitted. Ordinary key preservation now requires complete recorded
physical identity (including both source fields, checkout path, and branch).
Incomplete rows retain automatic guarded repair and post-repair attestation.
Explicit repair independently keeps strict requested-key validation. The
inventory-admission, workspace-inventory, retargeting, and persisted-preflight
suite passed together in 21.127s after the correction.

The review-requested nonempty `BranchSlug` assertion was added to the multi-repo
case. A temporary Go overlay disabling propagation made it fail with old key
versus main; the real implementation passed. Missing source/common/path/branch
subcases also failed before the eligibility guard and passed afterward.

Four unrelated macOS executor fixture failures reproduced against pre-PR
production code: `/dev/fd/3` access in continuation/missing-checkout tests and
bare-repository policy in the remote-selection test. These remain baseline
limitations; Linux CI owns the full unfiltered suite.

The final full executor run passed in 94.410s with only the four baseline macOS
fixture cases excluded. The original-source baseline reproduced those cases;
none was hidden from the GitHub Linux gate.

The second failing backend shard exercised the same explicit-repair boundary at
the orchestrator layer. Its two focused recovery tests passed after the fix
(1.821s). Changed executor-package lint passed with zero issues.
