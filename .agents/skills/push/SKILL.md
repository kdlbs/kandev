---
name: push
description: Commit and push to the current branch. Use --fixup to also wait for CI/CodeRabbit and fix issues.
---

## Context

- Current branch: !`git branch --show-current`
- Current git status: !`git status`

## Options

- `--fixup` — after pushing, run `/pr-fixup` to wait for CI and CodeRabbit review, fix issues, and push again.

## Your task

Commit any pending changes and push to the remote branch.

### Steps

**Create a todo/task for each step below and mark them as completed as you go.**

1. **Uncommitted changes:** If there are dirty or staged changes, run `/commit` first (it runs `/verify` internally).

2. **Push** the current branch:
   ```bash
   git push
   ```
   If the branch has no upstream, use `git push -u origin <branch>`.

3. **Report** the pushed commit hash and branch.

4. **If `--fixup`:** Run `/pr-fixup` to wait for CI checks and CodeRabbit review, fix any failures or valid comments, and push.
