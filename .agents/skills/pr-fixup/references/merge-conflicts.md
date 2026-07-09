# Merge Conflicts

Use this when the PR fixup flow finds GitHub file-level conflicts, an unmerged local index, or conflict markers in tracked files.

## Detect

Inspect GitHub's mergeability state:

```bash
gh pr view <PR> --json number,url,baseRefName,headRefName,mergeable,mergeStateStatus
```

Treat `mergeable:"CONFLICTING"` or `mergeStateStatus:"DIRTY"` as an actionable merge-conflict blocker. Treat `mergeable:"UNKNOWN"` as inconclusive: wait one short cadence and query again before deciding. States such as `BEHIND`, `BLOCKED`, `UNSTABLE`, or `HAS_HOOKS` may require an update or more CI/review work, but they are not by themselves proof of file-level conflicts.

Inspect the local worktree:

```bash
git status --short
git ls-files -u
rg -n "^(<<<<<<<|=======|>>>>>>>)" --glob '!apps/node_modules/**' --glob '!node_modules/**' --glob '!dist/**' --glob '!build/**'
```

If `git ls-files -u` prints entries, or conflict markers are present in tracked source files, resolve those conflicts before fixing CI or review comments. Do not start a new merge/rebase while the index is already unmerged.

## Resolve

When GitHub reports file-level conflicts but the local index is clean:

1. Fetch the latest base branch:
   ```bash
   git fetch origin <baseRefName>
   ```
2. Prefer merging `origin/<baseRefName>` into the PR branch for conflict-fixup work:
   ```bash
   git merge --no-edit origin/<baseRefName>
   ```
   Use `git rebase origin/<baseRefName>` only when the branch already uses a rebase-style history or the user asks for it. If a rebase is used and succeeds, the push later may need `git push --force-with-lease`.
3. If conflicts appear, inspect each conflicted file, preserve the intended behavior from both sides, remove all conflict markers, and stage only the resolved files.
4. Confirm the conflict is gone before continuing:
   ```bash
   git ls-files -u
   rg -n "^(<<<<<<<|=======|>>>>>>>)" --glob '!apps/node_modules/**' --glob '!node_modules/**' --glob '!dist/**' --glob '!build/**'
   git diff --check
   ```

Do not discard unrelated user changes to make a merge/rebase easier. If unrelated dirty files block the conflict-resolution attempt, stop and ask before stashing, committing, or reverting them.
