---
name: push
description: Commit and push to the current branch. Use when there's an existing PR or remote branch to update.
---

## Context

- Current branch: !`git branch --show-current`
- Current git status: !`git status`

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
