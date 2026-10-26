---
id: "01-preserve-checkout-identity"
title: "Preserve checkout identity on resume"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
acceptance_criteria:
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.1
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.2
system_design:
  - ../../specs/tasks/system-design/additional-session-workspace-reuse.md
---

# Task 01: Preserve checkout identity on resume

## Outcome

Select the existing checkout inventory during clone-URL resume when base and
checkout branches differ. Preserve attach-only preparation.

## Scope

- Carry checkout identity only for required workspace reuse on clone-URL executors
  with one matching attachment. Fresh materialization keeps its base identity.
- Keep missing, ambiguous, and non-clone attachment handling unchanged.
- Test resume preparation through canonical inventory admission and reuse.
- Keep the production comment limited to the invariant it preserves.

## Owned files

- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/executor_resume_checkout_identity_test.go`
- `apps/backend/internal/orchestrator/executor/executor_resume_checkout_reuse_test.go`
- `apps/backend/internal/orchestrator/executor/executor_environment_reuse.go`
- `apps/backend/internal/orchestrator/executor/executor_clone_url_reuse_identity.go`
- `apps/backend/internal/orchestrator/executor/executor_resume_materialization_reuse_test.go`
- This work order and its sibling plan.

## Acceptance

1. Docker, remote Docker, and Sprites resume preparation succeeds against the
   existing checkout row when the base is `release/x` and checkout is `feature/y`.
2. The selected environment, workspace path, and checkout inventory remain
   unchanged. Resume retains required reuse and requests neither clone nor checkout.
3. Existing tests retain the no-checkout, ambiguous-attachment, unmatched-repository,
   and local-executor behavior.

## Dependencies and exclusions

No dependency. No UI, persistence, configuration, executor provisioning, or
workspace-repair policy changes are in scope.

## Verification

From `apps/backend`:

```bash
go test -trimpath -tags fts5 -run 'Resume|Reuse|Inventory' ./internal/orchestrator/executor/ -count=1
go test -trimpath -race -tags fts5 -run '^TestPrepareResumeRepositorySettings_ReusesCheckoutBranchInventory$' ./internal/orchestrator/executor/ -count=1
```

Disable only the new identity assignment through a temporary Go overlay. Run
the new regression and require the original inventory mismatch. Restore the
normal source inputs and require a pass.

From the repository root, validate this changed work order and its references
with `.github/scripts/pr-docs.cjs`'s `validateCoverage`, check diff whitespace,
and run normal commit hooks.

## Risks

The branch identifier is selection input. It must not weaken the canonical
inventory guard or request a new checkout.

## Results

- GREEN: the focused resume, reuse, and inventory suite passed 372 tests and
  subtests, with one existing case-insensitive-filesystem skip.
- GREEN: the new resume regression passed with the race detector for all three
  executor types.
- RED mutation proof: removing only the identity assignment through a temporary
  Go overlay made all three cases fail with the original `release-x` inventory
  mismatch. The overlay changed no persisted source files.
- GREEN: local documentation coverage and diff whitespace checks passed.
- Hosted CI and current-head reviews remain externally pending until the
  delivered commit completes them.

### Review remediation

A non-reuse regression initially failed because fresh materialization published
`feature-y` identity while preparing `release/x`. The assignment now requires
`WorkspaceReuseRequired`, and the regression passes with `release-x` identity.
The existing request-builder test now declares required reuse explicitly.
Focused race checks passed after this correction. Hosted CI and a fresh Claude
review remain pending for the new commit.

### Claude readability suggestion

Review of head 69fe5a9c reported zero blockers and one suggestion. Reuse one
local `requiresCloneURL` value for the base-branch and checkout-identity guards.
This removes the repeated capability check and shortens both conditions.
The existing branch and executor regressions verify the preserved behavior.

### Fresh materialization followed by reuse

CodeRabbit identified a second valid sequence: fresh resume materializes the
base branch, then a later resume requests the configured checkout identity.
The stateful regression reproduced the rejection for all three clone-URL
executor types. It uses the real inventory projection between preparation calls.

When the checkout request has no match, ordinary clone-URL reuse can select
one active canonical row that exactly matches its base identity. The same
inventory query supplies that row and the final validation. This fallback is
limited to one top-level repository owned by the requesting task and executor.
Explicit checkouts, inherited environments, inventory repair, and worktree
requests retain their selection rules. Foreign branches, inactive rows,
ambiguous rows, foreign environment IDs, and wrong placements remain refused.

The expanded focused suite passed 394 tests/subtests with one filesystem skip.
The fresh-materialization, checkout-identity, and request-builder regressions
also passed with the race detector. The new sequence failed before the fallback
for Docker, remote Docker, and Sprites, while the negative inventory cases stayed
rejected. Fresh hosted checks and current-head reviews remain delivery gates.
