---
name: push
description: Push a committed branch whose task-defined checks passed. With --fixup, continue with CI and review handling in the primary conversation.
---

# Push

## Planner Entry

Push the verified commit directly in the primary conversation. With `--fixup`,
continue with `/pr-fixup` in that same conversation; do not delegate polling or
delivery.

## Available skills

- **`/commit`** — Creates the artifact after task-defined checks pass.
- **`/pr-fixup`** — Wait for CI checks and CodeRabbit, Greptile, Claude, OpenCode, and cubic review feedback, fix any failures or valid comments, and push again.

## Options

- `--fixup` — after pushing, begin `/pr-fixup` in the same conversation.

> **Note:** This skill only uses `git push`. GitHub CLI dependency is indirect via `/pr-fixup`.

## Your task

Push the already committed branch to its remote.

### Steps

**Create a todo/task for each step below and mark them as completed as you go.**

1. **Uncommitted changes:** If there are dirty or staged changes, stop: a new
   commit and the affected task checks are required first.

2. **Task-check evidence:** Require the task-defined unit, integration, and E2E
   checks affected by this commit to pass. If the checkout or commit changed
   afterward, rerun only the affected task checks.

3. **Safety check:** Verify the current branch is NOT `main` or `master`. If it is, stop and ask the user — direct pushes to the default branch should go through a PR.

4. **Push** the current branch:
   ```bash
   git push
   ```
   If the branch has no upstream, use `git push -u origin HEAD` rather than
   transcribing the branch name. Then verify
   `git rev-parse HEAD` equals `git rev-parse '@{upstream}'`, and report the
   branch from `git branch --show-current`.
   If the branch was rebased or history was rewritten, first confirm the current
   branch is not `main` or `master`, then use `git push --force-with-lease`.
   If the branch modifies `.github/workflows/*` and GitHub rejects the push with
   a message like `refusing to allow an OAuth App to create or update workflow
   ... without workflow scope`, treat it as push authentication/scope, not a code
   or branch-protection failure. Retry with an SSH remote when available, for
   example `git push git@github.com:<owner>/<repo>.git <branch>`, or tell the
   user the token needs `workflow` scope.

5. **Report** the pushed commit hash and branch.

6. **If `--fixup`:** Continue with `/pr-fixup` in this conversation.
