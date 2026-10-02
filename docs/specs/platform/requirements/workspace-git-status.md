---
status: active
system: platform
created: 2026-07-19
updated: 2026-10-02
owners:
  - kandev
---

# Workspace Git Status Requirements

## Overview

Users opening Changes and Review need current workspace status without excessive Git or filesystem work. Repeated requests share useful work. Slow refreshes have bounded recovery, and file visibility does not wait for diff content.

## Contract update

This requirement includes progressive refresh and bounded recovery.
It replaces the former non-publication rule in criterion `.2` and clarifies admission, completion, and cancellation in `.3`, `.4`, and `.5`.
A failed live source still cannot authorize an unmarked persisted fallback.

## Terminology

- **Dependency tree:** A directory named `node_modules` at any repository depth. The exclusion applies only to untracked content. It includes pnpm package content under `node_modules/.pnpm`. A path that Git tracks remains a tracked path.
- **Eligible changed path:** A tracked changed path, or an untracked path that Git does not ignore and that is not inside a dependency tree.

## Requirements

### REQ-PLATFORM-WORKSPACE-GIT-STATUS-001: Workspace Git Status

**Intent:** Users opening Changes and Review need current workspace status without excessive Git or filesystem work. Repeated requests share useful work. Slow refreshes have bounded recovery, and file visibility does not wait for diff content.

#### Acceptance criteria

- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.1:** Cached reads return the latest workspace-tracker snapshot. When no cached snapshot exists, the tracker performs a live observation.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.2:** Fresh reads observe the live worktree. Accepted tracker-owned results update the cache and subscribers, even after the requesting caller stops waiting.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.3:** Overlapping live observations for the same repository and admission class share one underlying observation. Different repositories in a multi-repository task may still be observed in parallel.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.4:** Every non-cancelled caller receives the same complete file-membership snapshot or error from a shared observation. Diff enrichment can arrive later. A caller whose own context is cancelled returns promptly without cancelling or otherwise poisoning the result for other callers.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.5:** Tracker shutdown or the bounded shared-observation deadline cancels the underlying work. Cancelled basic observations do not publish incomplete file membership. Cancellation during later enrichment preserves an already accepted basic snapshot without claiming complete diff data.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.6:** After Git output is parsed, changed-file and synthetic untracked-diff enrichment performs work proportional to the number of eligible changed entries plus the bounded content processed.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.7:** Existing diff limits remain in force: 10 MiB maximum source file size, 256 KiB maximum per emitted diff representation, and a 2 MiB enrichment threshold per status snapshot. Flattened compatibility diffs and layer-specific diffs all participate in the same snapshot threshold. Because the threshold is checked before enriching each representation, the final accepted representation may preserve the existing overshoot of up to the 256 KiB cap. Existing skip reasons remain unchanged.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.8:** Large changed sets retain every eligible path and its status metadata. Once the total diff budget is exhausted, files that are not enriched retain `budget_exceeded` as their diff skip reason.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.9:** When one path has both index and working-tree changes, the workspace snapshot preserves the staged and unstaged change facets independently, including each facet's status, line totals, rename origin, diff content, and diff skip reason when applicable.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.10:** Changes lists a mixed path in both Staged and Unstaged, with facet-specific status and line totals. Opening either row shows only that layer's diff. Overall changed-file totals continue to count the repository and path once.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.11:** Staging or unstaging a mixed path operates once on its repository-qualified path and the next snapshot removes the consumed facet while preserving any facet that remains.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.12:** Desktop and mobile Changes surfaces expose the same mixed-path sections, facet-specific diff selection, and staging actions without replacing their established platform-native layouts.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.13:** An untracked path inside a dependency tree does not appear in a workspace snapshot and does not receive per-file status or diff enrichment, even when the repository has no matching ignore rule.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.14:** A tracked changed path inside `node_modules` remains visible. An untracked path outside dependency trees remains visible when Git does not ignore it.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.15:** Creating, changing, or removing only untracked dependency-tree content does not change the workspace monitor fingerprint or start a full status refresh.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.16:** Kandev-owned comparison-target Git commands do not request credentials or confirmation through a terminal. Each command uses the instance's effective Git credential environment, enforces non-interactive prompt controls, and ends within its command deadline.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.17:** Comparison-target materialization does not delay instance readiness or a Git-status UI request. Until background materialization succeeds, file status remains usable and comparison-derived data reports unavailable.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.18:** When a comparison-target fetch fails over its canonical HTTPS transport, Kandev does not retry SSH or another transport automatically. It preserves checkout and push routing and reports that comparison data is unavailable.

- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.19:** Every eligible changed path and its staged or unstaged classification appears before slow diff or branch-total work completes. Mixed facets, rename origins, symlink identity, and submodule identity remain correct.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.20:** If a requesting client times out or disconnects, an accepted shared refresh result remains available for later reads and existing subscribers. Recovery does not require another workspace mutation.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.21:** Each tracker lifetime has a unique opaque source identity. Revisions order results only within that identity; capture timestamps order eligible sources across identities. Older observations cannot replace newer accepted state, and diff data cannot attach to a different checkout, index, worktree state, repository, or comparison target.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.22:** Fresh interactive file observations retain interactive admission independently of background observations. At most one enrichment job executes per repository tracker, with bounded replacement work and no duplicate job for an unchanged observation.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.23:** Before a complete file snapshot arrives, Changes shows loading or unavailable status. Only a successful complete empty snapshot permits the normal clean empty state.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.24:** Dirty files remain visible while diff enrichment is pending or unavailable. Opening a pending diff shows its state. Missing diff content cannot close the panel or imply that the file was discarded.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.25:** A pending or failed refresh preserves valid prior data in the same environment and repository. The UI identifies its freshness. Replaced workspace state cannot reuse old data as current.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.26:** A multi-repository failure preserves every healthy repository. Each failed repository has an identifiable state and retry path, even when another repository is clean.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.27:** Sessions sharing an environment see the same accepted repository state. Session and live-execution bindings are accepted only for the canonical environment root or an exact active repository worktree recorded on that environment. Late results from replaced sessions, executions, stream connections, or workspace bindings cannot change their successor state, including initial-subscribe reads and workspace-stream callbacks.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.28:** Each Changes-surface activation requests or joins one fresh snapshot for its shared environment and connection scope, even when complete cached membership exists. Foreground refreshes have bounded snapshot-response recovery when the initial notification is missed. After recovery fails, Changes exposes Retry instead of remaining indefinitely empty or loading.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.29:** Desktop and phone surfaces expose loading, failure, Retry, pending diffs, and preserved prior data through shared state. Phone controls remain touch usable and accessible.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.30:** New status text uses the translation system in all six supported catalogs. Traditional Chinese generation and punctuation follow the repository localization rules.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.31:** Progressive refresh retains subprocess admission, command deadlines, existing diff limits, and focus-based polling. The tracker-owned enrichment deadline covers pre-validation, diff work, and final validation. Per-file evidence bounds are independent from diff-output limits. Caller cancellation and tracker shutdown release owned jobs, resources, and subscriptions.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.32:** Compact persisted snapshots do not establish complete file membership or a clean workspace. Without a usable live source, Changes identifies incomplete or unavailable status instead of presenting missing file details as clean.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.33:** A failed Git detail command cannot certify an empty diff or zero statistics as ready. Healthy files and facets retain ready detail where available, failed detail is identifiable, ordinary same-fingerprint observations do not restart unavailable enrichment, and an explicit refresh can retry it without repository mutation.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.34:** An unavailable implicit comparison reference can make ancestry totals unavailable without invalidating independently successful per-file diffs.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.35:** A WebSocket connection admits at most four concurrent session Git refresh operations. Excess requests receive a correlated unavailable result without starting source work.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.36:** For every eligible tracked changed path in a stable observation, successful diff enrichment shall associate statistics and patch content with the exact repository-relative path in the file-membership snapshot, including Unicode, quoting characters, literal rename-like text, tabs, newlines, and leading or trailing whitespace supported by the task filesystem. Actual renames shall enrich the destination path while preserving observed rename metadata. This applies independently to flattened, staged, and unstaged representations; binary and content-unchanged changes may legitimately have zero line counts.

## Out of scope

- Suppressing generated directories other than `node_modules`. They continue to follow repository and global Git ignore rules.
- Suppressing tracked paths based on a directory name.
- Filtering dependency trees only after Git has enumerated their files.
- Replacing the existing Git-status routes or environment ownership.
- Persisting complete live diffs in the database.
- Aggressive polling of inactive tasks.
- Automatically changing the Git transport after an authentication or transport error.

## System design

See the [workspace status design](../system-design/workspace-git-status.md) and [delivery plan](../../../plans/changes-panel-git-refresh/plan.md).
