---
status: current
system: platform
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
created: 2026-10-02
updated: 2026-10-02
owners:
  - kandev
---

# Workspace Git path details

## Boundary and mapping

This is the path-association portion of the [workspace status design](workspace-git-status.md).
Platform owns workspace observation and detail identity. Tasks retains environment ownership.

| Acceptance criteria | Design section |
| --- | --- |
| AC-PLATFORM-WORKSPACE-GIT-STATUS-001.36 | NUL-framed path records |
| AC-PLATFORM-WORKSPACE-GIT-STATUS-001.9 | Enrichment integration |
| AC-PLATFORM-WORKSPACE-GIT-STATUS-001.7, .31, .33 | Preserved execution and quality contracts |

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
Only the numeric columns are converted to counts. Binary `-` columns retain the current zero-count projection.
Zero-count rows are retained: pure renames and mode-only changes can still have patch content.
Incomplete records cannot yield invented destinations or reinterpret a rename's path records as statistics rows.
Iteration remains linear and checks caller cancellation between entries.

## Enrichment integration

`enrichWithUnstagedDiffBudget`, `enrichMixedUnstagedDiffsBudget`, and
`enrichWithStagedDiffBudget` use the same NUL parser and pass entries to their existing per-file enrichers.
`GitStatusUpdate.Files` remains keyed by the exact parsed status path.
Porcelain remains authoritative for membership, classification, facets, and rename origins.
Numstat enriches existing entries only. It cannot create synthetic file keys.

The flattened comparison and two mixed facets keep their current comparison bases and patch commands.
Path arguments remain separate arguments after `--`.
The index snapshot, observed HEAD, and publication fingerprint remain owned by the current tracker pipeline.

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
