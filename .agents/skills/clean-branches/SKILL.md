---
name: clean-branches
description: Delete local branches whose remote has been deleted ([gone]), including their worktrees. Use for branch cleanup after merging PRs.
---

# Clean Branches

Remove local branches that have been deleted on the remote (marked `[gone]`), including any associated git worktrees.

## Steps

1. **Fetch and prune remote refs:**
   ```bash
   git fetch --prune
   ```

2. **List branches to identify [gone] status:**
   ```bash
   git branch -v
   ```
   Branches with a `+` prefix have associated worktrees that must be removed before deletion.

3. **List worktrees for reference:**
   ```bash
   git worktree list
   ```

4. **Remove worktrees and delete [gone] branches:**
   ```bash
   git branch -v | grep '\[gone\]' | sed 's/^[+* ]//' | awk '{print $1}' | while read branch; do
     echo "Processing branch: $branch"
     worktree=$(git worktree list | grep "\\[$branch\\]" | awk '{print $1}')
     if [ ! -z "$worktree" ] && [ "$worktree" != "$(git rev-parse --show-toplevel)" ]; then
       echo "  Removing worktree: $worktree"
       git worktree remove --force "$worktree"
     fi
     echo "  Deleting branch: $branch"
     git branch -D "$branch"
   done
   ```

5. **Report** which branches and worktrees were removed. If none were `[gone]`, say no cleanup was needed.
