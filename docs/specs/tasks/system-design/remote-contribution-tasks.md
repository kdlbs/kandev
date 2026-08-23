---
status: current
system: tasks
requirements:
  - REQ-TASKS-REMOTE-CONTRIBUTION-TASKS-001
created: 2026-08-04
updated: 2026-08-19
owners:
  - product
---
# Remote Contribution Tasks System Design

## Purpose and boundaries

This design record preserves the technical source for the capability mapped to REQ-TASKS-REMOTE-CONTRIBUTION-TASKS-001 while the task system completes its migration.

## Requirement mapping

| Requirement | Design source |
| --- | --- |
| REQ-TASKS-REMOTE-CONTRIBUTION-TASKS-001 | Migrated legacy design detail below |

## Migrated design source

## Why

Maintainers often need to help an external contributor finish a pull request or merge request. Today
they must manually reconstruct the target repository, contributor fork, head branch, and existing
review association before an agent can make a useful change. Kandev should accept the remote change
URL at task creation, prepare the contributor's exact branch, and push commits back to that branch
without making `create_task_kandev` materially larger.

The contributor can update or rewrite that branch after the task starts. Kandev keeps the task
checkout as the user's working version. The provider remains authoritative for the published change.
Kandev must preserve both versions and ask for user intent before one version replaces the other.

## What

- The existing `create_task_kandev.repositories[].repository_url` value accepts a canonical repository
  URL, GitHub pull request URL, or GitLab merge request URL. The tool adds no top-level or repository
  input properties; only the existing argument description changes.
- Kandev recognizes `https://github.com/<owner>/<repo>/pull/<number>` and
  `https://<configured-gitlab-host>/<project>/-/merge_requests/<iid>`. Query strings, fragments,
  embedded credentials, malformed paths, and unsupported providers are rejected.
- The backend resolves the change through the workspace's provider identity. The caller cannot assert
  the source repository, head branch, head SHA, target branch, or collaboration permission.
- Only open changes with a live source branch are accepted. Kandev validates branch names as Git refs
  before invoking Git.
- The task remains attached to the target repository. A versioned, non-secret contribution binding on
  that task-repository attachment identifies the existing change and its source repository and branch.
- The prepared checkout starts at the provider-reported head SHA on the contributor's head branch.
  `origin` continues to mean the target repository. A dedicated contribution remote points at the
  source repository. Normal pushes target the source branch without force.
- A same-repository change uses the target remote for both read and write. A fork change is accepted
  only when the provider reports that maintainers may update the source branch.
- GitHub pull requests and GitLab merge requests are associated with the new task before agent launch,
  so existing review, CI, and watch surfaces treat the remote change as already existing and do not
  create a second pull or merge request.
- Provider title and body remain untrusted remote content. They are not copied into the task title,
  description, trusted system context, or initial prompt. The agent receives only structured,
  server-authored contribution identity and branch guidance.
- Ordinary repository URLs retain their current behavior, including default branch resolution,
  normal `origin` pushes, and new-PR creation.
- Git status keeps two separate divergence concepts: `ahead`/`behind` compare the checkout with the
  target base branch, while `remote_ahead`/`remote_behind` compare it with its configured upstream.
  Push and pull affordances use the upstream-relative values; review scope and base-branch rebase
  affordances continue to use the base-relative values.
- For an associated contribution, Kandev compares the local HEAD and upstream snapshot with the
  provider's current commit history using commit identity and ancestry evidence. It never infers
  equivalence from commit message, author, timestamp, file statistics, or patch similarity.
- The Changes panel uses provider evidence only from a pull request whose repository and normalized
  head branch match the live checkout. Historical pull requests remain available in Review, but they
  do not affect Changes after the checkout moves to another branch.
- Associating a pull request with an ordinary task also reconciles the comparison base when the task
  is attached to the provider-reported head repository but the pull request targets another
  repository. Kandev stores that repository-qualified target on the exact matching task attachment,
  materializes a comparison-only remote, and keeps `origin` and push routing unchanged.
- A provider sync may update that target branch only for the same associated change. Another pull
  request, a repository-only match, or a historical sibling branch cannot replace it. Detaching the
  source association clears only its own target; merge and close retain the base comparison.
