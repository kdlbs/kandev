---
status: shipped
created: 2026-07-19
owner: kandev
---

# Workspace Git Status

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

## API surface

No route or payload shape changes.

- `GET /api/v1/git/status?repo=<subpath>&fresh=<bool>` returns the existing `GitStatusResult` shape.
- `GET /api/v1/git/status/multi?fresh=<bool>` returns the existing `MultiRepoGitStatusResult` shape containing `PerRepoGitStatus` entries.
- The `fresh` query parameter continues to select a live observation rather than a cached tracker snapshot.

## Failure modes

| Scenario | Observable behavior |
|---|---|
| Primary branch or porcelain observation fails | The live observation fails and the prior cached snapshot remains available. |
| Secondary diff enrichment fails | The established same-HEAD carry-forward behavior is preserved. |
| One caller cancels while a shared observation is running | That caller returns its context cancellation promptly; other callers remain eligible to receive the shared result. |
| The tracker stops or the shared deadline expires | Underlying work is cancelled and no partial result is published or cached. |
| One repository fails during a multi-repository request | Successful repository entries remain available and the failure is reported on its repository entry. |

## Scenarios

- **GIVEN** a stale cached snapshot after a commit, **WHEN** a caller requests `fresh=true`, **THEN** the response reflects the live clean tree and a later cached read still returns the prior cached snapshot.
- **GIVEN** six simultaneous fresh requests for one repository, **WHEN** their observations overlap, **THEN** exactly one underlying status observation runs and all non-cancelled callers receive the same capture timestamp and result.
- **GIVEN** simultaneous fresh requests for two repositories, **WHEN** multi-repository status runs, **THEN** one observation per repository may run in parallel and each response remains identified with its repository.
- **GIVEN** one waiter cancels during a shared observation, **WHEN** other waiters remain, **THEN** the cancelled waiter returns promptly and the remaining waiters receive the completed result.
- **GIVEN** tracker shutdown or the shared-observation deadline while enrichment is running, **WHEN** cancellation reaches the observation, **THEN** filesystem iteration stops and no partial snapshot is cached.
- **GIVEN** approximately 15,000 untracked text files, **WHEN** fresh status is computed, **THEN** every path is present, emitted diff content obeys the existing limits, files not enriched after total-budget exhaustion have `budget_exceeded`, and post-porcelain enrichment remains linear in the number of entries.
- **GIVEN** one invalid repository in a multi-repository request, **WHEN** other repositories succeed, **THEN** the response retains the successful entries and reports the failure only on the invalid repository.
- **GIVEN** verification needs writable scratch space, **WHEN** it selects a location, **THEN** the location is outside every Git worktree and existing shared caches remain reusable; if a legacy run creates root-level `.verify-cache` or `.tmp`, Git status ignores it.

## Out of scope

- Changing Git-status API routes, response shapes, or frontend rendering.
- Raising or removing existing diff-content limits.
- Changing multi-repository fan-out behavior.
- Making fresh reads owners of the polling cache.
- Replacing Git subprocesses with a native Git implementation.

## Implementation plan

See [Workspace Git Status Scalability plan](../../plans/workspace-git-status-scalability/plan.md).
