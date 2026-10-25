---
status: current
system: tasks
requirements:
  - REQ-TASKS-REMOTE-CONTRIBUTION-TASKS-003
created: 2026-09-12
updated: 2026-09-12
owners:
  - product
---

# Branch History Explanations System Design

## Purpose and boundaries

This design extends [remote contribution tasks](remote-contribution-tasks.md).
It covers ordinary tasks with a matching PR and imported contribution tasks.
The existing relation classifier and replacement operations retain their authority.
This design replaces the older warning wording and action labels in that design.
Other contribution contracts remain unchanged.

The diagnostic case was a local rebase with five rewritten commits and 29 newer
base commits. The published branch retained the original five commits.
The current warning incorrectly attributed the change to the PR branch.

## Requirement mapping

| Requirement criteria | Design section |
| --- | --- |
| 003.1, 003.2, 003.3 | Evidence and explanation |
| 003.4, 003.6 | Responsive presentation |
| 003.5, 003.7 | Identity and safety |
| 003.8 | Failure and compatibility |

All criteria belong to REQ-TASKS-REMOTE-CONTRIBUTION-TASKS-003.

## Existing components

- `classifyRemoteContribution` in `apps/web/hooks/domains/session/remote-contribution-relation.ts` owns the relation and action capabilities.
- `useRemoteContributionRelation` selects a branch-scoped PR and combines provider evidence with session Git status.
- `RemoteContributionHeaderActions`, `RemoteContributionActionItems`, and `RemoteContributionResolutionDialog` own current warning and resolution UI.
- `apps/web/components/task/mobile/mobile-changes-panel.tsx` wires the shared Changes header and history into the phone surface.
- `GitHandlers` in `apps/backend/internal/agent/handlers/git_handlers.go` routes session Git requests through the runtime agentctl client.
- `GitOperator` in `apps/backend/internal/agentctl/server/process/git.go` performs Git work in the selected executor repository.

## Evidence and explanation

Add a read-only `worktree.contribution_history_explanation` WebSocket request.
Use the existing session authorization and agentctl forwarding path.
Add `POST /api/v1/git/contribution/history-explanation` at agentctl.
These names and the typed payload below are proposed additions.
Neither endpoint belongs in the agent MCP catalog.

The request contains `session_id`, `repo`, `branch`, `expected_local_head`,
and `expected_remote_head`. Heads must be full hexadecimal commit IDs.
The selected branch and repository come from existing branch-scoped selection.
The executor resolves the attached repository through `gitOpForRepo`.
No caller-supplied filesystem path, remote URL, or shell expression is accepted.

The response contains the same identity and heads, plus:

| Field | Values or meaning |
| --- | --- |
| `kind` | `local_rebase` or `unexplained` |
| `reason` | `matched_reflog`, `no_matching_reflog`, `missing_objects`, `ambiguous`, `stale`, `limit`, or `unavailable` |
| `onto_head` | Optional resolved commit onto which the rebase finished |
| `task_commit_count` | Optional commits after `onto_head` in the current task history |
| `published_commit_count` | Optional commits after the old merge base in the published history |
| `new_base_commit_count` | Optional commits between the old merge base and `onto_head` |

Counts are independent exact graph counts. They do not attest patch equivalence.
The response contains no raw reflog messages, author identities, diffs, or paths.

### Bounded local observation

Run only when a confirmed diverged explanation surface opens. Share one in-flight
request for an identical identity and head pair across mounted consumers.
Do not add work to Git status polling or perform a network fetch.

Use structured argument arrays, a controlled locale, and the existing owned Git
process path. Bound the operation to two seconds, 200 reflog entries per log,
1 MiB of captured output, and 2,000 enumerated commits per count.
If a bound is reached, return `unexplained` or omit the affected count.
Cancellation must terminate the owned process and return promptly.

A local-rebase explanation requires all of these facts:

1. The executor branch and HEAD match the request at the start and end of the read.
2. The latest branch transition ends at the requested local head and identifies a completed rebase.
3. The transition starts at the requested published head, using the adjacent reflog identity.
4. The worktree HEAD reflog contains the corresponding completed rebase sequence and a resolvable onto commit.
5. The onto commit is an ancestor of the new head. All referenced commits exist locally.
6. No intervening branch transition, unfinished rebase, or ambiguous sequence contradicts that observation.

Read reflogs through Git, including linked-worktree HEAD reflogs. Do not assume
that `.git` is a directory. Unsupported Git reflog forms use neutral wording.
Reflog evidence describes an observed local operation. It cannot prove user
intent, validation status, or content equivalence.

