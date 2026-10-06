---
status: current
system: platform
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
created: 2026-10-02
updated: 2026-10-06
owners:
  - kandev
---

# Workspace Git path details

## Boundary and mapping

This is the path-association portion of the [workspace status design](workspace-git-status.md).
Platform owns workspace observation and detail identity. Tasks retains environment ownership.

| Acceptance criteria | Design section |
| --- | --- |
| AC-PLATFORM-WORKSPACE-GIT-STATUS-001.36 | NUL-framed path records; Literal selected paths |
| AC-PLATFORM-WORKSPACE-GIT-STATUS-001.9 | Enrichment integration |
| AC-PLATFORM-WORKSPACE-GIT-STATUS-001.10, .11, .37, .38, .44 | Literal selected paths |
| AC-PLATFORM-WORKSPACE-GIT-STATUS-001.7, .31, .33 | Preserved execution and quality contracts |
| AC-PLATFORM-WORKSPACE-GIT-STATUS-001.45 | Plain selected patches |

## NUL-framed path records

Workspace per-file enrichment in `apps/backend/internal/agentctl/server/process/workspace_git_diff.go`
requests `git diff --numstat -z` for the captured HEAD-to-worktree comparison,
the captured HEAD-to-index comparison (`--cached`), and the index-to-worktree comparison.
The latter is only requested for existing mixed facets.

An ordinary record is `additions<TAB>deletions<TAB>path<NUL>`.
A rename record is `additions<TAB>deletions<TAB><NUL>old-path<NUL>new-path<NUL>`.
The empty path field identifies a rename. The following two path records belong to that row;
they are not independent statistics rows. The destination is the enrichment lookup key.

One workspace-specific cursor parser reads the first two tabs and consumes NUL-framed paths.
It preserves every path byte without trimming, C-unquoting, splitting on newlines, or interpreting arrows/braces.
Numeric columns must contain unsigned decimal digits that fit the native count type. Invalid, negative,
overflowing, or half-binary counts reject the record. Paired binary `-` columns retain the current zero-count projection.
Zero-count rows are retained: pure renames and mode-only changes can still have patch content.
Incomplete records cannot yield invented destinations or reinterpret a rename's path records as statistics rows.
Each of the three callers marks details unavailable when a nonempty remainder fails framing or count
validation. Publication settles only pending file/facet details as unavailable, preserving successful
already-ready details and existing carry-forward. Empty output remains valid.
Iteration remains linear and checks caller cancellation between entries.

## Enrichment integration

`enrichWithUnstagedDiffBudget`, `enrichMixedUnstagedDiffsBudget`, and
`enrichWithStagedDiffBudget` use the same NUL parser and pass entries to their existing per-file enrichers.
`GitStatusUpdate.Files` remains keyed by the exact parsed status path.
Porcelain remains authoritative for membership, classification, facets, and rename origins.
Numstat enriches existing entries only. It cannot create synthetic file keys.

The flattened comparison and two mixed facets keep their current comparison bases and patch commands.
Path arguments remain separate arguments after `--`, with literal selection as specified below.
The index snapshot, observed HEAD, and publication fingerprint remain owned by the current tracker pipeline.

## Literal selected paths

The existing Changes row selection sends repository-relative filenames through
`useScopedStageOperations`, the Git operation transport, repository-scoped operator lookup,
and `GitOperator.Stage` / `GitOperator.Unstage`. These values identify selected files or
directories; they do not authorize wildcard queries. Platform owns this selection contract
alongside mixed-facet mutation identity. Workspace and task repository routing remain unchanged.

At the process command boundary, prefix each explicit path with `:(literal)` without changing
any bytes of the original path. Keep each result as one argv element after `--`.
An empty entry stays empty so Git retains its existing invalid-pathspec rejection;
it must not become the all-matching `:(literal)` selector. An empty list still uses the all-files commands.
`--` terminates flag parsing but does not disable Git pathspec matching.
The literal prefix handles bracket/star/question-mark patterns, leading colons and dash-prefixed
names while preserving Git's actual directory-subtree selection.
Use one small process-local path representation helper shared by selected mutations and patches.
It does not validate refs, normalize names, enumerate filesystem matches, or alter global Git
environment/configuration. Git argument validation remains authoritative before execution.
Selected mutation and per-file patch subprocesses set `GIT_LITERAL_PATHSPECS=0` and
`GIT_ICASE_PATHSPECS=0` in their own copied environments, so inherited literal mode cannot
turn the internal marker into filename text and inherited case-insensitive matching cannot
select case-distinct siblings. `GitOperator` uses its existing per-command override helper; tracker patches use
an environment-aware sibling of the capped streaming helper. Tracker detail commands reuse
`gitCommandEnv` for the captured instance snapshot, index-file context and Git environment
preparation before applying selection overrides. They do not read the live process environment.
Generic capped diffs retain their
inherited environment, and empty-list mutations receive no selection override.

For nonempty selections, Stage keeps `add --` and Unstage keeps `reset HEAD --`, followed by
the literal path arguments. Empty selections keep `add -A` and `reset HEAD` respectively.
Operator locking, refresh, result errors, admission and timeouts retain their existing owners.

All four per-file patch commands in `workspace_git_diff.go` use the same literal representation:
flattened captured-HEAD-to-worktree, mixed index-to-worktree, single-layer cached fallback, and
mixed captured-HEAD-to-index. Numstat framing/count validation and status membership remain
unchanged. Actual rename destinations keep their observed origin metadata.
Patch selection happens before output capping, so an unrelated wildcard match cannot consume
the selected file's diff budget or attach another file's patch.

