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
   Never add `--no-verify` to this merge or to the commit that completes it unless
   the user explicitly authorizes bypassing hooks. If the merge commit fails or
   exits ambiguously, inspect and report the hook result; do not bypass it just
   to finish the conflict resolution.
3. If conflicts appear, inspect each conflicted file, preserve the intended behavior from both sides, remove all conflict markers, and stage only the resolved files. When the index has conflict stages, inspect the competing versions with `git show :2:<path>`, `git show :3:<path>`, and `git log -- <path>` (or compare the merge-base, current branch, and base branch versions when stages are unavailable). Do not choose ours/theirs merely to remove markers; preserve each side's behavioral invariant, compare analogous provider implementations and their tests, then run the focused test for the conflicted feature.
4. Confirm the conflict is gone before continuing:
   ```bash
   git ls-files -u
   git grep -n -E '^(<<<<<<<|>>>>>>>)'
   git diff --check
   git diff --cached --check
   git diff --stat origin/<baseRefName>
   ```

   Confirm that the final diff against the current GitHub base contains only the PR's intended delta. A clean index is insufficient if a retargeted stacked PR accidentally retains its merged parent's changes.
   After the operation completes, inspect each resolved path with
   `git diff -- origin/<baseRefName>...HEAD -- <resolved-file>` and reread the
   complete surrounding comments and logic; this catches a duplicate-hunk
   resolution that silently drops an explanatory line.

   For structured files (especially JSON catalogs), validate syntax and duplicate
   keys before continuing. JSON's default parser may silently keep the last
   duplicate key; use an `object_pairs_hook` that raises on duplicates, then run
   the affected formatter/test and `git diff --check`.

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
   git commit --no-edit
   # or, if mid-rebase:
   git rebase --continue
   ```

   Use the normal hooks and capture the full receipt. A current-base merge may
   stage many incoming files, making status look like hundreds of changes; do
   not reset or broadly unstage them. Resolve only `UU` paths, then inspect
   `git diff --stat origin/<baseRefName>` and `git diff --name-only origin/<baseRefName>`
   before committing, and fix any incoming harness-file violation minimally
   rather than bypassing hooks.

If Git reports an `index.lock`, inspect the exact path and active Git processes
first (`git rev-parse --git-path index.lock` and a process listing). Never delete
a lock owned by a live Git process. Remove only that exact lock path after
confirming no Git process owns it, then retry the operation.

Do not discard unrelated user changes to make a merge/rebase easier. If unrelated dirty files block the conflict-resolution attempt, stop and ask before stashing, committing, or reverting them.

Before a force-push after a rebase or conflict merge, run the task-defined checks
for every affected package. For web changes, run the affected test suite plus
`cd apps/web && pnpm run typecheck` and the web lint gate; use the equivalent
package checks for backend changes. Cleared markers do not prove a semantic
resolution: duplicate imports/hooks and stale localized mocks require these
checks to pass.

## Push after rebasing or merging

Capture the remote branch SHA before fetching the base or starting a merge/rebase.
Recheck it immediately before pushing. This guards both merge-
based conflict fixes and rebases against another writer advancing the PR branch:

```bash
git ls-remote origin refs/heads/<branch>
# fetch/merge or rebase, resolve, test, and commit through hooks
git ls-remote origin refs/heads/<branch>
```

If the second SHA differs from the first, stop and review the intervening remote
changes before pushing; do not overwrite them. After a merge-based fixup whose
remote SHA is unchanged, use a normal push:

```bash
git push origin HEAD:refs/heads/<branch>
```

After a successful rebase, use an exact force lease:

```bash
git push \
  --force-with-lease=refs/heads/<branch>:<expected-remote-sha> \
  origin HEAD:refs/heads/<branch>
```

An intervening fetch can update the remote-tracking ref consulted by generic
`--force-with-lease`. If the prior remote SHA was not captured, stop and capture
it before rewriting when possible; use the generic form only as an explicit last
resort, and never use an unconditional force push.
