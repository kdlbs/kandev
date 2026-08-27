---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
created: 2026-07-19
updated: 2026-08-19
owners:
  - kandev
---
# Workspace Git Status System Design

## Purpose and boundaries

This design preserves the technical source detail for `REQ-PLATFORM-WORKSPACE-GIT-STATUS-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-WORKSPACE-GIT-STATUS-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Why

Users opening or focusing Changes and Review need a current workspace snapshot without a large generated or untracked tree monopolizing agentctl. Repeated requests for the same repository must not amplify expensive Git and filesystem work, and the initial session-hydration path must remain within its two-second live-status budget by falling back when necessary.

## What

- Cached reads return the latest workspace-tracker snapshot. When no cached snapshot exists, the tracker performs a live observation.
- Fresh reads observe the live worktree and do not themselves replace the polling cache.
- Overlapping live observations for the same repository share one underlying observation. Different repositories in a multi-repository task may still be observed in parallel.
- Every non-cancelled caller receives the same completed snapshot or error from a shared observation. A caller whose own context is cancelled returns promptly without cancelling or otherwise poisoning the result for other callers.
- Tracker shutdown or the bounded shared-observation deadline cancels the underlying work. Cancelled work does not publish or cache a partial snapshot.
- After Git output is parsed, changed-file and synthetic untracked-diff enrichment performs work proportional to the number of changed entries plus the bounded content processed.
- Existing diff limits remain in force: 10 MiB maximum source file size, 256 KiB maximum emitted diff per file, and a 2 MiB enrichment threshold per status snapshot. Because the threshold is checked before enriching each file, the final accepted file may preserve the existing overshoot of up to the 256 KiB per-file cap. Existing skip reasons remain unchanged.
- Large changed sets retain every path and its status metadata. Once the total diff budget is exhausted, files that are not enriched retain `budget_exceeded` as their diff skip reason.
- Multi-repository responses retain repository identity and partial-success behavior.
- Verification tooling preserves shared managed Go and lint caches for reuse while keeping invocation scratch and command output outside repository worktrees. The root-level `.verify-cache` and `.tmp` paths are ignored as safeguards against legacy or misconfigured verification runs.

### Base-commit staleness and refresh

The commits panel (`git log <base>..HEAD`) and cumulative diff anchor to a base commit derived from the session's stored `base_commit_sha` and `base_branch`. That anchor becomes stale when the branch history moves relative to the true integration branch after the base was recorded — most commonly a stacked-PR parent that merges and disappears, a rebase onto the integration branch, or a base branch that was deleted upstream. When the anchor is stale, the panel enumerates commits that are not part of the branch's own contribution, inflating the count.

- A stored base commit SHALL be treated as **stale** when it is a strict ancestor of the true merge-base between `HEAD` and the resolved integration candidate — the first branch that resolves through the priority list `origin/main → origin/master → main → master`. When `merge-base(HEAD, <resolved integration candidate>)` advances past the stored `base_commit_sha`, the stored base is stale. A stored base that equals or is a descendant of that merge-base is NOT stale.
- Staleness correction SHALL be limited to comparison targets that are not a live non-default upstream branch. The resolved default integration target (`main`/`master`, or its `origin/*` form) remains subject to the normal stale-base check. When the target IS a live non-default upstream branch (for example a feature comparing against `origin/develop` that has merged newer `origin/main` commits), its own merge-base is authoritative and SHALL NOT be overridden merely because it is an ancestor of the integration merge-base — doing so would hide commits that are genuinely part of that comparison.
- When the stored base is stale, commit enumeration and cumulative diff SHALL use the freshly computed merge-base against the resolved integration candidate instead of the stored SHA. The panel therefore reflects only the commits the branch actually introduces over the integration branch.
- Base resolution SHALL prefer the upstream integration ref (`origin/<name>`) over a bare local ref of the same name. A local ref that no longer tracks any live upstream (for example a merged/deleted stacked-parent branch that lingers only as a local ref) SHALL NOT anchor the commit range when an upstream integration ref is available.
- Staleness detection is a read-time correction: it changes which base the commits/diff are computed against. It does not by itself rewrite the persisted `base_commit_sha`; the persisted value continues to follow the existing capture and the "Compare against" base-branch reset paths.
- When neither an upstream integration ref nor a usable merge-base can be resolved (unrelated histories, offline mirror with no `origin/*`), behavior falls back to the existing stored-base / branch-tip anchor and the panel is unchanged from today.

### Repository-qualified comparison targets

A branch name alone is not a complete comparison identity. A task may be attached to a contributor fork while its pull request targets a branch with the same name in another repository. In that case `origin/main` is the fork's `main`, not the pull request base.

- Each task-repository attachment MAY persist one versioned, credential-free `comparison_target` in its metadata. The target identifies the provider change, the validated head repository and branch, and the target repository and branch.
- The attachment row is the scope boundary. A target applies only when the provider-reported head repository matches the attached repository and the normalized head branch matches that attachment's live checkout branch. Repository-only matches, historical pull requests, and sibling branches MUST NOT retarget the attachment.
- When the target repository is the attached repository, the existing `origin/<base_branch>` behavior remains authoritative and no extra remote is required.
- When the target repository differs, Kandev materializes a deterministic comparison-only remote without renaming or rewriting `origin`, fetches only the validated target branch, and compares against that exact remote-tracking ref.
- The repository-qualified ref is authoritative for task-card Git totals, ahead/behind, commits, cumulative diff, and Review expansion. Once configured, those surfaces MUST NOT silently fall back to `origin/<branch>` or a bare branch with the same name.
- A successful pull-request association or retarget updates the matching attachment, clears its sessions' stored base commit, refreshes the live agentctl comparison, and invalidates client commit/diff caches. The corrected result appears without a session restart. Launch and resume reconstruct the target from attachment metadata.
- Explicitly choosing a branch in **Compare against** returns the attachment to repository-local comparison: the write updates `base_branch` and removes any provider-derived `comparison_target` atomically.
- A detached pull-request association removes the target only when that target names the detached change. Closing or merging the associated change does not remove the target because its base remains the relevant integration line for the task history.
- Kandev does not broaden task Git credentials to materialize a target. Public targets and targets already readable through the execution's effective Git credentials are supported. An unreadable private target fails visibly.
- Target setup is fail-closed. If identity validation, remote collision checks, fetch, or merge-base resolution fails, file status remains usable, but comparison-derived totals, ahead/behind, commits, and cumulative diff are unavailable. The Changes panel names the intended `<owner>/<repository>:<branch>` target and shows an actionable comparison error; task-list summaries suppress numeric Git totals and expose the unavailable state instead of publishing a same-named `origin` fallback.

## API surface

Existing Git-status routes remain in place. Their result and stream payloads add optional comparison state so old clients continue to decode them:

- `GET /api/v1/git/status?repo=<subpath>&fresh=<bool>` returns the existing `GitStatusResult` shape.
- `GET /api/v1/git/status/multi?fresh=<bool>` returns the existing `MultiRepoGitStatusResult` shape containing `PerRepoGitStatus` entries.
- The `fresh` query parameter continues to select a live observation rather than a cached tracker snapshot.
- A repository status MAY include `comparison_target` with its credential-free display identity and `comparison_error_code` when the explicit target is unavailable.
- Commit and cumulative-diff requests use agentctl's configured repository-qualified ref when present. A caller-supplied branch name cannot override it.
- The internal agentctl control API adds a per-repository comparison-target update alongside the existing base-branch update. It validates identities and refs, materializes the exact ref, replaces tracker state, and triggers a refresh.
- The bounded task Git summary adds `comparison_unavailable`; when true, additions/deletions are not rendered as authoritative task-card statistics.

## Failure modes

| Scenario | Observable behavior |
|---|---|
| Primary branch or porcelain observation fails | The live observation fails and the prior cached snapshot remains available. |
| Secondary diff enrichment fails | The established same-HEAD carry-forward behavior is preserved. |
| One caller cancels while a shared observation is running | That caller returns its context cancellation promptly; other callers remain eligible to receive the shared result. |
| The tracker stops or the shared deadline expires | Underlying work is cancelled and no partial result is published or cached. |
| One repository fails during a multi-repository request | Successful repository entries remain available and the failure is reported on its repository entry. |
| Stored base commit is a strict ancestor of `merge-base(HEAD, <resolved integration candidate>)` and the target is not a live non-default upstream branch | Commit enumeration and cumulative diff use the freshly computed merge-base, not the stale stored SHA; the count reflects only the branch's own commits. |
| Comparison target is a live non-default upstream branch (e.g. `origin/develop`) whose merge-base is an ancestor of the integration merge-base | The target's own merge-base is preserved; the range is NOT re-anchored to the integration line. |
| Resolved base branch exists only as a stale local ref (upstream merged/deleted) but an integration `origin/*` ref is present | The `origin/*` integration ref anchors the range; the stale local ref does not. |
| No `origin/*` integration ref and no usable merge-base (unrelated histories) | Falls back to the existing stored-base / branch-tip anchor; behavior is unchanged from today. |
| A linked PR targets a same-named branch in another repository | The deterministic comparison remote and exact target ref anchor every comparison-derived surface. `origin/<branch>` is not consulted. |
| The PR head repository or branch does not match exactly one task attachment | The PR remains associated for Review, but no attachment comparison target changes; the mismatch is logged with task, repository, and branch identity. |
| The configured comparison remote already exists with another URL | Target setup fails closed and reports `comparison_remote_conflict`; Kandev does not overwrite the remote or fall back to `origin`. |
| The configured target branch cannot be fetched | File status remains available; comparison-derived data is unavailable with `comparison_fetch_failed`. Prior or same-named origin statistics are not published as current. |
| The backend or executor restarts | The persisted target is revalidated and materialized before comparison-derived status becomes authoritative. |

## Scenarios

- **GIVEN** a stale cached snapshot after a commit, **WHEN** a caller requests `fresh=true`, **THEN** the response reflects the live clean tree and a later cached read still returns the prior cached snapshot.
- **GIVEN** six simultaneous fresh requests for one repository, **WHEN** their observations overlap, **THEN** exactly one underlying status observation runs and all non-cancelled callers receive the same capture timestamp and result.
- **GIVEN** simultaneous fresh requests for two repositories, **WHEN** multi-repository status runs, **THEN** one observation per repository may run in parallel and each response remains identified with its repository.
- **GIVEN** one waiter cancels during a shared observation, **WHEN** other waiters remain, **THEN** the cancelled waiter returns promptly and the remaining waiters receive the completed result.
- **GIVEN** tracker shutdown or the shared-observation deadline while enrichment is running, **WHEN** cancellation reaches the observation, **THEN** filesystem iteration stops and no partial snapshot is cached.
- **GIVEN** approximately 15,000 untracked text files, **WHEN** fresh status is computed, **THEN** every path is present, emitted diff content obeys the existing limits, files not enriched after total-budget exhaustion have `budget_exceeded`, and post-porcelain enrichment remains linear in the number of entries.
- **GIVEN** one invalid repository in a multi-repository request, **WHEN** other repositories succeed, **THEN** the response retains the successful entries and reports the failure only on the invalid repository.
- **GIVEN** verification needs writable scratch space, **WHEN** it selects a location, **THEN** the location is outside every Git worktree and existing shared caches remain reusable; if a legacy run creates root-level `.verify-cache` or `.tmp`, Git status ignores it.
- **GIVEN** a session whose stored `base_commit_sha` is a strict ancestor of `merge-base(HEAD, <resolved integration candidate>)` (for example a stacked-PR parent branch that has since merged into the integration branch and lost its upstream ref), **WHEN** the commits panel is requested, **THEN** the enumerated commit count matches `git rev-list --first-parent --count $(git merge-base HEAD <resolved integration candidate>)..HEAD` and excludes the commits that already landed on the integration branch.
- **GIVEN** a stored base branch that no longer has an upstream ref (the remote branch was deleted) but a local ref of the same name still points at an old branch point, **WHEN** the commits panel resolves its base, **THEN** it anchors to the merge-base against the `origin/main`/`origin/master` integration ref rather than the stale local ref.
- **GIVEN** a session whose stored `base_commit_sha` equals or is a descendant of the current merge-base against the integration branch, **WHEN** the commits panel is requested, **THEN** the stored base is used unchanged and the count is identical to today's behavior.
- **GIVEN** a session comparing against a live non-default upstream branch (for example `origin/develop`) whose merge-base is a strict ancestor of the integration merge-base, **WHEN** the commits panel resolves its base, **THEN** it keeps the develop merge-base and does not re-anchor to the integration line, so commits genuinely part of the develop comparison remain visible.
- **GIVEN** a worktree with no `origin/*` integration ref and a HEAD sharing no history with any candidate branch, **WHEN** the commits panel resolves its base, **THEN** it falls back to the existing stored-base / branch-tip anchor and does not error.
- **GIVEN** a task attached to `contributor/widget` whose branch targets `upstream/widget:main`, and the fork's `main` is hundreds of commits behind, **WHEN** Kandev associates the matching PR, **THEN** the task card, commits panel, and cumulative diff compare against the fetched upstream ref and show only the branch contribution.
- **GIVEN** the same fork and upstream both have a branch named `main`, **WHEN** a repository-qualified target is active, **THEN** no comparison path resolves that target through `origin/main`.
- **GIVEN** two attachments for the same repository on different branches, **WHEN** a PR matches one head branch, **THEN** only that attachment receives the target and the sibling's comparison remains unchanged.
- **GIVEN** a historical PR for a previous checkout branch, **WHEN** provider sync refreshes it, **THEN** it cannot replace the current branch's comparison target.
- **GIVEN** a live cross-fork target, **WHEN** the backend restarts and the session resumes, **THEN** Kandev reconstructs the same comparison remote/ref before publishing authoritative totals.
- **GIVEN** an explicit target that cannot be fetched, **WHEN** Changes and task-list surfaces refresh, **THEN** working-tree files stay visible, comparison-derived values are marked unavailable, and no inflated same-named-origin total is shown.
- **GIVEN** a provider-derived target, **WHEN** the user selects a branch from the attachment's **Compare against** picker, **THEN** the provider target is removed atomically and the selected local-repository branch becomes authoritative.

## Out of scope

- Replacing existing Git-status routes.
- Raising or removing existing diff-content limits.
- Changing multi-repository fan-out behavior.
- Making fresh reads owners of the polling cache.
- Replacing Git subprocesses with a native Git implementation.
- Rewriting the persisted `base_commit_sha` as part of staleness detection. The read-time correction changes only which base the commits/diff compute against; persistence continues to follow the existing capture and "Compare against" reset paths.
- Auto-retargeting the session's `base_branch` when a stacked parent merges. Detecting a stale base and picking a live integration ref is in scope; changing the stored base branch is not.
- Granting a new Git credential scope for a private comparison repository.
- Periodically fetching a moving target branch on every status poll. Targets are refreshed on association, retarget, launch/resume, and explicit comparison refresh.
- Automatically changing the checkout, push remote, upstream, or `origin` because a comparison target changes.

## Implementation plan

See [Workspace Git Status Scalability plan](../../../plans/workspace-git-status-scalability/plan.md) and [Fork PR Comparison Targets plan](../../../plans/fork-pr-comparison-targets/plan.md).