`handleGitStage`, `handleGitUnstage`, and status reads with `details=wait` retain their request
and response shapes. Read transport serializes the already-correct patch and exact file key;
it never decodes the internal pathspec representation into a product filename.
No intentional wildcard query outside these selected-file commands changes.
`GitOperator.Discard` uses `literalGitPathspec` for its selected `status --porcelain --`
query, `discardUntrackedFiles`'s `rm --cached --force --` selection, and
`discardTrackedFiles`'s `restore --source=HEAD --staged --worktree --` selections.
Each uses `runGitCommandWithEnvironment` with the existing selected-command overrides
for literal-marker parsing and exact case matching. Environment capture, filtering,
validation, admission and deadlines remain owned by that command runner.
Classification observes the selected literal file rather than a wildcard-matching sibling.
Filesystem deletion continues to use the original filename with `os.Remove`, never the
internal pathspec. Preserve empty-list rejection, empty-entry Git rejection, status-error
fallback, existing classification, per-file error aggregation, locking and `triggerRefresh`.
No directory deletion, rename parsing, rollback, global environment policy, or Stage/Unstage
change is introduced.

The registered `POST /api/v1/git/discard` route binds `GitDiscardRequest`, rejects an empty
list, resolves `req.Repo` through `gitOpForRepo` / `Manager.GitOperatorFor`, then calls
`Discard` on that repository operator. Request/result shapes and existing invalid-repository
handling remain unchanged. Real registered-route tests independently verify repository
identity and selected/unselected index and working-tree bytes.

Desktop and phone consume the same operation result and existing repository refresh.
This correction changes data selection only, without rendered layout, navigation, touch,
copy, or responsive behavior; real operator and HTTP tests cover the shared outcome.
The earlier Stage/Unstage package's tracked bracket and added Discard controls remain
passing controls. The distinct magic-name and mixed untracked/tracked cases are covered
by the [Discard selection package](../../../plans/git-discard-literal-selections/plan.md).

## Plain selected patches

Each of the four selected `WorkspaceTracker.capDiffOutput` call sites in
`workspace_git_diff.go` passes `--no-color` as a separate diff option before refs
and `--`: `enrichUnstagedFileDiff` (flattened captured HEAD to worktree),
`enrichMixedUnstagedFileDiff` (retained index to worktree), `enrichStagedFileDiff`
(single-layer cached fallback), and `enrichMixedStagedFileDiff` (captured HEAD to index).
The exact option is already admitted by `securityutil`. No validator change is needed.
Generic capped output and numstat commands keep their existing policy.

Disable presentation at production, before capping and publication. Do not strip
escape bytes afterward: they can be real source content or filename bytes.
Keep literal pathspecs, selected environment overrides, captured instance environment,
observed HEAD and retained index, admission, deadlines, budgets, cancellation,
carry-forward, and ready/unavailable propagation with their current owners.
This correction introduces no external-diff/textconv policy, Git configuration writes,
parser change, new process, wire field, or standalone comparison change.

`handleGitStatus` and `collectStatusForRepo` join `GetGitStatusWithDetails` when
`details=wait` is requested. The registered selected `/api/v1/git/status` and
aggregate `/api/v1/git/status/multi` routes serialize the same accepted file/facet
patches through `gitStatusResult`; repository resolution remains manager-owned.
Real registered HTTP tests prove decoded bytes and repository identity, alongside
public tracker tests. The cached-fallback test enters the existing staged-enrichment
boundary with empty flattened data and executes real Git, without inventing a new
production route to that branch.

Desktop and phone use the same patch data. This is a pure-data mobile exception:
no frontend, layout, navigation, touch, copy, or responsive contract changes.
Tracker and HTTP evidence cover the changed boundary; no browser/build/E2E is needed.
See the [plain-patch repair package](../../../plans/workspace-tracker-plain-patches/plan.md).

## Preserved execution and quality contracts

Keep `runGitOutput` and `capDiffOutput`, admission classes, deadlines, cancellation,
shared byte budgets, truncation, carry-forward, and ready/unavailable propagation unchanged.
No extra Git processes, worker queues, configuration, metrics, or API fields are introduced.

The branch aggregate command and `branchDiffTotals` retain their existing text protocol;
they do not associate statistics with individual paths.
`git_log.go` currently calls `parseNumstatEntry` through `numstatByPath` on mixed human-readable
stat/numstat/patch output. Retain that text parser and `resolveNumstatPath` for that consumer.
Its diff-section C-quote helpers do not solve literal-arrow ambiguity in workspace numstat.
History parsing and non-UTF-8 wire serialization are outside this change.

## Verification and platform scope

Disposable real repositories exercise `GetGitStatusWithDetails(ctx, true)` with ordinary and special
names across staged, unstaged, and mixed observations. Raw parser tests cover all framing independent
of filesystem support. Native Windows cannot create several POSIX-valid names, including literal
arrows containing `>`, trailing spaces, quotes, tabs, and newlines. Only those filesystem cases are
platform-scoped; portable ordinary/Unicode/rename cases and all parser cases remain enabled.

See the [implementation package](../../../plans/workspace-numstat-exact-paths/plan.md).

Literal-selection regressions use distinct selected and unselected index/worktree sentinels,
and distinct flattened/staged/unstaged patch markers. Bracket filenames are portable, including
native Windows. Only names with actual native filesystem restrictions are scoped to supported
platforms. Real repositories cover operator selection, directory and empty-list behavior,
tracker detail reads, and the existing HTTP transport without introducing browser work.
See the [literal-selection repair package](../../../plans/git-literal-file-selections/plan.md).
