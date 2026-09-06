# Merge Conflicts

Use this when the PR fixup flow finds GitHub file-level conflicts, an unmerged local index, or conflict markers in tracked files.

## Detect

Inspect GitHub's mergeability state:

```bash
gh pr view <PR> --json number,url,baseRefName,headRefName,mergeable,mergeStateStatus
```

The `gh pr view --json` field is `baseRefName`, not `baseRefOid`; do not request
the unsupported field. Resolve the base OID with `git ls-remote` or the base
fields in `scripts/pr-state`. When exact OIDs are required, query `gh api repos/<owner>/<repo>/pulls/<PR> --jq '{base_sha:.base.sha,base_ref:.base.ref,head_sha:.head.sha,head_ref:.head.ref}'` and verify the fetched base tip matches `base_sha` before merging.

Treat `mergeable:"CONFLICTING"` or `mergeStateStatus:"DIRTY"` as an actionable merge-conflict blocker. Treat `mergeable:"UNKNOWN"` as inconclusive: wait one short cadence and query again before deciding. States such as `BEHIND`, `BLOCKED`, `UNSTABLE`, or `HAS_HOOKS` may require an update or more CI/review work, but they are not by themselves proof of file-level conflicts.

Always use the freshly queried `baseRefName`. GitHub can retarget a stacked child PR after its parent merges, so neither the Git upstream nor a base branch remembered from an earlier fixup round is authoritative.

Before any checkout, merge, or rebase, confirm that the current worktree owns
the PR head. After reading `headRefName` and `headRefOid`, run
`git worktree list --porcelain`, locate the worktree whose branch matches
`headRefName`, and operate there. Verify `git branch --show-current`,
`git rev-parse HEAD`, and `git status --short` before mutating anything. If no
matching worktree exists, state that and create or use an explicit worktree only
within the authorized workflow; never risk mutating `main` or another task's
worktree.

Inspect the local worktree:

```bash
git status --short
git ls-files -u
git grep -n -E '^(<<<<<<<|>>>>>>>)'
```

If `git ls-files -u` prints entries, or conflict markers are present in tracked source files, resolve those conflicts before fixing CI or review comments. `git grep` scans only tracked files, and intentionally checks only the unambiguous start/end markers so Markdown setext headings do not create false positives. Do not start a new merge/rebase while the index is already unmerged.

## Resolve

### When GitHub reports conflicts and the local index is clean

Capture the PR branch's remote head before fetching the base or starting the
merge. Recheck it immediately before pushing; use the explicit lease shown in
**Push after rebasing or merging**.

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

   Before committing a merged specification or system-design file, rerun the
   repository's specification lint, including size checks; a marker-free merge
   can still fail the file-size hook. Do not raise a legacy size exception to
   hide that failure.

   Confirm that the final diff against the current GitHub base contains only the PR's intended delta. A clean index is insufficient if a retargeted stacked PR accidentally retains its merged parent's changes.
   After the operation completes, inspect each resolved path with
   `git diff -- origin/<baseRefName>...HEAD -- <resolved-file>` and reread the
   complete surrounding comments and logic; this catches a duplicate-hunk
   resolution that silently drops an explanatory line.

   Incoming base commits may already contain a partial or similar fix. Compare
   their helpers and tests with the PR side before staging, and deduplicate
   overlapping test symbols or behavior rather than blindly preserving both.

   If the base extracted logic into a new helper or type file, or moved its
   tests, while the PR changed the old inline location, a marker-only merge can
   silently drop the PR invariant. Inspect the extracted or renamed base file
   and moved tests, port the PR-specific behavior into the new owner, update
   imports and exports, then run the focused helper and consumer tests. Verify
   with `git diff origin/<baseRefName>...HEAD -- <resolved-files>` and the
   affected test suite.

   If the current base moved a flat legacy spec into `requirements/` and
   `system-design/` paths, preserve the canonical base split. Move branch-only
   authority into the correct migrated part, add branch-only documents to the
   system README maps, and remove stale old-path entries from
   `docs/specs/spec-lint.json`. Then run `python3 scripts/lint-spec-files.py
   --all` and inspect `git diff <base>...HEAD` for the intended migration delta.
   Verify again with `git ls-files -u`, the marker scan, and the full spec lint.

   If conflict resolution touches `apps/backend/go.mod` or `apps/backend/go.sum`,
   marker-free text is not enough: a union can silently lose a required module
   checksum. From `apps/backend`, run `go mod tidy` and then `go mod verify`
   sequentially, inspect that only intended module files changed, and run the
   affected backend package tests or build before committing.

   For structured files (especially JSON catalogs), validate syntax and duplicate
   keys before continuing. JSON's default parser may silently keep the last
   duplicate key; use an `object_pairs_hook` that raises on duplicates, then run
   the affected formatter/test and `git diff --check`. Also run the
   authoritative schema or domain validator when the file's semantics span
   arrays or ownership assignments; object-key checks cannot catch duplicate
   array-level assignments.

   When the resulting `HEAD` is a merge commit, do not use `HEAD^1..HEAD` or a
   parent diff as the PR delta: depending on parent order it can include the
   merged base or omit the branch's changes. Use the authoritative three-dot
   range, `git diff <base-remote>/<base-ref>...HEAD`, and inspect the merge
   parents with `git log --first-parent` only for history. If
   `git diff --cached --check` reports whitespace that came from the incoming
   base, compare the base-only range separately before attributing it to the PR;
   keep the strict check for whitespace introduced by the PR or resolution.

