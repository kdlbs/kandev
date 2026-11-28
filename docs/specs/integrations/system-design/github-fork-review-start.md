---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001
created: 2026-09-23
updated: 2026-11-27
owners:
  - kandev
---

# GitHub fork review task start design

## Ownership and mapping

The integration system owns GitHub review-watch task creation and its
provider-specific auto-start decision. The task system owns task persistence.
The [workspace worktree design](../../workspaces/system-design/worktree-base-refresh.md)
continues to own repository-qualified PR preparation.

| Acceptance criterion | Design section |
| --- | --- |
| `AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.1` | [Identity and auto-start token](#identity-and-auto-start-token) |
| `AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.2` | [Automated launch gates](#automated-launch-gates) |
| `AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.3` | [Manual start](#manual-start) |
| `AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.4` | [Manual PR-link creation](#manual-pr-link-creation) |
| `AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.5` | [Identity and auto-start token](#identity-and-auto-start-token) |
| `AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.6` | [Task open](#task-open) |
| `AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.7` | [Task open](#task-open) |

## Trust boundary

A fork PR head controls files read by repository setup commands such as package
install scripts. Those commands run before the agent starts and can receive
executor-profile environment values. Provider validation proves the head
identity and target relationship; it does not establish trust in the source
code.

The [manual-start decision](../../../decisions/2026-09-23-fork-pr-review-watch-manual-start.md)
records why the watcher leaves fork tasks idle and why provider identity alone
does not authorize execution.

## Identity and auto-start token

`CheckReviewWatch` receives lightweight PRs from GitHub search and enriches
untracked results with `GetPR` before publishing review events. Enrichment
must carry the full response's head and base repository identity, head and
base revisions, mergeability details, and maintainer modification permission
alongside the existing branch and diff fields. A PR with branch names but no
head repository identity still needs enrichment.

`enrichPRDetails` returns only the PRs that are ready to publish. A PR whose
`GetPR` call fails or returns no PR is dropped from that poll: no event, no
reservation, and no task. The PR stays untracked, so the next poll searches
for it and enriches it again. Search results that already carry complete
identity skip `GetPR` and are always published.

A successful `GetPR` response without a head repository is provider data, not
a read failure. This happens when the fork was deleted. That PR is published
with empty head identity, and the launch decision remains fail-closed.
Search-result target identity remains the repository used to look up the PR.

`buildReviewTaskRequest` compares the PR head owner and repository name with the
target owner and name using case-insensitive GitHub identity. If both identities
are present and equal, it creates the normal one-shot auto-start token. If the
head differs or either identity is missing, it persists
`fork_pr_requires_manual_start` and does not create that token. The permanent
auto-start guard remains so competing watcher and workflow paths cannot bypass
the decision.

The check does not alter `checkout_branch`, `base_branch`, PR association,
comparison target, contribution binding, or Git push configuration.

## Automated launch gates

The task metadata marker is checked before deferred workflow launch and at the
central automatic `StartTask` boundary. Review-watch and workflow auto-start
paths leave the task available without preparing a worktree or running setup.
The central check protects other automated callers that reach `StartTask`
directly. A missing marker preserves existing behavior.

The marker is durable for the task. Manual launch does not clear it, so later
background automation remains blocked. This avoids converting one user action
into ongoing approval for future unattended runs.

## Manual start

A direct user start uses the existing non-automatic task launch path. It is not
blocked by `fork_pr_requires_manual_start`, so no approval screen, setting, or
new API field is required. The existing task state and PR association remain
available while the task waits.

## Task open

`EnsureSession` is the passive task-open entry point. When it has no session to
return and the task carries `fork_pr_requires_manual_start`, a passive
`session_open` request without an explicit automatic start (unset or false)
does not call `LaunchSession`, regardless of the resolved workflow intent. It
returns a successful response with no session ID, source `awaiting_manual_start`,
activation disposition `suppressed`, and activation reason
`fork_pr_manual_start`. It does not fall back to a workspace-only prepare. That
would launch the executor environment, and the setup script would run against
fork-controlled files. Step-derived automatic starts from other sources are
also suppressed. An explicit `auto_start: true` continues to reach the central
`StartTask` rejection.

The web client treats the waiting result as an empty chat state. The task page
and preview show a localized warning and a Start Task action at the bottom of
chat. Use the existing edit/start dialog and its launch handler. Direct entry
through Edit shows the same warning above the launch control. Opening or
dismissing the dialog leaves the task idle. The durable marker covers a known
fork or a missing source identity. The central StartTask rejection protects
other automated callers.

## Manual PR-link creation

Browser PR-link creation is a user-submitted task flow rather than a review
watch. It does not carry the review-watch marker. Existing explicit task
creation and launch behavior therefore remains unchanged for fork PR links.

## Verification

Backend tests cover search-result enrichment through review event creation,
deferral of PRs whose detail read failed until a later successful poll,
same-repository auto-start tokens, fork and missing-head identity suppression,
session-less passive task open for marked tasks in prepare and auto-start steps,
both review-watch auto-start paths, rejection at the central automatic launch
boundary, and successful manual start. Existing browser tests continue to
prove manual PR-link launch from a fork head.

## Related decisions

- [Fork PR review watch manual start](../../../decisions/2026-09-23-fork-pr-review-watch-manual-start.md)
- [Repository secrets](../../../decisions/2026-08-03-scope-and-merge-repository-secrets.md)
