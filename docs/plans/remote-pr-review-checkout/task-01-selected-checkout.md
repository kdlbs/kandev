---
id: "01-selected-checkout"
title: "Honor selected remote checkout"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-REPOSITORY-BRANCH-002
acceptance_criteria:
  - AC-EXECUTORS-REPOSITORY-BRANCH-002.1
  - AC-EXECUTORS-REPOSITORY-BRANCH-002.2
  - AC-EXECUTORS-REPOSITORY-BRANCH-002.3
  - AC-EXECUTORS-REPOSITORY-BRANCH-002.4
  - AC-EXECUTORS-REPOSITORY-BRANCH-002.5
system_design:
  - ../../specs/executors/system-design/repository-branch-resolution.md
---

# Task 01: Honor selected remote checkout

## Summary

Materialize the selected PR or branch in fresh remote workspaces using strict managed preparation.

## Scope

Typed selection propagation, non-worktree branch selection, shared checkout fragment, and focused regression tests.
Preserve base comparison, generated branches, resume state, and existing contribution bindings.
No UI changes, live task mutation, deployment, persistence migration, or new push capability.

## Acceptance

1. A fork PR with no matching origin branch starts on the selected local branch at the fetched PR head.
2. Explicit checkout failures prevent agent startup without fallback; shell input remains literal data.
3. Generated tasks, existing workspaces, and contribution-bound launches retain their current behavior.

## Sequence and files

Read `/tdd` and backend test guidance. Mark this work order in progress only after an explicit implementation request.
Add `TestSpritesPrepareScript_SelectedCheckout` in a new `selected_checkout_prepare_test.go` beside lifecycle code.
Build its metadata from a typed launch request, not a manually completed metadata map.
Run RED before changing production code; assert the observed wrong branch and commit.

Likely files under `apps/backend/internal/agent/runtime/lifecycle/`:

- `env_preparer_docker.go` and its shared non-worktree branch tests.
- `manager_launch.go`, metadata constants, and launch-metadata tests.
- `default_scripts.go` and managed-checkout tests.
- `executor_sprites_operations.go` and equivalent Docker, SSH, and Kubernetes script resolvers.
- New `selected_checkout_prepare_test.go` and existing `repository_branch_prepare_test.go` fixtures.

Use `internal/scriptengine/` for shared data quoting and binding-aware helpers if required by current ownership.
Do not duplicate the checkout logic across executors.
Cover both current fetch and saved clone templates, missing refs, plain branches, fork and same-repository PRs, injection, conflicting local state, generated branches, and resume.
Add a regression proving an existing contribution binding does not run a competing selected-checkout path.
Update `docs/public/git-operations.md` through `/docs-maintainer` after implementation.

## Verification

Run from the repository root; prefix shell commands with the configured RTK wrapper.

```bash
(cd apps/backend && go test ./internal/scriptengine -count=1)
(cd apps/backend && go test ./internal/agent/runtime/lifecycle -run 'Test.*(SelectedCheckout|NonWorktree|BuildLaunchMetadata|SpritesPrepareScript|ResolvePrepareScript|BranchCheckoutPostlude|BranchNameCommandInjection|RemoteContribution)' -count=1)
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Name new tests to match the selection above. Confirm that each intended test executes rather than accepting an empty match.
The real-Git regression is end-to-end preparation evidence; no Sprites credentials or live user task is required.

## Dependencies and results

Sequential. Retain the earlier origin-reference normalization fix in this worktree.
Implementation completed on 2026-09-10.

## Results

- RED: current and saved-clone templates selected `feature/hello-abcdef` instead of the requested PR branch.
- RED: missing refs and conflicting local branches reported success. Recreated compute used the base commit instead of the PR head.
- RED: a competing selected-checkout fragment blocked a valid contribution binding on a nonexistent review ref.
- GREEN: the exact lifecycle command in Verification passed (10.644s).
  Its test selection includes all new regressions and existing launch-metadata, resolver, contribution, and shell-injection tests.
- `go test ./internal/scriptengine -count=1`: passed (0.090s).
- Specification validator tests: 36 passed. All specification files passed validation.
- Public-doc validator tests: 62 passed. All 46 published pages passed validation.
- `git diff --check`: passed.

Fixup follow-up on 2026-09-10 moved explicit checkout before repository setup for built-in remote templates, made retained-checkout detection ownership-safe, and documented the negative PR-number fail-closed guard.
The focused lifecycle selection suite passed after these changes, including setup-order and simulated different-owner regressions.

The new `selected_checkout.go` owns typed metadata projection and the shared strict checkout wrapper.
Tests live in `selected_checkout_prepare_test.go` and `selected_checkout_contract_test.go`.
The real-Git fixtures cover shallow saved clones, fork and same-repository PRs, explicit branches, missing refs, and recreated compute.
Resume coverage preserves a local commit, an edited tracked file, an untracked file, and a user-selected branch.
Shared resolver coverage executes Sprites, Docker, SSH, and Kubernetes scripts.
Contribution coverage executes the existing Sprites contribution path and checks its source remote and branch.

Public docs updated: `docs/public/git-operations.md` (reference).
The earlier origin-reference correction remains intact.
No live task mutation, deployment, commit, push, PR creation, or delegation occurred.