### When the base advanced without conflicts

An advanced base is not itself a reason to rebase or merge the PR. When exact-
head CI is green, the base advanced, and GitHub reports no file conflict,
compare the paths changed on both sides of the old merge base:

```bash
git diff --name-only <old-merge-base>..<authoritative-base> > /tmp/base-paths
git diff --name-only <old-merge-base>..HEAD > /tmp/head-paths
comm -12 <(sort /tmp/base-paths) <(sort /tmp/head-paths)
```

If paths overlap, build the exact conflict-free synthetic merge without
mutating the PR branch. `git merge-tree --write-tree` returns a tree object;
turn it into an unreachable merge commit and inspect it in a detached
worktree:

```bash
SYNTHETIC_TREE="$(git merge-tree --write-tree <authoritative-base> HEAD)"
SYNTHETIC_COMMIT="$(printf 'review synthetic merge\n' | \
  git commit-tree "$SYNTHETIC_TREE" -p <authoritative-base> -p HEAD)"
git worktree add --detach /tmp/pr-merge-review-<number> "$SYNTHETIC_COMMIT"
# Run focused tests for the overlapping paths in that worktree.
git worktree remove --force /tmp/pr-merge-review-<number>
```

Do not push or create a branch for the synthetic commit. Frontend checks need
the detached worktree's own `apps/node_modules` installation; do not assume a
sibling worktree's dependencies are available. Verify the synthetic merge,
focused tests, clean normal worktree, local `HEAD` equal to its upstream and PR
head, and a fresh GitHub query still reporting `MERGEABLE`/`CLEAN`.

### When related PRs conflict semantically without file conflicts

GitHub's mergeability state covers file-level compatibility, not whether
stacked or parallel PRs still agree on the durable contract. Use this path when
a plan, review, or PR names a dependency and the current PR or landed dependency
has overlapping `REQ-*`/`AC-*` IDs, conflicting design statements, or a changed
API/interface contract even though GitHub reports `MERGEABLE`.

1. Resolve the current head and merged state of every related PR. Treat refs in
   an older plan or review as snapshots; re-read the affected requirements,
   system designs, plans, work orders, and test annotations at the current
   heads.
2. Identify the landed contract as authoritative. Preserve its stable IDs and
   meanings; allocate the next unused IDs for distinct behavior instead of
   renumbering or reusing an existing ID to hide the collision.
3. Reconcile all affected traceability links, including every `REQ-*`/`AC-*`
   reference in requirements, system designs, plans, work orders, and test
   `@covers` annotations, plus implementation assumptions to the current interface.
   Do not call isolated green checks composable until the producer and consumer
   agree on the same contract.
