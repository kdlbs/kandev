---
status: draft
system: workspaces
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
---

# Worktree Base Refresh System Design

## Purpose and boundaries

The workspace system owns the repository checkout, remote state, and base ref
used to create a worktree. This design makes `repositories.pull_before_worktree`
an admission gate for new and recreated worktrees.

The task system continues to own session launch state and error presentation.
It consumes preparation errors through the existing launch-failure contract in
[Task Launch Failure Recovery](../../tasks/system-design/task-launch-failure-recovery.md).
The integration system continues to own provider credentials. The executor
continues to own credentials and SSH configuration inherited from its runtime.

This design does not refresh a worktree that Kandev can reuse without
recreation. It also does not change Git commands that an agent runs after
launch.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.1` | [Refresh policy](#refresh-policy) |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.2` | [Refresh routing](#refresh-routing) |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.3` | [Failure and recovery](#failure-and-recovery) |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.4` | [Base-ref selection](#base-ref-selection) |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.5` | [Base-ref selection](#base-ref-selection) |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.6` | [Multi-repository launch](#multi-repository-launch) |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.7` | [Launch-error projection](#launch-error-projection) |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.8` | [Empty remote exception](#empty-remote-exception) |

## Components and responsibilities

- `internal/orchestrator/executor` resolves every task repository and selects
  the refresh route that matches its provider and task Git credential policy.
  It passes `PullBeforeWorktree`, `RemoteSyncHandled`, and a provider-authenticated
  refresh callback to the lifecycle layer. The callback is not invoked until a
  worktree must be materialized or recreated.
- `internal/repoclone.Cloner` performs a strict refresh for a Kandev-managed
  provider checkout when provider credentials belong to the backend refresh
  boundary. `RefreshWorkspaceRepositoryWithCredentialRequest` binds the fetch
  to the exact workspace, task, session, and repository scope.
- `internal/worktree.Manager` performs strict non-interactive fetch and base-ref
  selection when the checkout uses the host or executor Git route. It creates
  no worktree until the required sync and ancestry checks succeed.
- `internal/agent/runtime/lifecycle` propagates repository preparation errors
  without starting the agent runtime.
- The task launch-failure projection stores and renders the resulting bounded,
  credential-safe error. This design does not add a parallel workspace error
  store.

## Data and contracts

`repositories.pull_before_worktree` remains the persisted user policy. No
schema change is required.

The existing worktree preparation fields have these contracts:

| Field | Contract |
| --- | --- |
| `PullBeforeWorktree` | A required remote refresh remains to be completed before base-ref selection. |
| `RemoteSyncHandled` | A strict provider-authenticated refresh completed for the exact checkout. The worktree manager must not run a second unauthenticated fetch. |
| `RefreshRepository` | An optional exact-scope refresh callback. Lifecycle invokes it only when creating or recreating a worktree; valid reuse bypasses it. |

`RemoteSyncHandled` is valid only after a successful strict refresh. A
best-effort fetch or a failed refresh cannot set it.

## Refresh policy

When `PullBeforeWorktree` is false, base-ref resolution keeps its offline local
behavior. This path does not claim that the checkout contains current remote
state.

When `PullBeforeWorktree` is true, worktree materialization requires one
successful refresh. A caller cannot convert a refresh error into a warning and
local ref. The rule applies to initial launch and to resume whenever Kandev
creates or recreates a worktree. A valid reusable worktree is returned before
the callback is invoked.

## Refresh routing

The executor resolves one of two refresh routes before lifecycle preparation:

1. A Kandev-managed provider route supplies a callback that calls the strict
   `repoclone` refresh with the exact credential request. The lifecycle layer
   invokes it only when it must materialize or recreate a worktree. On success,
   worktree creation clears `PullBeforeWorktree` and sets `RemoteSyncHandled`.
2. A host or executor route leaves `PullBeforeWorktree` set. The worktree
   manager uses the reconciled `origin` URL and the non-interactive host Git
   environment. This includes an executor-inherited GitHub checkout whose
   origin uses the protocol selected by the host `gh` configuration.

When a managed checkout carries a pull request number, the strict refresh also
fetches `refs/pull/<N>/head` into a local `origin/pr/<N>` remote-tracking ref.
Worktree creation uses that ref for the checkout branch, including fork pull
requests, without a second unauthenticated fetch.

Provider selection must follow the existing task Git credential policy. A
managed GitHub workspace uses the backend-managed credential route. An
executor-inherited GitHub workspace must not replace the reconciled SSH origin
with a managed HTTPS origin.

The plugin-provider strict-refresh behavior remains the model for exact
credential scope. GitLab and Azure DevOps keep their existing provider-specific
credential resolution.

## Base-ref selection

After a successful fetch, the worktree manager compares the local base `L`
with the fetched remote base `R`:

| Relationship | Start ref | Reason |
| --- | --- | --- |
| `R` contains `L` | `R` | Includes all local commits and current remote commits. |
| `L` contains `R` | `L` | Preserves local-only commits and includes current remote commits. |
| Neither contains the other | Error | The refs diverged and either choice would omit commits. |
| Ancestry cannot be proven | Error | Kandev cannot prove that the start ref is safe. |

A successful fast-forward pull normally produces the first or second state.
If pull fails after fetch, the same table selects the safe ref. The manager does
not reset, rebase, merge, or delete a branch.

`pullBaseBranch`, `resolveLocalBaseRef`, and
`pullCurrentBranchOrFallback` return errors to their callers. The errors retain
Git failure classification but exclude tokens, credential helper output, and
secret-bearing URLs.

## Empty remote exception

An authenticated remote advertisement with zero refs is not a stale local
fallback. It is the typed empty-remote state defined by
[Empty Remote Repository System Design](empty-remote-repositories.md).

This exception creates only the marked local baseline. It does not permit a
launch-time remote write or relax any authentication, network, timeout,
cancellation, missing-branch, or ancestry error.

## Multi-repository launch

The executor resolves and prepares repository specs before agent startup. If a
required refresh fails for one spec, lifecycle preparation returns an error and
does not start the runtime. The error carries stable repository identity and a
safe display name so same-provider sibling repositories remain distinguishable.

Preparation can leave an already refreshed checkout on disk. This state is
safe and retryable. A later launch performs the required checks again. Kandev
does not report a partial task launch because no agent process started.

## Launch-error projection

The executor returns required-refresh errors through its existing launch
failure path. The task system stores the active error in the current typed
summary and renders it on desktop and mobile after reload. The generic launch
failure category is sufficient unless implementation evidence shows that a
distinct category enables a safe recovery action.

The bounded detail names the repository and failure class. It does not include
credentials, tokens, raw authenticated URLs, or unrestricted Git output. Retry
uses the normal task launch action after the user corrects network, SSH, or
credential configuration.

## Failure and recovery

- Fetch authentication, network, timeout, cancellation, and Git command errors
  stop required preparation.
- A missing fetched remote base on a non-empty remote stops preparation. It
  cannot fall back to a local ref when refresh is required.
- A failed ancestry probe stops preparation.
- A divergent base stops preparation and preserves both refs.
- Disabling pull-before-worktree is the explicit offline opt-out. Kandev does
  not disable it automatically after a failure.
- Retrying task launch reruns remote refresh and base-ref selection.

## Security

Managed refresh uses the provider credential boundary and exact task, session,
and repository scope. Executor refresh uses only credentials available to the
selected host or executor route. Neither route copies a provider token into an
error, progress event, or log field.

Git remains non-interactive. `GIT_TERMINAL_PROMPT=0`, batch-mode SSH, bounded
command contexts, and Git subprocess admission prevent a hidden prompt from
blocking task launch.

## Observability

The existing sync progress callback reports a running event and then either a
completed event or a failed event. A failed event contains the bounded failure
class and repository identity, not a fallback-ref success message.

Structured logs record the repository ID, task ID, session ID, provider,
configured transport, refresh route, and failure class. Logs do not record
credential material or secret-bearing remote URLs.

## Related decisions

- [Required Worktree Refresh Fails Closed](../../../decisions/2026-08-25-required-worktree-refresh-fails-closed.md)
- [Separate GitHub Automation From Task Git Credential Policy](../../../decisions/2026-07-27-task-git-credential-policy.md)
- [Provider-Neutral Git Credential Broker](../../../decisions/2026-07-31-provider-neutral-git-credential-broker.md)
