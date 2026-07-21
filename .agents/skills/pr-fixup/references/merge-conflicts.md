# Merge Conflicts

Use this when the PR fixup flow finds GitHub file-level conflicts, an unmerged local index, or conflict markers in tracked files.

## Detect

Inspect GitHub's mergeability state:

```bash
gh pr view <PR> --json number,url,baseRefName,headRefName,mergeable,mergeStateStatus
```

Treat `mergeable:"CONFLICTING"` or `mergeStateStatus:"DIRTY"` as an actionable merge-conflict blocker. Treat `mergeable:"UNKNOWN"` as inconclusive: wait one short cadence and query again before deciding. States such as `BEHIND`, `BLOCKED`, `UNSTABLE`, or `HAS_HOOKS` may require an update or more CI/review work, but they are not by themselves proof of file-level conflicts.

Always use the freshly queried `baseRefName`. GitHub can retarget a stacked child PR after its parent merges, so neither the Git upstream nor a base branch remembered from an earlier fixup round is authoritative.

Inspect the local worktree:

```bash
git status --short
git ls-files -u
git grep -n -E '^(<<<<<<<|>>>>>>>)'
```

If `git ls-files -u` prints entries, or conflict markers are present in tracked source files, resolve those conflicts before fixing CI or review comments. `git grep` scans only tracked files, and intentionally checks only the unambiguous start/end markers so Markdown setext headings do not create false positives. Do not start a new merge/rebase while the index is already unmerged.

## Resolve

### When GitHub reports conflicts and the local index is clean

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
   git grep -n -E '^(<<<<<<<|>>>>>>>)'
   git diff --check
   git diff --cached --check
   git diff --stat origin/<baseRefName>
   ```

   Confirm that the final diff against the current GitHub base contains only the PR's intended delta. A clean index is insufficient if a retargeted stacked PR accidentally retains its merged parent's changes.

### When the local index is already unmerged

If `git ls-files -u` shows entries, a previous merge or rebase was left incomplete. Do not start another merge. Instead:

1. Inspect each conflicted file:
   ```bash
   git diff --diff-filter=U
   ```
2. Resolve conflict markers manually, preserving the intended behavior from both sides.
3. Stage each resolved file:
   ```bash
   git add <file>
   ```
4. Confirm the conflict is gone before continuing:
   ```bash
   git ls-files -u
   git grep -n -E '^(<<<<<<<|>>>>>>>)'
   git diff --check
   git diff --cached --check
   ```
5. Complete the interrupted operation:
   ```bash
   git commit
   # or, if mid-rebase:
   git rebase --continue
   ```

Do not discard unrelated user changes to make a merge/rebase easier. If unrelated dirty files block the conflict-resolution attempt, stop and ask before stashing, committing, or reverting them.

## Push after rebasing

Capture the remote branch SHA before rebasing when possible. After a successful rebase, prefer an exact force lease:

```bash
git push \
  --force-with-lease=refs/heads/<branch>:<expected-remote-sha> \
  origin HEAD:refs/heads/<branch>
```

An intervening fetch can update the remote-tracking ref consulted by generic `--force-with-lease`. Use the generic form only when the prior remote SHA was not captured, and never use an unconditional force push.
