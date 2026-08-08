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

> **Note:** This skill normally uses `git push`. It uses `gh pr view` before a
> no-upstream fallback so a checked-out fork PR is not accidentally pushed to
> its base repository.

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
   If the branch has no upstream, first look up the current branch's PR. Treat
   an unavailable or ambiguous lookup as a stop condition; only a confirmed
   absence of a PR permits the ordinary `origin` fallback. For a confirmed PR,
   require its `headRefName` to match the checked-out local branch, then inspect
   its delivery target:
   ```bash
   gh pr view --json isCrossRepository,headRepositoryOwner,headRepository,headRefName,headRefOid,maintainerCanModify
   ```
   For a cross-repository PR, the PR head owner can push its own fork directly.
   When acting as a base-repository maintainer on somebody else's fork,
   `maintainerCanModify` must be `true`; do not treat that field as a universal
   fork-owner gate. Push the exact current commit to the reported head owner,
   repository, and ref. Before using the HTTPS URL, configure Git to use the
   authenticated `gh` credential helper:
   ```bash
   gh auth setup-git
   git push "https://github.com/<head-owner>/<head-repository>.git" "HEAD:refs/heads/<head-ref>"
   ```
   If that push fails with `git repository does not match any credential lease
   scope`, use a one-shot HTTPS Basic auth header instead of changing the
   remote or persisting credentials:
   ```bash
   token=$(gh auth token)
   auth=$(printf 'x-access-token:%s' "$token" | base64 | tr -d '\n')
   git -c credential.helper= -c "http.extraheader=Authorization: Basic $auth" push "https://github.com/<head-owner>/<head-repository>.git" "HEAD:refs/heads/<head-ref>"
   ```
   Never embed the token in a remote URL or persist it. Re-fetch the PR and
   require its `headRefOid` to equal local `HEAD` afterward.
   If HTTPS still rejects a contributor-fork push and SSH authentication is
   available, verify the exact target with
   `git ls-remote git@github.com:<head-owner>/<head-repository>.git`, then retry
   the same exact ref over SSH (using the exact remote-OID lease if history was
   rewritten). Re-fetch and verify the PR head OID again. Do not substitute a
   local `fork` remote or an inferred repository.
   Do not use a conveniently named local `fork`/`contributor` remote: linked
   worktrees share remote configuration, so it can refer to another task.
   Re-fetch the PR and require `headRefOid` to equal local `HEAD`.

   Only for a non-PR or same-repository PR, use `git push -u origin HEAD`
   rather than transcribing the branch name. Then verify
   `git rev-parse HEAD` equals `git rev-parse '@{upstream}'`, and report the
   branch from `git branch --show-current`.
   If the branch was rebased or history was rewritten, first confirm the current
   branch is not `main` or `master`, then use the exact remote-OID lease
   `git push --force-with-lease=refs/heads/<branch>:<captured-remote-sha>`.
   Use generic `--force-with-lease` only when no remote OID was captured; never
   use an unconditional force push.
   If the branch modifies `.github/workflows/*` and GitHub rejects the push with
   a message like `refusing to allow an OAuth App to create or update workflow
   ... without workflow scope`, treat it as push authentication/scope, not a code
   or branch-protection failure. Retry with an SSH remote when available, for
   example `git push git@github.com:<owner>/<repo>.git <branch>`, or tell the
   user the token needs `workflow` scope.

5. **Report** the pushed commit hash and branch.

6. **If `--fixup`:** Continue with `/pr-fixup` in this conversation.
