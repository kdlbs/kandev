---
id: "00-baseline-transition"
title: "Move to the plugin-backed baseline"
status: done
wave: 0
depends_on: []
plan: "plan.md"
requirements: []
acceptance_criteria: []
system_design: []
---

# Task 00: Move to the plugin-backed baseline

## Summary

Preserve the approved design package on a clean branch from current
`origin/main`. Then close PR #3061 because its declarative canvas model is
superseded.

## In scope

- Fetch the latest `origin/main` and inspect the current branch and worktree.
- Run the documentation checks before the branch transition.
- Commit only the complete design package on the current local branch.
- Do not push that documentation commit to the branch for PR #3061.
- Create a new implementation branch from the latest `origin/main`.
- Transfer only the documentation commit to the new branch.
- Resolve documentation conflicts without copying production code from PR
  #3061.
- Make sure that the new branch differs from `origin/main` only in `docs/**`.
- Close PR #3061 with a comment that identifies the superseded design.
- Name the replacement ADR path and clean baseline in the closing comment.

## Out of scope

- Canvas production code, migrations, tests, generated files, or dependency
  changes.
- A force push, branch removal, or rewrite of the remote PR branch.
- Reuse of code from PR #3061. A later work order can copy one reviewed helper
  when the new design needs it.

## Acceptance

- The new branch contains the complete design package and has `origin/main` as
  an ancestor.
- The diff from `origin/main` contains only documentation files before Task 01
  starts.
- PR #3061 is closed with a comment that states why the new implementation
  replaces it.

## Verification

```bash
git merge-base --is-ancestor origin/main HEAD
git diff --name-only origin/main...HEAD
git diff --name-only origin/main...HEAD -- . ':!docs/**'
python3 scripts/lint-spec-files.py --all
git diff --check origin/main...HEAD
gh pr view 3061 --json state,url
```

The second command lists the preserved documentation package. The third
command must have no output. The pull-request state must be `CLOSED`.

## Files likely touched

- `docs/copilot-canvas-reference.md`
- `docs/decisions/2026-08-25-server-owned-declarative-canvases.md`
- `docs/decisions/2026-08-26-plugin-backed-web-app-canvases.md`
- `docs/decisions/INDEX.md`
- `docs/specs/canvases/**`
- `docs/specs/plugins/**`
- `docs/plans/plugin-backed-canvases/**`

## Dependencies

None.

## Risks

- A branch switch can lose an untracked design file.
- A broad commit can transfer superseded production code to the new branch.
- Closing the old pull request before the transfer can make recovery harder.

## Parallelism

`sequential`

## Inputs

- [Plugin-backed canvas ADR](../../decisions/2026-08-26-plugin-backed-web-app-canvases.md)
- The current worktree and PR #3061 metadata.
- The repository commit and pull-request procedures.

## Results

Completed on 2026-08-27. Created `feature/plugin-backed-canvases` from
`origin/main` at `144106c72c`, transferred the 26-file design package in
`5915850495fd5bd69f15f6361167adeedf2ee9b3`, and verified that the branch diff
contains only `docs/**`. Specification lint and `git diff --check` pass. PR
#3061 was commented and closed as superseded by
`docs/decisions/2026-08-26-plugin-backed-web-app-canvases.md`.