- Provider commit history is optional enrichment for the Changes panel. Kandev shares identical
  provider reads across Changes consumers and retries one failed read. If the retry fails, the panel
  keeps the checkout history and does not show a provider-history warning.
- Kandev reconciles provider and checkout commits by SHA. Shared commits keep the normal commit
  marker. Provider-only commits use the current-PR color and label. Checkout-only commits in a
  confirmed divergence use a separate local-checkout color and label. The accessible label carries
  the same meaning as the color.
- Provider-only commits appear newest first with the rest of the commit history. A complete provider
  history that contains local HEAD proves a provider-ahead relation even when no upstream is
  configured. Kandev does not offer Pull until the checkout has a configured upstream.
- When the provider history no longer contains local HEAD, the Changes panel keeps the task version as
  the primary version. A yellow warning icon in the Changes toolbar opens the available version
  actions; the panel body does not repeat the warning. The current provider history remains collapsed
  behind a **PR #<number> version** disclosure.
- A diverged task keeps local edit, commit, amend, reset, and review actions available. Generic Pull is
  unavailable because it requires a merge strategy. Push becomes an explicit **Replace PR branch**
  action instead of a disabled control.
- **Replace PR branch** requires a direct user action and an explicit destructive confirmation. The
  confirmation identifies the selected repository and the current provider version. Kandev replaces
  the remote branch only when its head still equals the confirmed provider head.
- The managed replacement action uses an exact force-with-lease condition. If the provider head moves
  after confirmation, Kandev changes neither version and asks the user to review the new state.
- The user can select **Use PR version** instead. Kandev requires a clean working tree, creates a local
  recovery branch at the current task HEAD, fetches the confirmed provider head, and resets the task
  branch to that head. The result shows the recovery branch name.
- Kandev exposes no replacement action through agent MCP tools or automatic Git operations. Generic
  contribution force-push requests remain rejected. Direct terminal commands remain outside this UI
  approval boundary.
- After equal heads are classified as aligned, a complete provider-ahead history that still contains
  local HEAD is a safe fast-forward case: the UI may offer Pull, but must not label the existing
  local commits as unpushed work. A local-ahead history whose tracked upstream equals the provider
  head may offer a normal Push for exactly `remote_ahead` commits.

Decisions:
[ADR-2026-08-04-remote-contribution-bindings](../../../decisions/2026-08-04-remote-contribution-bindings.md),
[ADR-2026-08-10-remote-contribution-head-drift](../../../decisions/2026-08-10-remote-contribution-head-drift.md),
[ADR-2026-08-12-local-first-contribution-replacement](../../../decisions/2026-08-12-local-first-contribution-replacement.md),
and
[ADR-2026-08-13-provider-history-changes-enrichment](../../../decisions/2026-08-13-provider-history-changes-enrichment.md).

## Data model

The target `task_repositories.metadata` JSON object may contain `remote_contribution`:

```json
{
  "version": 1,
  "provider": "github",
  "kind": "pull_request",
  "canonical_url": "https://github.com/acme/widget/pull/123",
  "number": 123,
  "state": "open",
  "base_branch": "main",
  "head_branch": "fix/widget",
  "head_sha": "0123456789abcdef",
  "source_repository": {
    "host": "github.com",
    "path": "contributor/widget",
    "provider_id": "optional-provider-repository-id",
    "remote_url": "https://github.com/contributor/widget.git"
  },
  "collaboration_allowed": true
}
```

`provider` is `github` or `gitlab`; `kind` is `pull_request` or `merge_request`; and `number` is the
GitHub PR number or GitLab project-scoped MR IID. The target repository is the attachment's existing
`repository_id`, not another copy inside the binding. `source_repository.remote_url` is canonical and
credential-free. The binding never stores access tokens, credential-helper state, lease IDs, provider
title/body, or other user-authored remote text.

The JSON field is versioned so later providers or collaboration attributes can be added without a
database migration. Unknown versions fail closed during materialization and credential authorization.

An ordinary repository task MAY also gain `metadata["comparison_target"]` after a matching pull
request is associated:

