---
status: draft
system: workspaces
requirements:
  - REQ-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001
  - REQ-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002
---

# Empty Remote Repository System Design

## Purpose and boundaries

The workspace system owns repository materialization, base refs, and task worktrees. This design adds a safe path for a remote that advertises zero refs.

The task system continues to own session launch state and launch-error presentation. The integration system continues to own provider credentials and provider change-request APIs.

Task launch creates local Git state only. Push and Create PR remain the only actions in this design that can initialize the remote.

## Requirement mapping

| Acceptance criteria | Design section |
| --- | --- |
| `AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001.1` | [Remote-state classification](#remote-state-classification) |
| `AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001.2` through `001.4` | [Local baseline](#local-baseline) |
| `AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001.5` | [Launch and recreation flow](#launch-and-recreation-flow) |
| `AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001.6` | [Failure and recovery](#failure-and-recovery) |
| `AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001.7` | [Multi-repository tasks](#multi-repository-tasks) |
| `AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002.1` through `002.4` | [First publication](#first-publication) |
| `AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002.5` through `002.7` | [Publication races and partial results](#publication-races-and-partial-results) |
| `AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002.8` | [Desktop and mobile behavior](#desktop-and-mobile-behavior) |
| `AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002.9` | [Change-request creation](#change-request-creation) |

## Components and responsibilities

- `internal/repoclone` reports authenticated remote-ref state after clone or strict refresh.
- `internal/orchestrator/executor` carries the typed remote-ref state into repository preparation.
- `internal/worktree.Manager` creates the local baseline and normal task worktree under the repository lock.
- `internal/gitbootstrap` owns the deterministic commit contract, marker-ref format, and marker validation.
- `internal/agentctl/server/process.GitOperator` performs first publication with task runtime credentials.
- The existing Changes hooks map bounded publication error codes to translated user messages.
- Existing Changes controls remain the desktop and mobile entry points.

## Remote-state classification

Repository refresh returns one typed state: `has_refs`, `empty`, or `unknown`. Only an authenticated remote advertisement with zero refs produces `empty`.

A missing requested branch on a remote with other refs is not an empty remote. Authentication, network, timeout, cancellation, and malformed-response errors produce `unknown` with a classified error.

Managed provider refresh returns the state through its exact-scope credential callback. Host and executor refresh use the same non-interactive Git credential route as the required fetch.

The worktree preparation contract carries the typed state with `RemoteSyncHandled`. A successful zero-ref advertisement counts as a completed required refresh.

## Local baseline

`internal/gitbootstrap` creates one deterministic empty commit. The commit uses an empty tree, fixed Kandev identity, fixed timestamp, and fixed message.

The helper creates two refs in one local ref transaction:

- `refs/heads/<base>` points to the baseline commit.
- `refs/kandev/empty-remote/<base>` marks the exact baseline commit.

The marker never uses commit-message, author, timestamp, or tree heuristics. Publication requires the marker and local base ref to point to the same commit.

The worktree manager performs this operation under its existing repository lock. If the local repository gains a real ref before the transaction, preparation stops and reevaluates normal base selection.

The baseline command does not run repository hooks. It does not use user signing configuration. It writes no working-tree file.

## Launch and recreation flow

The normal refresh gate runs before worktree creation. If the result is `empty`, the manager creates or reuses the marked local baseline.

The existing branch planner then creates the task branch and worktree from the local base. Worktree records retain the selected `BaseBranch` and generated task branch.

Resume and recreation use the same marker validation. If a managed checkout is cloned again, the deterministic helper produces the same baseline for the same Git object format.

No launch path calls `git push`, a provider mutation API, or a credential route with write intent.

## First publication

`GitOperator` checks for the marker before an ordinary Push or Create PR branch push. The operator obtains the task base branch from the existing per-repository base-branch map.

If the marker is absent, the current Git operation does not use this feature. It follows the existing push or change-request flow.

If the marker is valid, the operator advertises the selected remote with task runtime credentials. If the remote still has zero refs, it performs these operations:

1. Push the marked local base to `refs/heads/<base>` without force.
2. Push the current task branch through the existing push path.
3. Set the task branch upstream through the existing rules.

The base publication does not use provider background credentials. It uses the same Git environment and remote authority as the user-selected task operation.

Force Push can retain its existing task-branch behavior. The baseline push never receives a force flag.

## Publication races and partial results

Another actor can initialize the remote after task launch. The pre-publication advertisement detects this state before Kandev publishes the baseline.

If the remote base equals the marked baseline, publication continues. This case makes retries idempotent after a prior baseline push.

If the remote has other refs or a different base, Kandev does not publish the marked baseline. It fetches safe evidence when possible and returns `empty_remote_remote_changed`.

Kandev does not merge, rebase, reset, delete, or force a ref during this recovery. The local task branch and marker remain available.

If baseline publication fails, Kandev returns `empty_remote_base_publish_failed`. It does not start the task-branch push.

If baseline publication succeeds but task-branch publication fails, Kandev returns `empty_remote_branch_publish_failed`. The result states that the base exists and the task branch remains local.

## Change-request creation

Create PR uses its requested base branch after normalization. The requested base must match the marked task base during empty-remote publication.

The operator publishes the base first and the task branch second. It calls the existing GitHub, GitLab, Azure DevOps, or plugin provider only after both pushes succeed.

Provider retry behavior starts after the Git refs are present. A provider API error does not remove either published ref.

## Desktop and mobile behavior

The feature adds no new page, dialog, drawer, or control. Desktop users use the existing Changes split button and change-request dialog.

Phone users use the same Changes actions through the existing touch menu and task layout. The global mobile menu treatment remains the nearest shipped interaction pattern.

Shared action handlers map publication error codes to localized recovery text. The mobile path does not depend on hover, right-click, or a desktop-only control.

## Multi-repository tasks

Remote-state classification and local baseline state are repository-scoped. One empty repository does not change another repository's base or marker.

Task launch continues only after every repository completes its required preparation. Push and change-request actions continue to target one selected repository.

## Persistence and cleanup

The local marker ref is the durable bootstrap identity. No database migration is required.

Worktree cleanup follows the existing task lifecycle. The canonical checkout retains the local base and marker until normal repository maintenance removes that checkout.

If Kandev reclones an unchanged empty remote, it recreates the deterministic baseline. If the remote now has refs, normal refresh replaces the empty-remote path.

## Security

Read access does not grant write authority. Task launch never tests write permission and never publishes a remote ref.

First publication uses the task runtime credential route and exact repository remote. Managed credentials retain workspace, task, session, repository, host, and path scope.

Logs and user messages contain bounded error classes. They do not contain tokens, helper output, authenticated URLs, or unrestricted Git stderr.

The operator validates every branch name before it constructs a Git command. The marker ref is derived only from a validated base branch.

## Failure and recovery

- If remote-state classification is `unknown`, required preparation fails closed.
- If a non-empty remote lacks the requested base, normal base fallback or failure behavior applies.
- If local baseline creation fails, task launch stops before worktree creation.
- If another actor initializes the remote, the user must reconcile history before publication.
- If write credentials are absent, local work remains available and publication returns the existing credential failure class.
- If only task-branch publication fails, the user can retry Push without recreating the task.

## Observability

Structured logs record repository identity, task identity, session identity, remote-ref state, base branch, and publication phase. Logs omit remote credentials and raw authenticated URLs.

Preparation progress reports a bounded `empty_remote_baseline` result. Publication reports `base`, `task_branch`, or `provider` as the failed phase.

Diagnostics expose the marker ref name and commit ID only for the selected repository. They do not include task prompts or credential helper data.

## Related decisions

- [Keep Empty Remote Bootstrap Local Until Publication](../../../decisions/2026-08-30-empty-remote-bootstrap-publication.md)
- [Required Worktree Refresh Fails Closed](../../../decisions/2026-08-25-required-worktree-refresh-fails-closed.md)
- [Provider-Neutral Git Credential Broker](../../../decisions/2026-07-31-provider-neutral-git-credential-broker.md)
- [Separate GitHub Automation From Task Git Credential Policy](../../../decisions/2026-07-27-task-git-credential-policy.md)