For counts, require a unique old merge base between the published head and onto
commit. Require linear old and new topic ranges and an ancestor relationship
from that merge base to onto. Omit counts for merges or ambiguous ranges.
Count the topic ranges separately and the base range separately.
Conflict resolution can change patches while the local-rebase explanation remains valid.
No subject matching or range-diff similarity threshold participates in permissions.

Conservative first-release exclusions include rebase followed by another commit,
expired reflogs, squash operations, and missing published objects. These cases
still receive the neutral explanation and all existing resolution capabilities.

## Identity and safety

The browser accepts a result only while workspace, session, repository, branch,
selected PR, local head, and current provider head still match its request key.
A new provider sync invalidates the result until current evidence returns.
The backend checks executor branch and head again before returning evidence.
The browser discards a late response after navigation or head movement.

Do not persist explanation evidence or modify task contribution metadata.
Cache only within mounted consumers. Dismissal releases the result and cancellation
subscription. A new opening starts a fresh observation.
Older executors without the endpoint return the neutral explanation.

`remoteContributionActionPolicy` remains independent of explanation fields.
Exact provider-head leases, recovery branches, clean-tree checks, and explicit
confirmation retain their existing implementations. A detected rebase never
changes the relation to aligned or removes either version from the history.
The UI never claims that an agent is validating solely because its session runs.

## Responsive presentation

The common title is “Task and PR histories differ”. Neutral body copy is
“The task and published PR contain different histories. Compare them before choosing a version.”
The local-rebase body is “The task branch was rebased locally. The PR still contains the earlier history.”
Guidance is “Publish the task version after validation finishes.” This is advice,
not a claim about current validation progress.

Exact count rows use separate pluralized translation keys:
“Task commits: 5”, “Published commits: 5”, and “Newer base commits: 29”.
Do not substitute upstream ahead/behind totals into these rows.
During observation, retain the neutral body with “Checking local history...”.
On failure, remove the progress text and retain the neutral body without another alert.

`Compare versions` is first and visually primary. It closes the menu, opens the
existing Changes surface, expands the provider-version disclosure, and focuses
the version heading. Keep task history visible with its existing local actions.
The provider history remains read-only. This is a history comparison, not a new
patch-matching editor. Preserve the existing provider link as “Open PR on GitHub”.

`Publish task version...` and `Restore published PR version...` retain destructive
consequence text. Publication replaces published history. Restoration creates a
recovery branch and replaces the current checkout history. Show those effects
inside confirmations and inline on touch surfaces.

Desktop uses the existing compact toolbar trigger and anchored menu. Phone and
coarse-pointer users receive an inset bottom `Drawer` with the same view model.
The nearest shipped exemplar is `MobileMenuSheet` in
`apps/web/components/kanban/mobile-menu-sheet.tsx`. Reuse its inset drawer treatment.
The existing phone Changes header supplies the entry point and repository context.
The drawer closes before comparison navigation or confirmation opens.

The drawer has one internal vertical scroll owner, safe-area padding, and a
maximum height based on `dvh`. Touch actions have at least 44px hit targets.
Desktop controls retain existing compact dimensions. Labels wrap on phones.
Escape, dismiss, and Back return focus to the initiating trigger.
Comparison uses the existing phone Changes view with vertically ordered histories.
No side-by-side desktop pane is mounted and hidden on phones.

All copy uses `t()` or `<Trans>`. Update English, Portuguese, and Simplified
Chinese, then generate Traditional Chinese and pseudo catalogs through repo scripts.

## Failure and compatibility

Unknown evidence does not alter Git action permissions or create a second warning.
Missing endpoint, timeout, cancellation, shallow history, and offline executors
preserve the existing Changes view. There is no automatic fetch or retry loop.
Confirmed divergence still offers comparison when optional explanation evidence fails.
Provider evidence failures continue to obey the existing unknown-relation policy.
A branch without a matching PR receives no contribution explanation.
Aligned, provider-ahead, and local-ahead relations retain existing UI behavior.

## Observability and decisions

Use existing request diagnostics with duration and the bounded reason enum.
Do not log reflog text, credentials, patches, or user-entered branch descriptions.
No new metric family, database schema, feature flag, or background worker is needed.

The [local-first decision](../../../decisions/2026-08-12-local-first-contribution-replacement.md)
and [head-drift decision](../../../decisions/2026-08-10-remote-contribution-head-drift.md)
retain their mutation and commit-identity rules. This design adds diagnostic
explanation only. Its rationale fits this feature design, so no new ADR is required.

## Implementation plans

- [Branch history explanations](../../../plans/branch-history-explanations/plan.md)