```json
{
  "version": 1,
  "provider": "github",
  "kind": "pull_request",
  "number": 1154,
  "head_branch": "fix/cursor-cost",
  "target_branch": "main",
  "head_repository": {
    "host": "github.com",
    "path": "contributor/widget",
    "provider_id": "provider-head-id",
    "remote_url": "https://github.com/contributor/widget.git"
  },
  "target_repository": {
    "host": "github.com",
    "path": "upstream/widget",
    "provider_id": "provider-target-id",
    "remote_url": "https://github.com/upstream/widget.git"
  }
}
```

The binding is credential-free and server-authored from provider data. It is stored only when the
head identity and normalized branch resolve to exactly one task-repository attachment. See
[ADR-2026-08-19-repository-qualified-comparison-targets](../../../decisions/2026-08-19-repository-qualified-comparison-targets.md).

## API surface

The `create_task_kandev` input schema keeps the same property set. The existing field is documented as:

> `repository_url`: Repository URL, GitHub pull request URL, or GitLab merge request URL.

The normal task response is unchanged. A provider-neutral internal resolver accepts the URL plus the
resolved workspace, returns the target repository input and validated contribution binding, and exposes
an association operation for the newly created task. Provider-specific API payloads do not cross that
internal boundary.

The user-facing WebSocket API adds two session-scoped actions:

- `worktree.replace_contribution` accepts `session_id`, an optional repository scope, and
  `expected_remote_head`. It replaces the contribution branch only when the exact lease matches.
- `worktree.use_contribution` accepts the same identity fields. It creates a local recovery branch and
  moves the clean task checkout to the confirmed provider head.

Both actions return the existing Git operation result. `worktree.use_contribution` also returns the
recovery branch name after a successful reset. Neither action appears in the agent MCP catalog.

## Permissions

- Existing MCP authentication, workspace reachability, workflow, profile, and executor checks still
  apply.
- Provider reads use the workspace-scoped GitHub or GitLab automation identity. A private contribution
  is unavailable when that identity cannot read both the target change and source repository.
- In managed GitHub credential mode, the broker may issue a source-repository scope only when the exact
  host and owner/repository match a validated `remote_contribution` binding on the session's linked task
  repository. The existing target-repository scope remains unchanged.
- In executor credential mode, Kandev does not mint credentials. Runtime preparation performs a
  non-mutating push preflight with the executor's effective Git credentials before starting the agent.
- GitLab uses the configured workspace connection for provider validation and the existing executor
  credential policy for Git transport. Self-hosted MR URLs must match the configured origin exactly.
- A user with access to the task can invoke the replacement actions from the task UI. The gateway and
  handler apply the existing session authorization before they access the execution.
- Agents, MCP callers, and automatic Git operations cannot invoke the managed replacement actions.

## Failure modes

| Condition                                                                                    | Observable behavior                                                                                                                      |
| -------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| URL is malformed, credential-bearing, or unsupported                                         | Task creation fails before persistence with an argument error.                                                                           |
| Provider connection cannot read the change or source repository                              | Task creation fails before persistence with an authorization/not-found error that does not reveal cross-workspace data.                  |
| Change is closed, merged, or has no live head                                                | Task creation fails before persistence and explains that only open contributions are supported.                                          |
| Provider returns an invalid head/base ref or inconsistent target identity                    | Task creation fails closed before any Git command.                                                                                       |
| Fork does not allow maintainer collaboration                                                 | Task creation fails before persistence with provider-specific remediation guidance.                                                      |
| Task persists but the existing-change association fails                                      | Kandev compensates the newly created task and returns failure; it does not launch an agent.                                              |
| Checkout SHA no longer matches the source branch during preparation                          | Launch fails without checking out or pushing a different revision; retry resolves fresh provider state.                                  |
| Provider branch advances after launch and still contains local HEAD                          | Kandev identifies a provider-ahead fast-forward, shows the current provider history, and offers Pull instead of Push.                    |
| Provider branch is rewritten after launch and no longer contains local HEAD                  | Kandev preserves the task version, shows a compact remote-change status, and offers user-controlled version choices.                     |
| Provider head changes after the replacement confirmation opens                               | The exact lease fails. Kandev does not change the remote branch and refreshes the shown provider state.                                  |
| Provider rejects the leased replacement                                                      | The task version remains unchanged. Kandev shows the provider or Git error.                                                              |
| The user selects **Use PR version** with local file changes                                  | Kandev does not reset the checkout. It asks the user to commit or discard the file changes first.                                        |
| The user selects **Use PR version** and the fetch does not match the confirmed provider head | Kandev does not create a recovery branch or reset the checkout. It refreshes the provider state.                                         |
| Current provider commits cannot be loaded                                                    | Kandev retries once, keeps the checkout history without a warning, and derives remote actions only from sufficient evidence.             |
| Effective Git credentials cannot dry-run a push to the source branch                         | The task remains durable, but the session does not start and exposes an actionable credential/collaboration error.                       |
| Contribution binding is missing, malformed, or an unknown version                            | Runtime preparation and managed source-scope issuance fail closed.                                                                       |
| Agent attempts a normal create-PR action                                                     | Kandev reuses the existing association and does not open a second remote change.                                                         |
| Associated PR head does not match exactly one task attachment                                | The PR remains linked for Review, but no comparison target changes; Kandev logs the scoped mismatch.                                    |
| Cross-repository comparison remote collides or cannot fetch                                  | Working-tree status remains available, comparison-derived data is explicitly unavailable, and Kandev never falls back to same-named `origin`. |

