# ADR-2026-08-19-repository-qualified-comparison-targets: Qualify Git Comparison Targets by Repository

**Status:** accepted
**Date:** 2026-08-19
**Area:** backend, agentctl, frontend, protocol, GitHub, GitLab

## Context

Kandev stores a task attachment's comparison base as a branch name. Agentctl expands `main` to
`origin/main` before trying a local `main`. That works only while `origin` identifies the repository
that owns the intended integration branch.

An ordinary task can instead be attached to a contributor fork. If its feature branch opens a pull
request against an upstream repository, both repositories commonly have a branch named `main`.
GitHub correctly compares the feature branch with upstream `main`, while Kandev continues comparing
with the fork's stale `origin/main`. The provider PR then reports one commit and a small diff, but the
local task can report hundreds of commits and repository-wide additions/deletions.

Pull-request association currently persists review identity and provider statistics. It does not
change the task attachment's comparison identity, reconfigure the live worktree, clear the session's
stored base SHA, or invalidate comparison caches. Fetching an `upstream/main` ref manually does not
help because Kandev's resolver still hardcodes `origin/<branch>`.

Branch name plus implicit `origin` is therefore not a durable comparison identity. Multi-branch
tasks also make repository-only retargeting unsafe: several attachment rows can share one repository,
and historical PRs remain available after the checkout changes branch.

## Decision

Kandev will model an explicit comparison target as a versioned, credential-free provider change
identity plus head and target repository/branch identities. The binding lives in the exact
`task_repositories.metadata["comparison_target"]` row whose attached repository and live checkout
branch match the provider-reported head.

The binding contains:

- provider, change kind, and provider-scoped change number;
- validated head repository identity and head branch;
- validated target repository identity and target branch;
- canonical HTTPS clone URLs without credentials, queries, or fragments; and
- provider repository IDs when the provider supplies them.

The binding contains no token, credential-helper state, provider title/body, merge-base SHA, local
path, or user-authored remote name. Unknown versions and malformed identities fail closed.

When the target repository is the attachment repository, Kandev keeps the normal
`origin/<base_branch>` comparison. When the repositories differ, agentctl owns one deterministic
comparison-only remote, fetches only the validated target branch into its remote-tracking ref, and
uses that exact ref for status totals, ahead/behind, commits, cumulative diff, and Review. `origin`,
the checkout branch, its upstream, and push routing do not change.

Agentctl's configured target is authoritative. Once an explicit target exists, caller-supplied branch
names and same-named `origin` or local refs cannot override it. Remote collision, validation, fetch,
or merge-base failure makes comparison-derived data unavailable; it does not trigger an implicit
fallback. Working-tree file status remains useful. The task summary suppresses numeric Git totals,
and Changes exposes a bounded error code with the intended repository/branch label.

Pull-request association invokes a provider-neutral task-service reconciler after the provider has
returned authoritative repository identities. Reconciliation requires exactly one attachment match:
the attached repository equals the PR head repository and the normalized live checkout branch equals
the PR head branch. A repository-only match, missing head identity, ambiguous multi-branch match, or
historical PR remains linked for Review but cannot retarget comparison.

The target records the change identity that authored it. A provider retarget updates the binding only
when it is for that same change. A newly and explicitly associated matching PR may replace the prior
target. Detaching that PR removes only its own binding. Merge and close retain the target because the
integration base remains useful for the task history. A user selection in **Compare against**
atomically clears the provider-derived target and returns to the attachment repository's selected
branch.

The binding is persisted before runtime fan-out. Launch and resume hydrate it into agentctl. Live
association resets affected session base SHAs, pushes the new target to running executions, refreshes
trackers, publishes task/status changes, and invalidates client commits and cumulative-diff caches.

This decision does not grant a new Git credential scope. A public target or a target already readable
with the execution's effective credentials can be materialized. An otherwise private target is shown
as unavailable until a separate security design authorizes that repository.

## Consequences

- Cross-fork PRs compare against the same repository and branch as the provider, even when the fork
  has a stale same-named branch.
- PR association becomes responsible for synchronizing review identity and local comparison identity.
- The comparison contract becomes restart-safe and executor-neutral because agentctl materializes the
  ref where the checkout actually lives.
- Multi-branch and historical-PR safety depends on exact repository plus branch matching, not list
  order or repository ID alone.
- Git status, commits, cumulative diff, Review, and bounded task summaries carry a small optional
  comparison-state contract.
- A failed explicit comparison is more visible, but it cannot silently publish misleading totals.
- Private cross-fork targets may remain unavailable when the task's existing credential policy cannot
  read them. This is preferable to silently widening repository authority.

## Alternatives considered

### Rewrite `origin` to the upstream repository

Rejected. The attachment repository remains the fork and owns push routing for the task. Rewriting
`origin` breaks repository identity, credentials, branch upstreams, issue lookup, and PR head routing.

### Store only `upstream/main`

Rejected. Remote names are mutable local configuration and do not survive every executor or restart.
They also do not prove which repository the ref represents.

### Infer the upstream from Git remote topology

Rejected. Fork parentage is provider data, while Git remotes are user- and agent-controlled. A remote
named `upstream` is neither required nor authoritative.

### Keep the branch-only model and refresh `origin/main`

Rejected. Refreshing the wrong repository produces a newer wrong answer. The missing information is
repository identity, not ref freshness.

### Use GitHub's additions/deletions and commit count everywhere

Rejected. Provider statistics do not include uncommitted workspace changes and do not replace local
Git status, staging, Review, or non-GitHub providers. Provider data is evidence, not the workspace
comparison engine.

### Fall back to `origin/<branch>` when target fetch fails

Rejected. The same branch name in another repository is the defect. A fallback would turn a known
failure into authoritative-looking but incorrect totals.

### Add the target repository to managed credential scopes automatically

Rejected for this repair. A linked PR proves comparison intent, not permission to expose another
repository's credential capability inside the executor. Private-target authorization needs its own
security contract.
