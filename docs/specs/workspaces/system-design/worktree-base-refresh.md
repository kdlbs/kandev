---
status: draft
system: workspaces
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
---

# Worktree Base Refresh System Design

## Purpose and boundaries

The workspace system owns the repository checkout and the worktree base ref.
This design keeps host worktrees local-first while remote materialization stays
strict.

The task system owns session launch state and error presentation. The
integration system owns provider credentials. Executors own their Git transport
and remote workspace preparation.

This design does not refresh a valid worktree that Kandev reuses. It does not
change Git commands that an agent runs after launch.

## Requirement mapping

| Acceptance criterion | Design section |
| --- | --- |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.1` | [Refresh policy](#refresh-policy) |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.2` | [Refresh policy](#refresh-policy) |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.3` | [Local fallback](#local-fallback) |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.4` | [Base-ref selection](#base-ref-selection) |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.5` | [Base-ref selection](#base-ref-selection) |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.6` | [Required materialization](#required-materialization) |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.7` | [Multi-repository launch](#multi-repository-launch) |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.8` | [Required materialization](#required-materialization) |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.9` | [Empty remote](#empty-remote) |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.10` | [Failure and recovery](#failure-and-recovery) |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.11` | [Pull-request base reconciliation](#pull-request-base-reconciliation) |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.12` | [Pull-request base reconciliation](#pull-request-base-reconciliation) |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.13` | [Missing-base fallback](#missing-base-fallback) |

## Components and responsibilities

- `internal/orchestrator/executor` resolves repository paths, refresh routes,
  credentials, and executor capabilities. It also resolves live GitHub pull-
  request bases through an injected provider seam.
- `internal/worktree.Manager` verifies local refs, attempts host refresh, and
  selects the worktree base. Pull-request base refresh remains strict except
  for the verified missing-remote-ref fallback.
- `internal/repoclone.Cloner` materializes provider-managed repositories when
  no user-owned local checkout supplies the branch.
- `internal/github.Service` reconciles changed pull-request bases through
  narrow injected interfaces. Provider lookup and task-repository propagation
  are best-effort.
- `internal/agent/runtime/lifecycle` stops launch only when no usable base
  remains.
- The task projection shows bounded warnings and required-materialization
  errors.

## Data and contracts

`repositories.pull_before_worktree` remains a persisted user policy. No schema
change is necessary.

The preparation fields have these contracts:

| Field | Contract |
| --- | --- |
| `PullBeforeWorktree` | Kandev attempts remote refresh before local worktree creation. |
| `RemoteSyncHandled` | The configured remote route completed a refresh for this checkout. |
| `RefreshRepository` | An optional provider refresh for worktree materialization. |

`PullBeforeWorktree` is not a universal admission gate. Local-ref availability
determines whether a failed refresh is recoverable.

The task repository's stored base remains the durable comparison target and
offline launch fallback. The GitHub task-PR row tracks the provider observation;
polling propagates a changed non-empty base to the matching task repository.

## Pull-request base reconciliation

The executor reads the pull-request number from task-repository metadata and
the owner and repository name from the repository entity. For GitHub repository
rows with a positive pull-request number, it asks an injected resolver for the
current base before materialization. A successful non-empty result overrides
the launch request only. A missing resolver, lookup error, or empty result keeps
the stored base and does not stop launch.

Once the pull-request task reaches Git refresh, its base remains strict for
unproven failures. A proven missing remote base can use only the fallback
described below.

The GitHub polling service compares the previous `TaskPR.BaseBranch` with the
incoming non-empty base. On change, it updates the task repository whose task
and repository IDs match. When more than one association matches, the checkout
branch must also match the pull-request head. Update failures are logged and do
not fail the authoritative task-PR sync.

## Refresh policy

If `PullBeforeWorktree` is false, Kandev uses the selected local base without a
remote request.

If `PullBeforeWorktree` is true, Kandev attempts the configured refresh before
new or recreated host worktrees. A valid reusable worktree bypasses this
attempt.

The refresh route still follows the task Git credential policy. A host checkout
keeps its configured origin and transport. A provider-managed checkout keeps
its exact credential scope.

## Local fallback

Before refresh, the worktree manager verifies the selected local ref. If that
ref exists, refresh is best effort.

Authentication, network, timeout, Git, and missing-remote-ref errors return the
selected local ref. The manager emits a bounded warning and does not include
raw credential output.

The fallback does not change local or remote refs. It does not push the local
branch.

Pull-request base refresh does not use this local fallback. It remains strict
for unproven failures and uses only the verified missing-base fallback below.

## Base-ref selection

After a successful fetch, the worktree manager compares local base `L` and
remote base `R`:

| Relationship | Start ref | Result |
| --- | --- | --- |
| `R` contains `L` | `R` | The worktree includes current remote commits. |
| `L` contains `R` | `L` | The worktree preserves local-only commits. |
| Refs diverge | `L` | The worktree preserves the selected local history and shows a warning. |
| Ancestry is unknown | `L` | The worktree preserves the selected local history and shows a warning. |

The manager never resets, rebases, merges, removes, or hides either ref during
selection.

## Required materialization

Remote access is required when Kandev cannot verify a usable local base. This
case includes an explicit remote-only ref and an executor that must clone the
repository.

If materialization fails, lifecycle stops launch. The task error identifies the
repository and uses a bounded failure class.

Provider selection still follows the existing Git credential policy. A managed
HTTPS route must not replace an executor-owned SSH route.

## Missing-base fallback

A fetch error that explicitly reports a missing remote ref is the only failed
fetch eligible for fallback. When the request carries a different non-empty
fallback base, the manager fetches that branch through the same non-interactive
route and applies the normal containing-ref checks. Success completes sync,
uses the fallback ref, and records a warning naming the requested and fallback
branches.

If the fallback is absent or its refresh fails, preparation remains failed. A
missing requested or fallback remote ref uses `missing_remote_ref`; auth,
network, timeout, cancellation, and other Git failures keep their existing
fail-closed classifications.

## Empty remote

An authenticated remote with zero refs uses the marked local baseline from the
[Empty Remote Repository System Design](empty-remote-repositories.md).

This path creates no remote ref during launch. Publication remains an explicit
user action.

## Multi-repository launch

Each repository resolves its base independently before agent startup. A local
fallback counts as a prepared repository.

If one repository needs remote materialization and has no usable base, the
whole task stops before agent startup. The error names that repository.

## Failure and recovery

- A refresh error with a valid local base produces a warning and continues.
- A missing local and remote base produces a launch error.
- Caller cancellation stops preparation without a fallback worktree.
- A retry repeats remote materialization only when the task still needs it.
- A valid reused worktree does not refresh or materialize its base again.
- A pull-request base refresh failure stops preparation unless the missing
  remote ref has a separately refreshed configured fallback.
- A pull-request fallback reports the requested and selected branches and uses
  `missing_remote_ref` when no usable fallback exists.

## Security

Git stays non-interactive. Host refresh uses the host credential route.
Provider refresh uses exact task and repository scope.

Warnings contain a failure class, branch name, and repository identity. They do
not contain tokens, credential-helper output, or secret URLs.

## Observability

The existing sync progress callback reports a running event and then either a
completed event or a failed event. A failed event contains the bounded failure
class and repository identity. A successful missing-base fallback reports a
completed event and records the branch substitution warning on the worktree.
Local fallback reports completion with a warning, not a launch error.

Structured logs record repository identity, refresh route, failure class, and
selected fallback ref. Logs exclude credential material and raw remote URLs.

## Related decisions

- [Local Worktree Refresh Is Best Effort](../../../decisions/2026-08-31-local-worktree-refresh-best-effort.md)
- [Required Worktree Refresh Fails Closed](../../../decisions/2026-08-25-required-worktree-refresh-fails-closed.md)
- [Separate GitHub Automation From Task Git Credential Policy](../../../decisions/2026-07-27-task-git-credential-policy.md)
- [Provider-Neutral Git Credential Broker](../../../decisions/2026-07-31-provider-neutral-git-credential-broker.md)
