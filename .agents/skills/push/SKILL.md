---
name: push
description: Commit and push to the current branch. Use --fixup to also wait for CI/CodeRabbit and fix issues.
---

# Push

## Available skills

- **`/commit`** — Stage and commit changes using Conventional Commits. Runs `/verify` internally.
- **`/pr-fixup`** — Wait for CI checks and CodeRabbit review, fix any failures, and push again.

## Context

- Current branch: !`git branch --show-current`
- Current git status: !`git status`

## Options

- `--fixup` — after pushing, run `/pr-fixup` to wait for CI and CodeRabbit review, fix issues, and push again.

> **Note:** This skill only uses `git push`. GitHub CLI dependency is indirect via `/pr-fixup`.

## Your task

Commit any pending changes and push to the remote branch.

### Steps

**Create a todo/task for each step below and mark them as completed as you go.**

1. **Uncommitted changes:** If there are dirty or staged changes, run `/commit` first (it runs `/verify` internally).

2. **Safety check:** Verify the current branch is NOT `main` or `master`. If it is, stop and ask the user — direct pushes to the default branch should go through a PR.

3. **Push** the current branch:
   ```bash
   git push
   ```
   If the branch has no upstream, use `git push -u origin <branch>`.

4. **Report** the pushed commit hash and branch.

5. **If `--fixup`:** Run `/pr-fixup` to wait for CI checks and CodeRabbit review, fix any failures or valid comments, and push.
