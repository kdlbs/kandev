---
status: current
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
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.14` | [Safe refresh diagnostics](#safe-refresh-diagnostics) |

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

The task repository's stored base remains the branch-only comparison target
and offline launch fallback. An explicit repository-qualified target takes
precedence under the proposed amendment below. The GitHub task-PR row tracks the provider observation;
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

## Repository-qualified PR bases

This section implements criteria .15 through .19. It qualifies the branch-only
lookup and fallback rules above. Delivery is tracked in the
[fork PR base resolution package](../../../plans/fork-pr-base-resolution/plan.md).

### Identity and provider lookup

`ResolveRemoteDefaultBranch` retains its origin-default meaning. It cannot
resolve a PR base because its input contains no task or provider identity.
Do not replace `repositories.default_branch` with a PR target.

Extend the executor's `PRBaseResolver` result beyond a string. The proposed
result contains the validated PR identity, head identity, target repository,
target branch, and optional observed base OID. Reuse `models.ComparisonTarget`
for repository and branch validation. Keep the observed OID transient; it is
neither a merge-base nor a replacement for durable branch identity.

Resolve the provider namespace from the exact attachment's existing binding:

1. Use `ComparisonTarget.TargetRepository` and its change number when present.
2. For `RemoteContribution`, use its validated canonical change identity. PR URL
   tasks already attach to the target repository and configure a source remote.
3. Otherwise use an exact linked `TaskPR` identity, matched to this attachment
   and checkout branch. A bare PR number is not globally unique.
4. Legacy metadata-only requests can query the attached repository. Accept the
   response only after its head repository and branch match the attached
   checkout. For a `RemoteContribution`, compare the PR head to the validated
   source repository and require the PR target repository to match the
   attachment. An ambiguous or mismatched response cannot resolve a qualified
   PR base. Ordinary target-attached PR links use the compatibility path below.

Never choose the first PR in a task-wide list. Validate current binding ownership
before applying a live retarget. Preserve manual comparison selections and
historical associations under the existing task-service reconciliation rules.

`githubPRBaseResolver` in `internal/backendapp/orchestrator.go` returns a
validated target and optional observed base OID. REST uses `base.sha` and
GraphQL uses `baseRefOid`. The supported `gh pr view/list --json` field set does
not include `baseRefOid`; `GHClient.GetPR` reads `.base.sha` through `gh api`
and keeps PR details usable if that best-effort OID read fails. A lookup failure
can retain a valid stored qualified target, but cannot downgrade that target to
a bare branch name. A known cross-repository lookup failure or invalid legacy
association cannot fall back to a stored branch without a qualified target;
ordinary provider outages remain eligible for the existing stored-base rule.

A stored qualified target can materialize its exact branch without a fresh
provider snapshot. The fetched commit supplies the current OID. Without a
validated target identity, a known cross-repository request remains unresolved.
Same-repository offline branch fallback retains criterion .11.

### Ordinary PR-link launch compatibility

The [PR-link fork launch repair](../../../plans/pr-link-fork-launch/plan.md)
clarifies criterion .15 for existing browser-created attachments. This amendment
is planned. Its work orders own implementation status.

The browser PR-link path stores the target repository, PR number, checkout
branch, and base branch. It does not create a `RemoteContribution` binding.
`associatePRFromRepoInputs` links the PR asynchronously. Launch must therefore
resolve identity correctly before and after that association exists.

Base validation distinguishes these attachment forms:

- With `RemoteContribution`, the persisted source repository and head branch
  remain authoritative. The provider target must match the attached repository.
- With a fork-attached checkout, the provider head must match the attached
  repository and checkout branch. Existing comparison-target checks still apply.
- With an ordinary target-attached PR checkout, provider resolution must use
  the attached target namespace and exact PR number. The returned target must
  match that attachment, and the returned head branch must match the checkout.
  The validated provider response supplies the source repository for this
  launch. Missing, malformed, unrelated, or ambiguous identities are rejected.

The last form requires a positive PR number and a non-empty checkout branch.
It applies only without an explicit contribution or comparison binding.
It cannot override an explicit source binding after a mismatch. An unrelated
linked PR cannot authorize this form merely by sharing a number or branch.

`validPRBaseIdentity` must evaluate the attachment form before applying its head
repository comparison. Do not remove repository checks or accept a branch-only
match. `githubPRBaseResolver` retains provider namespace validation and exact
linked-association selection. Without a linked row, it resolves the attached
namespace directly. Multiple matching linked rows remain an error.

For a target-attached checkout, the existing PR-head fetch uses the target
repository's PR namespace. The comparison base remains that target repository's
branch. A fork-attached checkout retains qualified upstream materialization.
No contribution binding, comparison binding, credential scope, or push destination
is synthesized by this compatibility path. Provider resolution is read-only.

Retry of an unprepared legacy task uses the same validation. Valid worktree
reuse and explicit manual-base overrides retain their existing rules.
A provider outage retains the existing fallback policy. Known cross-repository
errors and cancellation must not become branch-only fallback successes.

Coverage includes ordinary PR-link metadata with and without a linked row,
explicit source mismatch, unrelated target, malformed identity, colliding PR
numbers, and two attachments where only one is valid. A real Git fixture proves
that PR head and base commits come from their respective repositories.

### Preparation and materialization

Carry the qualified target through `repoInfo`, lifecycle launch and preparation
requests, and `worktree.CreateRequest`. Audit their copy and conversion methods,
including multi-repository preparation, workspace-only creation, and recreation.
Keep checkout-head fetching separate from target-base fetching. A fork-origin
checkout must not fetch an upstream PR number from the fork's `refs/pull` namespace.
Use the validated PR host for that ref or the validated head repository/branch.

Before branch-only fallback, select the explicit target when present. Fetch only
its target branch into `ComparisonTarget.ComparisonRef()`. Resolve the resulting
commit OID and return it for Git operations. Retain repository/branch identity
for metadata and diagnostics; do not persist a hash as `BaseBranch`.

Reuse the deterministic comparison remote and collision rules from
[ADR-2026-08-19-repository-qualified-comparison-targets](../../../decisions/2026-08-19-repository-qualified-comparison-targets.md).
Extract reusable Git materialization into `internal/common/gitbase` rather than
importing agentctl process code into the backend. Keep typed consumer errors
in their existing owners. Agentctl keeps its asynchronous comparison scheduling.
The shared primitive only validates and materializes a target through an injected,
classified Git runner. It neither schedules work nor chooses fallback policy.

The remote keeps push disabled. A conflicting configured URL is an error.
Commands remain bounded and noninteractive under the existing credential route.
This repair grants no additional private-repository credential scope.

When a provider OID is supplied, verify that exact commit against the fetched
branch snapshot. A mismatch stops required preparation with a stale-observation
error. A later retry obtains fresh provider evidence; no automatic merge occurs.
Do not fetch an arbitrary unvalidated SHA or overwrite a user branch.

Remote executor materialization must receive the same qualified target before
its base-dependent Git operations. Carry the optional contract through the
runtime client and `agentctl/server/api/workspace_materialize.go`. Keep branch
labels separate from resolved refs in existing request validation. An ordinary
checkout with no qualified target uses the existing route.

Required materialization failure stops new preparation. A valid reused worktree
still follows the existing reuse policy. Background comparison failure still
leaves working-tree status usable under the platform status contract.

### Recovery and persistence

`recoverTaskLaunchBranch` currently resolves origin and calls the manual
`UpdateRepositoryBaseBranch` operation. That operation can clear a comparison
target, even when the branch name is unchanged.

Reject `retry_default` for a validated cross-repository binding before any
repository-default or task-base write. Preserve the binding and error stamp.
Use the existing bounded recovery-error envelope. Do not relabel the action
or silently treat it as permission to abandon the target. `retry_launch` can
retry the same qualified preparation. An explicit user-selected base branch
is marked in task-repository metadata and remains authoritative across provider
refreshes. Only an explicit comparison-target association clears that marker;
provider sync and recovery cannot replace the manual selection.

Ordinary `retry_default` still resolves and updates the repository default.
Recovery must check the exact attachment and current error stamp. A second
attachment or historical PR cannot supply the selected target.

No database migration is required. Existing contribution/comparison bindings
remain the durable identity. New OID observations and preparation refs remain
transient. Provider retarget persistence stays in the existing reconciler.

### Evidence and compatibility

Tests must distinguish identical branch names in two repositories with different
commits. Include a fork-only origin, a non-default PR target, linked worktrees,
provider retargeting, target fetch errors, remote collisions, and cancellation.
Include two PRs with the same number in different namespaces and a mixed task
with one valid repository and one unresolved cross-repository base.

Preserve the completed comparison-target and stacked-PR packages. Their old
verification results remain historical. New compatibility results belong to
this repair's work orders. Issue #3856 owns the separate reconciliation hint.

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

### Safe refresh diagnostics

AC .14 extends the existing observability contract. `manager_git.go` captures
combined command output, but failure paths retain only coarse reasons or an
exit status. Inspect that output in memory and emit fixed diagnostic values.
Do not append raw output to logs, returned errors, or progress events.

Keep the existing policy `reason` separate from a diagnostic-only
`diagnostic_code`. Add an allow-listed classifier in
`internal/worktree/refresh_diagnostics.go`. It recognizes authentication,
SSH public-key rejection, SSH host verification, DNS, connection failure,
TLS verification, missing remote ref, non-fast-forward rejection, repository
access failure, and lock contention. Unknown output maps to `unknown`.
Context cancellation and execution timeout take precedence over text matches.
Each code maps to a fixed English explanation. No explanation includes
substrings from Git output. Repository access failure does not assert that
credentials are invalid, and lock contention does not assert a stale lock.

Emit one structured diagnostic per failed fetch or pull with `operation`,
`repository_path`, `branch`, `reason`, `diagnostic_code`, `detail`, and an exit
code when available. The local repository path identifies the checkout without
reading its remote URL. Existing failure or fallback logs can carry these fields
instead of adding a duplicate diagnostic. Cover both required and best-effort
paths, including a failed configured fallback branch.

Classification is observational. Do not feed new diagnostic categories into
missing-base fallback eligibility or alter `syncFailureCause` identities.
Preserve context errors and current progress/error payloads. Correct the
`syncFailureCause` comment that implies raw output reaches internal logs.
No new logging configuration, subprocess, retry, or persistence is required.

Delivery: [Recovery diagnostic fixes](../../../plans/recovery-diagnostics/plan.md).

## Related decisions

- [Local Worktree Refresh Is Best Effort](../../../decisions/2026-08-31-local-worktree-refresh-best-effort.md)
- [Required Worktree Refresh Fails Closed](../../../decisions/2026-08-25-required-worktree-refresh-fails-closed.md)
- [Separate GitHub Automation From Task Git Credential Policy](../../../decisions/2026-07-27-task-git-credential-policy.md)
- [Provider-Neutral Git Credential Broker](../../../decisions/2026-07-31-provider-neutral-git-credential-broker.md)
