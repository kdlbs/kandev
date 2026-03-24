---
name: push-fixup
description: Commit, push, then wait for CI and CodeRabbit review, fix any issues, and push again.
---

## Context

- Current branch: !`git branch --show-current`
- Current git status: !`git status`

## Your task

Commit and push pending changes, then wait for CI and code review and fix any issues.

### Steps

**Create a todo/task for each step below and mark them as completed as you go.**

1. **Uncommitted changes:** If there are dirty or staged changes, run `/commit` first (it runs `/verify` internally).

2. **Push** the current branch:
   ```bash
   git push
   ```
   If the branch has no upstream, use `git push -u origin <branch>`.

3. **Run `/pr-fixup`** to wait for CI checks and CodeRabbit review, fix any failures or valid comments, and push.