## Persistence guarantees

The contribution binding and GitHub PR or GitLab MR association survive backend restarts. New and reset
environments reconstruct the target checkout, contribution remote, upstream branch, and push routing
from the binding. Credential leases and preflight results are ephemeral and are recomputed on each
launch or resume. A moved or deleted source branch causes a later launch to fail visibly rather than
silently falling back to the target repository.

The original contribution `head_sha` remains creation-time provenance. It is not changed after a
provider rewrite. Live provider commits and Git status remain observed state. Drift detection does not
reset, rebase, merge, or replace either version without a direct user action.

A recovery branch created by **Use PR version** remains in the task repository across backend
restarts. Kandev does not persist confirmation dialogs, provider snapshots, or replacement leases.

Repository-qualified comparison targets survive backend and executor restarts. Their deterministic
remote/ref is reconstructed without changing `origin`, the checkout upstream, or push routing.

## Scenarios

### Create from a same-repository GitHub pull request

GIVEN an open GitHub pull request whose source and target repository are the same
WHEN `create_task_kandev` receives its URL as `repository_url`
THEN Kandev creates a task on the target repository, checks out the exact head branch and SHA, links the
existing pull request, and pushes future commits to that head branch

### Create from an editable GitHub fork pull request

GIVEN an open fork pull request whose author enabled maintainer edits
WHEN a maintainer creates a task from the pull request URL
THEN Kandev keeps `origin` on the target repository, configures the fork as the contribution remote,
authorizes only that validated source repository, and pushes normally to the contributor's head branch

### Reject a non-editable GitHub fork pull request

GIVEN a fork pull request whose author disabled maintainer edits
WHEN a maintainer creates a task from its URL
THEN no task is created and the result explains that the contributor must allow maintainer edits

### Create from an editable GitLab merge request

GIVEN an open merge request on the workspace's configured GitLab host whose source project permits
collaboration
WHEN `create_task_kandev` receives the merge request URL
THEN Kandev attaches the target project, checks out the source project branch, links the existing merge
request, and routes pushes to that source project

### Reject stale provider state

GIVEN a contribution was resolved but its source branch moved before worktree preparation
WHEN Kandev prepares the task
THEN preparation fails rather than checking out the new head or pushing from the stale SHA

### Show a provider fast-forward safely

GIVEN a running contribution task whose provider branch advances without rewriting local HEAD
WHEN Kandev loads the current provider commits
THEN the Changes panel treats the provider as ahead, does not mark the existing commits as local work
to push, and offers Pull rather than Push

### Keep a rewritten contribution local-first

GIVEN a running contribution task whose provider branch is force-pushed and no longer contains local
HEAD
WHEN Kandev loads the current provider commits
THEN the Changes panel keeps the task version primary, shows one warning icon in its toolbar, and offers
**Replace PR branch**, **Use PR version**, and **PR #<number> version** from the warning menu
AND each action explanation appears in an immediate tooltip on pointer hover
AND the panel body does not repeat the warning