4. Run `python3 scripts/lint-spec-files.py --all` and the focused producer,
   consumer, and integration tests. After any integration push, refresh the
   exact PR head and repeat the dependency and contract comparison.

Verify that the related PR heads and merged state were recorded, no duplicate
or repurposed stable IDs remain, all affected artifacts reference the same
contract, spec lint and focused tests pass, and the final exact-head evidence
was collected after the last integration push.

### Queue synthetic-base conflicts

A merge-queue coordinator may provide a synthetic base that conflicts with
the PR even when the PR reports `MERGEABLE/CLEAN` and exact-head checks pass.
Treat this as queue-order merge evidence, not a CI failure. Verify the SHA is
a commit, run `git merge-tree <synthetic-base> HEAD`, and inspect both
`git show <synthetic-base>:<path>` and `git show HEAD:<path>` (or staged blobs).
Preserve both sides manually, then require `git merge-tree <synthetic-base>
<new-head>` to be conflict-free before focused checks, a hooked commit, push,
and fresh exact-head PR state.

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

   Use the normal hooks and capture the full receipt. If `git commit --no-edit`
   fails, fix the reported semantic or hook error and retry the commit; never
   bypass it with `--no-verify`. A current-base merge may stage many incoming
   files, making status look like hundreds of changes; do not reset or broadly
   unstage them. Resolve only `UU` paths, then inspect `git diff --stat
   origin/<baseRefName>` and `git diff --name-only origin/<baseRefName>` before
   committing, and fix any incoming harness-file violation minimally rather
   than bypassing hooks.

If Git reports an `index.lock`, inspect the exact path and active Git processes
first (`git rev-parse --git-path index.lock` and a process listing). Never delete
a lock owned by a live Git process. Remove only that exact lock path after
confirming no Git process owns it, then retry the operation.

Do not discard unrelated user changes to make a merge/rebase easier. If unrelated dirty files block the conflict-resolution attempt, stop and ask before stashing, committing, or reverting them.

Before a force-push after a rebase or conflict merge, run the task-defined checks
for every affected package. For web changes, run the affected test suite plus
`cd apps/web && pnpm run typecheck` and the web lint gate; use the equivalent
package checks for backend changes. Run managed E2E with `pnpm e2e:run` after
the merge, or rebuild both `make -C apps/backend build` and
`make -C apps/backend e2e-plugin-package` before any `--no-build` run, because
global setup rejects stale packaged fixtures. Cleared markers do not prove a semantic
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
changes before pushing; do not overwrite them. When the captured SHA is
unchanged, prefer the explicit lease for both conflict merges and rebases:

```bash
git push \
  --force-with-lease=refs/heads/<branch>:<expected-remote-sha> \
  origin HEAD:refs/heads/<branch>
```

An unchanged merge-based fixup is also a fast-forward, so a normal push is
safe when an explicit lease is unavailable:

```bash
git push origin HEAD:refs/heads/<branch>
```

An intervening fetch can update the remote-tracking ref consulted by generic
`--force-with-lease`. If the prior remote SHA was not captured, stop and capture
it before rewriting when possible; use the generic form only as an explicit last
resort, and never use an unconditional force push.

After a conflict merge is pushed, run `scripts/pr-await <PR>` for the new head
and then refresh `scripts/pr-state --summary <PR>` and
`scripts/pr-resolve list <PR>`. Treat the merge commit's checks and review state
as new evidence; do not reuse the pre-merge report.

If preserving both sides of a conflict makes a whole-file hook exceed its
max-lines or size limit, remove duplicated guidance or refactor the file while
keeping both contracts. Rerun the affected lint; never bypass the hook or add
an unexplained exception just to complete the merge.

If the user explicitly requires a one-commit PR, resolve and verify the current
base first, then use a soft reset to that base, recommit through normal hooks,
and push with the captured exact-head lease. Verify
`git rev-list --count <base>..HEAD` and the live PR head. Otherwise retain the
merge commit created by conflict resolution.
