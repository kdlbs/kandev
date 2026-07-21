---
name: push
description: Push an already verified and committed branch. With --fixup, return control to the planner for delegated CI and review handling.
---

# Push

## Planner Entry

Load `/planner-orchestration`. The user-started primary session delegates any
required verification and commit first, then assigns the push to an
`implementer` worker. With `--fixup`, the planner coordinates `/pr-fixup`
workers after the push. It does not run Git or GitHub commands directly.

An explicitly assigned push worker pushes only the already verified and
committed branch and does not spawn other workers.

## Available skills

- **`/commit`** — Planner-side prerequisite for verified, committed changes.
- **`/pr-fixup`** — Wait for CI checks and CodeRabbit, Greptile, Claude, OpenCode, and cubic review feedback, fix any failures or valid comments, and push again.

## Options

- `--fixup` — after pushing, report that the planner should begin the delegated `/pr-fixup` workflow.

> **Note:** This skill only uses `git push`. GitHub CLI dependency is indirect via `/pr-fixup`.

## Your task

Push the already committed branch to its remote.

### Steps

**Create a todo/task for each step below and mark them as completed as you go.**

1. **Uncommitted changes:** If there are dirty or staged changes, stop and tell
   the planner that verification and commit assignments are required first.

2. **Safety check:** Verify the current branch is NOT `main` or `master`. If it is, stop and ask the user — direct pushes to the default branch should go through a PR.

3. **Push** the current branch:
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

4. **Report** the pushed commit hash and branch.

5. **If `--fixup`:** Return the pushed branch state to the planner so it can
   coordinate `/pr-fixup`. Do not poll, fix, verify, or spawn another worker.
