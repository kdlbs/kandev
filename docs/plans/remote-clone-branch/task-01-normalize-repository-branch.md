---
id: "01-normalize-repository-branch"
title: "Normalize repository branch placeholders"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-REPOSITORY-BRANCH-001
acceptance_criteria:
  - AC-EXECUTORS-REPOSITORY-BRANCH-001.1
  - AC-EXECUTORS-REPOSITORY-BRANCH-001.2
  - AC-EXECUTORS-REPOSITORY-BRANCH-001.3
  - AC-EXECUTORS-REPOSITORY-BRANCH-001.4
  - AC-EXECUTORS-REPOSITORY-BRANCH-001.5
system_design:
  - ../../specs/executors/system-design/repository-branch-resolution.md
---

# Task 01: Normalize repository branch placeholders

## Summary

Convert origin base references into upstream branch names at `RepositoryProvider`.
Prove successful preparation through real Git and preserve the existing shell quoting contract.

## In scope

- One-prefix conversion after metadata selection and before shell quoting.
- Provider cases: `main`, `feature/login`, `origin/main`, `origin/feature/login`, `refs/remotes/origin/main`, and `refs/heads/main`.
- Preservation cases: `upstream/main`, `refs/heads/origin/topic`, `origin/origin/topic`, empty input, and quoted literal data.
- Both metadata keys, base precedence, unchanged metadata, and unchanged worktree references.
- Current Sprites preparation and a saved clone template against isolated Git repositories.
- A short public Git operations explanation after implementation.

## Out of scope

Live task mutation, deployment, automatic retry, new remote selection, persistence changes, and UI changes.

## Acceptance

1. Provider and real-Git regressions fail for the observed branch mismatch before the correction and pass afterward.
2. The resulting checkout has the intended base commit and task branch for both script forms.
3. Missing branches still fail; metadata, worktree references, and shell safety remain intact.

## Implementation sequence

Read `/tdd` and its backend test guidance.
Add `TestRepositoryProvider_NormalizesOriginBranch` and run it before the production change.
Add the lifecycle regressions in a new test file, using existing default-script fixtures.
Use temporary directories and neutralize unrelated agent installation without replacing the repository preparation commands.
Implement the private conversion helper and run the commands below.
Update this work order, the plan, and the paired design status only after the checks pass.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test ./internal/scriptengine -count=1)
(cd apps/backend && go test ./internal/agent/runtime/lifecycle -run 'Test(SpritesPrepareScript_|SpritesResolvePrepareScript|DefaultPrepareScript_|KandevBranchCheckoutPostlude_|BranchNameCommandInjection_Regression)' -count=1)
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

- `apps/backend/internal/scriptengine/providers.go`
- `apps/backend/internal/scriptengine/repository_branch.go` (new)
- `apps/backend/internal/scriptengine/providers_branch_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/repository_branch_prepare_test.go` (new)
- `docs/public/git-operations.md`
- This work order, its plan, and the paired design.

## Dependencies

None.

## Risks

Repeated prefix removal corrupts literal branch names. Preserve one-prefix semantics.
Custom scripts share the placeholder, including local setup and cleanup scripts.
Do not change `WorktreeProvider` to compensate for that shared use.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/repository-branch-resolution.md)
- [System design](../../specs/executors/system-design/repository-branch-resolution.md)
- `apps/backend/internal/scriptengine/providers_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/default_scripts_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_sprites_operations.go`

## Results

Completed on 2026-09-10.

- RED: the provider regression returned `'origin/main'` instead of `'main'`.
- RED: the real-Git tests failed on the requested origin reference in both current fetch and saved clone scripts.
- GREEN: the complete scriptengine package passed (0.070s).
- GREEN: the specified lifecycle test selection passed (2.019s), including all new preparation and existing injection regressions.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `node --test scripts/validate-public-docs.test.mjs`: 62 tests passed.
- `node scripts/validate-public-docs.mjs`: 46 published pages validated.
- `git diff --check`: passed.

The implementation matches the paired design. No live Sprites task was relaunched and no deployment occurred.
