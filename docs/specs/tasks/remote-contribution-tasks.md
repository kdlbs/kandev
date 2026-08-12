---
status: approved
created: 2026-08-04
updated: 2026-08-10
owner: product
---

# Remote Contribution Tasks

## Why

Maintainers often need to help an external contributor finish a pull request or merge request. Today
they must manually reconstruct the target repository, contributor fork, head branch, and existing
review association before an agent can make a useful change. Kandev should accept the remote change
URL at task creation, prepare the contributor's exact branch, and push commits back to that branch
without making `create_task_kandev` materially larger.

The contributor can update or rewrite that branch after the task starts. Kandev must not present the
old checkout and the provider's current change as one continuous history, label old contributor
commits as maintainer-authored work waiting to be pushed, or offer a remote mutation whose safety has
not been established. It must preserve local work while making the drift explicit.

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
  `origin` continues to mean the target repository; a dedicated contribution remote points at the
  source repository. Push operations for that attachment target the source branch without force.
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
- When the provider history no longer contains the local HEAD and the histories cannot be proven to be
  a safe local-ahead or provider-ahead relationship, the Changes panel shows the current remote change
  and local checkout as separate histories. Local work remains intact. Push, force-push, and generic
  pull actions are unavailable until the histories are reconciled outside this flow.
- A provider-ahead history that still contains local HEAD is a safe fast-forward case: the UI may offer
  Pull, but must not label the existing local commits as unpushed work. A local-ahead history whose
  tracked upstream equals the provider head may offer a normal Push for exactly `remote_ahead` commits.

Decisions:
[ADR-2026-08-04-remote-contribution-bindings](../../decisions/2026-08-04-remote-contribution-bindings.md)
and
[ADR-2026-08-10-remote-contribution-head-drift](../../decisions/2026-08-10-remote-contribution-head-drift.md).

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

## API surface

The `create_task_kandev` input schema keeps the same property set. The existing field is documented as:

> `repository_url`: Repository URL, GitHub pull request URL, or GitLab merge request URL.

The normal task response is unchanged. A provider-neutral internal resolver accepts the URL plus the
resolved workspace, returns the target repository input and validated contribution binding, and exposes
an association operation for the newly created task. Provider-specific API payloads do not cross that
internal boundary.

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

## Failure modes

| Condition | Observable behavior |
|---|---|
| URL is malformed, credential-bearing, or unsupported | Task creation fails before persistence with an argument error. |
| Provider connection cannot read the change or source repository | Task creation fails before persistence with an authorization/not-found error that does not reveal cross-workspace data. |
| Change is closed, merged, or has no live head | Task creation fails before persistence and explains that only open contributions are supported. |
| Provider returns an invalid head/base ref or inconsistent target identity | Task creation fails closed before any Git command. |
| Fork does not allow maintainer collaboration | Task creation fails before persistence with provider-specific remediation guidance. |
| Task persists but the existing-change association fails | Kandev compensates the newly created task and returns failure; it does not launch an agent. |
| Checkout SHA no longer matches the source branch during preparation | Launch fails without checking out or pushing a different revision; retry resolves fresh provider state. |
| Provider branch advances after launch and still contains local HEAD | Kandev identifies a provider-ahead fast-forward, shows the current provider history, and offers Pull instead of Push. |
| Provider branch is rewritten after launch and no longer contains local HEAD | Kandev preserves the checkout, separates local and provider histories, explains the drift, and disables unsafe remote actions. |
| Current provider commits cannot be loaded | Kandev does not assert that a rewrite occurred. It shows the provider error and derives Push/Pull only from available upstream evidence. |
| Effective Git credentials cannot dry-run a push to the source branch | The task remains durable, but the session does not start and exposes an actionable credential/collaboration error. |
| Contribution binding is missing, malformed, or an unknown version | Runtime preparation and managed source-scope issuance fail closed. |
| Agent attempts a normal create-PR action | Kandev reuses the existing association and does not open a second remote change. |

## Persistence guarantees

The contribution binding and GitHub PR or GitLab MR association survive backend restarts. New and reset
environments reconstruct the target checkout, contribution remote, upstream branch, and push routing
from the binding. Credential leases and preflight results are ephemeral and are recomputed on each
launch or resume. A moved or deleted source branch causes a later launch to fail visibly rather than
silently falling back to the target repository.

The original contribution `head_sha` remains creation-time provenance. It is not mutated to pretend an
existing checkout follows a later provider rewrite. Live provider commits and Git status are observed
state. Neither observation automatically resets, rebases, merges, or deletes the task checkout.

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

### Separate a rewritten provider history

GIVEN a running contribution task whose provider branch is force-pushed and no longer contains local
HEAD
WHEN Kandev loads the current provider commits
THEN the Changes panel warns that the contribution changed, renders "Current PR commits" separately
from "Local checkout commits", preserves every local commit, and offers no Push, force-push, or generic
Pull action

### Keep ordinary local-ahead work pushable

GIVEN a contribution checkout whose upstream equals the provider's current head
AND the maintainer creates commits on top
WHEN Kandev computes Git status
THEN Push reports only the commits absent from the upstream and the provider commits are not duplicated

### Avoid guessing when provider history is unavailable

GIVEN Kandev cannot load the current provider commit list
WHEN the Changes panel renders the checkout
THEN it reports that provider history is unavailable, does not claim the branch was rewritten, and
does not compare commits by message or patch similarity

### Preserve ordinary repository creation

GIVEN an ordinary GitHub, GitLab, or provider-neutral repository URL
WHEN it is passed as `repository_url`
THEN Kandev follows the existing repository task path without a contribution binding or source scope

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
- Kandev performing force pushes, branch renames, retargeting, merging, or changing collaboration
  settings.
- Automatically resetting, rebasing, merging, or replacing an existing checkout after provider drift.
- A guided history-reconciliation workflow; this iteration detects and contains the unsafe state.
- Copying remote titles, bodies, comments, or diffs into trusted prompts.
- Guaranteeing write access after credentials or provider permissions change during a running session.
