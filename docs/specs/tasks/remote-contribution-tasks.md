---
status: approved
created: 2026-08-04
owner: product
---

# Remote Contribution Tasks

## Why

Maintainers often need to help an external contributor finish a pull request or merge request. Today
they must manually reconstruct the target repository, contributor fork, head branch, and existing
review association before an agent can make a useful change. Kandev should accept the remote change
URL at task creation, prepare the contributor's exact branch, and push commits back to that branch
without making `create_task_kandev` materially larger.

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

Decision: [ADR-2026-08-04-remote-contribution-bindings](../../decisions/2026-08-04-remote-contribution-bindings.md).

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
| Effective Git credentials cannot dry-run a push to the source branch | The task remains durable, but the session does not start and exposes an actionable credential/collaboration error. |
| Contribution binding is missing, malformed, or an unknown version | Runtime preparation and managed source-scope issuance fail closed. |
| Agent attempts a normal create-PR action | Kandev reuses the existing association and does not open a second remote change. |

## Persistence guarantees

The contribution binding and GitHub PR or GitLab MR association survive backend restarts. New and reset
environments reconstruct the target checkout, contribution remote, upstream branch, and push routing
from the binding. Credential leases and preflight results are ephemeral and are recomputed on each
launch or resume. A moved or deleted source branch causes a later launch to fail visibly rather than
silently falling back to the target repository.

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
- Force pushes, branch renames, retargeting, merging, or changing collaboration settings.
- Copying remote titles, bodies, comments, or diffs into trusted prompts.
- Guaranteeing write access after credentials or provider permissions change during a running session.
