---
name: push
description: Push an already verified and committed branch. With --fixup, return control to the planner for delegated CI and review handling.
---

# Push

## Planner Entry

The planner may push a routine verified commit directly. With `--fixup`, it
keeps long monitoring on delegated `pr-poller`; delegate delivery only when it
has a material isolation or coordination benefit.

An explicitly assigned push worker pushes only the already verified and
committed branch and does not spawn other workers.

## Available skills

- **`/commit`** — Creates the artifact and hook receipt before verification.
- **`/verify`** — Mandatory post-commit gate for the exact current `HEAD`.
- **`/pr-fixup`** — Wait for CI checks and CodeRabbit, Greptile, Claude, OpenCode, and cubic review feedback, fix any failures or valid comments, and push again.

## Options

- `--fixup` — after pushing, report that the planner should begin the delegated `/pr-fixup` workflow.

> **Note:** This skill only uses `git push`. GitHub CLI dependency is indirect via `/pr-fixup`.

## Your task

Push the already committed branch to its remote.

### Steps

**Create a todo/task for each step below and mark them as completed as you go.**

1. **Uncommitted changes:** If there are dirty or staged changes, stop and tell
   the planner that a new commit and verification run are required first.

2. **Verification evidence:** Require a successful post-commit `verify` result
   whose reported `HEAD` exactly equals current `HEAD`. If the checkout or
   commit changed afterward, stop and require fresh verification.

3. **Safety check:** Verify the current branch is NOT `main` or `master`. If it is, stop and ask the user — direct pushes to the default branch should go through a PR.

4. **Push** the current branch:
   ```bash
   git push
   ```
   If the branch has no upstream, use `git push -u origin <branch>`.
   If the branch was rebased or history was rewritten, first confirm the current
   branch is not `main` or `master`, then use `git push --force-with-lease`.
   If the branch modifies `.github/workflows/*` and GitHub rejects the push with
   a message like `refusing to allow an OAuth App to create or update workflow
   ... without workflow scope`, treat it as push authentication/scope, not a code
   or branch-protection failure. Retry with an SSH remote when available, for
   example `git push git@github.com:<owner>/<repo>.git <branch>`, or tell the
   user the token needs `workflow` scope.

5. **Report** the pushed commit hash and branch.

6. **If `--fixup`:** Return the pushed branch state to the planner so it can
   coordinate `/pr-fixup`. Do not poll, fix, verify, or spawn another worker.