### Ignore a historical pull request after the checkout changes branch

GIVEN a task retains a merged pull request for an earlier branch
AND a newer pull request exists for the current checked-out branch in the same repository
WHEN Kandev computes the Changes relation and provider history
THEN it uses only the newer pull request
AND the historical pull request remains available in Review without producing a Changes warning

### Replace the PR branch with the task version

GIVEN a diverged contribution and a confirmed provider head
WHEN the user confirms **Replace PR branch**
THEN Kandev replaces the remote branch only when the exact provider-head lease still matches

### Reject a stale replacement lease

GIVEN the replacement confirmation names provider head A
AND the provider branch advances to head B
WHEN the user confirms **Replace PR branch**
THEN Kandev leaves head B unchanged and shows the refreshed remote-change state

### Use the current PR version with recovery

GIVEN a diverged contribution with a clean working tree
WHEN the user confirms **Use PR version**
THEN Kandev creates a local recovery branch at the prior task HEAD and moves the task branch to the
confirmed provider head

### Preserve local file changes

GIVEN a diverged contribution with staged, unstaged, or untracked file changes
WHEN the user selects **Use PR version**
THEN Kandev keeps the checkout unchanged and asks the user to commit or discard those changes

### Provide the same choice on mobile

GIVEN a diverged contribution on a phone viewport
WHEN the user opens the Git actions menu
THEN the user can replace the PR branch, use the PR version, or inspect the PR version without a desktop
workflow

### Keep ordinary local-ahead work pushable

GIVEN a contribution checkout whose upstream equals the provider's current head
AND the maintainer creates commits on top
WHEN Kandev computes Git status
THEN Push reports only the commits absent from the upstream and the provider commits are not duplicated

### Keep Changes usable when provider history is unavailable

GIVEN Kandev cannot load the current provider commit list
WHEN the Changes panel renders the checkout
THEN it retries the provider read once and keeps the local checkout history without a warning
AND it does not claim the branch was rewritten or compare commits by message or patch similarity

### Distinguish commits when the provider is ahead

GIVEN a complete provider history contains local HEAD and one newer provider commit
WHEN the Changes panel reconciles the commit lists
THEN it shows the provider-only commit first within its repository with the current-PR color and accessible label
AND it keeps shared commits neutral and does not mark them as local work to push
AND it does not offer Pull when the checkout has no configured upstream

### Preserve ordinary repository creation

GIVEN an ordinary GitHub, GitLab, or provider-neutral repository URL
WHEN it is passed as `repository_url`
THEN Kandev follows the existing repository task path without a contribution binding or source scope

### Reconcile an ordinary fork after PR creation

GIVEN an ordinary task attached to a contributor fork whose `main` is stale
AND the checked-out feature branch opens a PR against `upstream/widget:main`
WHEN Kandev associates the provider PR with the task
THEN only the attachment whose repository and checkout branch match the PR head stores the upstream
comparison target
AND Changes, commits, cumulative diff, Review, and task-card totals use the upstream ref without
rewriting `origin` or the push destination

### Keep the MCP catalog compact

GIVEN the external MCP catalog before and after this feature
WHEN clients inspect `create_task_kandev`
THEN its input property names and count are unchanged and only the existing `repository_url` description
mentions pull and merge request URLs

## Out of scope

- A new task-creation UI for pasting pull or merge request URLs.
- Creating tasks from issues, review comments, or arbitrary commit URLs.
- Azure DevOps or additional source-control providers.
- Multiple remote contributions in one create call.
- Automatic force pushes, branch renames, retargeting, merging, or collaboration-setting changes.
- Automatic reset, rebase, merge, or remote replacement after provider drift.
- Automatic replay or deduplication of task commits across rewritten provider history.
- Bulk replacement across multiple repositories from one confirmation.
- GitLab mobile or Changes-panel drift presentation until that surface supplies complete MR-head
  evidence. The backend replacement operations remain provider-neutral.
- Copying remote titles, bodies, comments, or diffs into trusted prompts.
- Guaranteeing write access after credentials or provider permissions change during a running session.
- Expanding task Git credentials so an otherwise unreadable private target can be fetched.