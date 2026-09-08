---
status: active
system: integrations
created: 2026-08-06
updated: 2026-08-06
owners:
  - tbd
---
# GitLab Workflow Sync Requirements

## Overview

Workflow sync keeps a workspace's workflows in lockstep with definition files committed to a repository, so workflow changes are reviewed, versioned, and rolled out like code. Today that path exists only for GitHub: the `workflowsync` service reaches the repository through a `ClientProvider` interface whose methods are typed against `github.RepoContentEntry` and whose sole implementation is `github.Service`.

## Requirements

### REQ-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-001: GitLab Workflow Sync

**Intent:** Workflow sync keeps a workspace's workflows in lockstep with definition files committed to a repository, so workflow changes are reviewed, versioned, and rolled out like code. Today that path exists only for GitHub: the `workflowsync` service reaches the repository through a `ClientProvider` interface whose methods are typed against `github.RepoContentEntry` and whose sole implementation is `github.Service`.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-001.1:** **One sync source per workspace.** The provider is a property of the single existing config row, not a new dimension.
- **AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-001.2:** **GitLab targets are addressed by `project_path`.** GitHub keeps `repo_owner` + `repo_name`.
- **AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-001.3:** **The GitLab host comes from the workspace's existing GitLab connection.** The sync config stores no host of its own.

## Migrated source detail

## Why

Workflow sync keeps a workspace's workflows in lockstep with definition files
committed to a repository, so workflow changes are reviewed, versioned, and
rolled out like code. Today that path exists only for GitHub: the
`workflowsync` service reaches the repository through a `ClientProvider`
interface whose methods are typed against `github.RepoContentEntry` and whose
sole implementation is `github.Service`.

Kandev already ships a complete GitLab integration — per-workspace tokens,
self-managed hosts, MRs, issues, watches — and repositories already carry
provider identity (`provider`, `provider_host`, `provider_owner`,
`provider_name`). A team whose code lives on GitLab can connect GitLab
everywhere else in Kandev but cannot sync workflows from it, so they are pushed
back to manual workflow editing with no version control.

This is a capability gap, not a defect: the GitLab path was never built.

## What

A workspace can configure workflow sync against **either** GitHub or GitLab.
The user selects the provider, points at a repository, branch, and directory,
and the existing poller and "Sync now" behavior apply GitLab-hosted workflow
definition files exactly as they apply GitHub-hosted ones.

Decisions taken (see `## Decisions` for rationale):

- **One sync source per workspace.** The provider is a property of the single
  existing config row, not a new dimension.
- **GitLab targets are addressed by `project_path`.** GitHub keeps
  `repo_owner` + `repo_name`.
- **The GitLab host comes from the workspace's existing GitLab connection.**
  The sync config stores no host of its own.

Everything downstream of fetching files is unchanged: parsing, validation,
warnings, the apply/reconcile diff, `workflows.source` ownership, release on
config delete, and the recorded sync status.

## Decisions

### One sync source per workspace

`workflow_sync_configs` keeps `workspace_id` as its primary key. A new
`provider` column selects the source. Switching provider replaces the config.

The apply path (`ApplySyncedWorkflows`) reconciles the *complete* set of synced
workflows for a workspace against one fetched file set — a workflow that is
synced but absent from the file set is deleted. Two concurrent sources would
each see the other's workflows as deletions. Supporting multiple sources means
redesigning ownership so each synced workflow records which source owns it;
that is out of scope.

### GitLab targets use `project_path`

GitLab projects live at arbitrarily nested namespace paths
(`group/subgroup/project`). `repo_owner` and `repo_name` both reject slashes
today, and the `gitlab.Client` interface takes a single `projectPath` argument
on every method. Overloading `repo_owner` to hold a namespace would make the
field's meaning provider-dependent and would require relaxing slash validation
for both providers.

A new `project_path` column (`TEXT NOT NULL DEFAULT ''`) is populated when
`provider = "gitlab"` and holds an empty string for GitHub rows. Validation
is provider-conditional: GitHub requires `repo_owner` and `repo_name` and
forbids `project_path`; GitLab requires `project_path` and forbids
`repo_owner`/`repo_name`.

### Host comes from the workspace GitLab connection

`gitlab.Service.ClientForWorkspace(ctx, workspaceID)` already resolves the
workspace's configured host, auth method, and credential, with a revision-keyed
cache. Workflow sync routes through it and therefore supports self-managed
GitLab with no new configuration surface. A workspace with no GitLab connection
gets a clear, actionable failure rather than a partial config.

## Data Model

### `workflow_sync_configs`

Two idempotent `ADD COLUMN` migrations, following the existing
`addPollEnabledColumn` pattern in `store.go`:

| Column | Type | Default | Notes |
| --- | --- | --- | --- |
| `provider` | TEXT NOT NULL | `'github'` | `github` or `gitlab`. The default backfills every existing row to its current implicit meaning. |
| `project_path` | TEXT NOT NULL | `''` | GitLab namespace path (`group/subgroup/project`). Empty for GitHub. |

`repo_owner` and `repo_name` stay `NOT NULL` and hold empty strings for GitLab
rows. No existing column changes type, and no existing row changes meaning.

### `Config` / `SetConfigRequest`

Both gain `Provider string` and `ProjectPath string`. `Normalize()` becomes
provider-conditional:

- `provider` empty defaults to `github` (preserves existing API clients).
- `provider` not in {`github`, `gitlab`} → `ErrInvalidConfig`.
- `github`: `repo_owner` and `repo_name` required, no slashes or spaces
  (unchanged); `project_path` must be empty.
- `gitlab`: `project_path` required; must contain at least one `/`, no spaces,
  no empty segments, no `.` or `..` segment, and no leading/trailing slash;
  `repo_owner` and `repo_name` must be empty.
- `branch`, `path`, `interval_seconds`, `poll_enabled` validate identically for
  both providers.

## API Surface

### Backend

`workflowsync.ClientProvider` is replaced by two provider-specific interfaces.
Each keeps its own upstream listing shape at the boundary (`[]github.RepoContentEntry`
for GitHub, `[]gitlab.RepoTreeEntry` for GitLab), and `workflowsync` converts
both to a provider-neutral `dirEntry` inside its fetch loop — provider-typed
values never leak past that conversion:

```go
type GitHubClientProvider interface {
    ListRepoDirectoryForWorkspace(ctx, workspaceID, owner, repo, path, ref string) ([]github.RepoContentEntry, error)
    GetRepoFileContentForWorkspace(ctx, workspaceID, owner, repo, path, ref string) ([]byte, error)
}

type GitLabClientProvider interface {
    ListRepoTreeForWorkspace(ctx, workspaceID, projectPath, path, ref string) ([]gitlab.RepoTreeEntry, error)
    GetRepoFileContentForWorkspace(ctx, workspaceID, projectPath, path, ref string) ([]byte, error)
}
```

`Service.fetchFiles` dispatches on `cfg.Provider`. A nil provider client for the
configured provider produces a provider-specific, actionable error — the
existing hardcoded "GitHub is not authenticated" string becomes conditional.

`workflowsync.Provide` and `initWorkflowSyncService` take both providers.

### `gitlab.Client`

Two new methods, matching the package's `projectPath`-first convention:

```go
ListRepoTree(ctx context.Context, projectPath, path, ref string) ([]RepoTreeEntry, error)
GetRepoFileContent(ctx context.Context, projectPath, path, ref string) ([]byte, error)
```

Backed by `GET /projects/:id/repository/tree` (non-recursive, paginated) and
`GET /projects/:id/repository/files/:file_path/raw`. Implemented in
`pat_client.go`, `mock_client.go`, and `noop_client.go`; `glab_client.go`
inherits via its embedded `*PATClient`.

`gitlab.Service` gains workspace-routed wrappers that resolve the client via
`ClientForWorkspace` and satisfy `GitLabClientProvider`.

### HTTP

`/api/v1/workflow-sync/config` GET and POST payloads gain `provider` and
`project_path`. Omitting `provider` on POST means `github`, so existing clients
are unaffected. Routes, methods, and status codes are unchanged.

## Permissions

Unchanged from the GitHub path. Workflow sync is workspace-scoped; the caller
supplies `workspace_id` and the standard workspace authorization applies.

GitLab credential resolution is delegated entirely to
`ClientForWorkspace`, which reads the workspace's own connection — a workspace
can only ever sync from a GitLab project its own token can read. No
cross-workspace credential reuse is introduced.

GitHub's `ensureRepositoryInWorkspaceScope` allowlist has no GitLab equivalent
because the GitLab integration has no App-installation or repo-scope-mode
concept; the workspace token is itself the scope boundary.

## Failure Modes

| Condition | Behavior |
| --- | --- |
| `provider = gitlab`, workspace has no GitLab connection | Sync fails with an actionable message naming GitLab; recorded via `recordFailure`, `last_hash` cleared. |
| GitLab token lacks read access to the project | Underlying 403/404 wrapped with project path, branch, and directory, same shape as the GitHub error. |
| Configured directory missing on the branch | 404 from the tree endpoint surfaces as a sync failure, matching GitHub behavior. |
| Directory exists but is empty | Successful sync applying an empty file set — deletes previously-synced workflows. Matches GitHub. |
| A file fails to parse | Warning recorded; that file's previously-synced workflow is left untouched. Unchanged. |
| GitLab host unreachable / TLS failure | Wrapped fetch error; failure recorded, poller continues on the next tick. |
| Invalid provider value in a POST | HTTP 400 via `ErrInvalidConfig`. |
| Legacy row with empty `provider` | Read as `github`. |

Every GitLab failure follows the existing contract: recorded on the config row,
logged, never fatal to the poller.

## Persistence Guarantees

- The per-workspace mutex still serializes config mutations against in-flight
  syncs, including across the GitLab fetch.
- Switching provider is a single upsert; the new provider's first sync
  reconciles from scratch because `last_hash` no longer matches.
- Deleting a config releases synced workflows to manual ownership before the
  row is removed, regardless of provider.
- Both migrations are idempotent and tolerate replay on an existing database.
- Existing GitHub rows are semantically unchanged after migration.

## Scenarios

1. **Configure GitLab sync.** A workspace with a GitLab connection sets
   `provider=gitlab`, `project_path=group/subgroup/project`, branch `main`,
   path `.kandev/workflows`. The first sync creates the workflows defined
   there, marked as synced and read-only.

2. **Poller applies a GitLab commit.** A definition file changes upstream. On
   the next tick the poller fetches, diffs, and updates only the changed
   workflow, broadcasting the update.

3. **Self-managed GitLab.** A workspace connected to `gitlab.internal.example`
   syncs from it without any host field on the sync config.

4. **Switch GitHub → GitLab.** A workspace with an existing GitHub sync config
   POSTs a GitLab config. The row is replaced; the next sync reconciles the
   workspace's synced workflows against the GitLab file set.

5. **Missing GitLab connection.** A workspace with no GitLab token configures
   GitLab sync. Saving succeeds; syncing fails with a message directing the
   user to connect GitLab. Status is visible in the settings UI.

6. **Existing GitHub workspace is untouched.** After deploying this change, a
   workspace already syncing from GitHub continues on the same schedule with
   the same results and no user action.

7. **Nested subgroup project.** `project_path` with two or more segments
   resolves correctly, exercising URL-encoding of the project reference.

## Out Of Scope

- Multiple simultaneous sync sources per workspace.
- Azure DevOps or any other provider, though the two-interface split leaves
  room for one.
- Recursive directory sync — still non-recursive, matching GitHub.
- Webhook-driven sync; polling and manual sync only.
- Writing workflow definitions back to the repository.
- Auto-deriving the sync target from a workspace's connected repository row.

## Implementation Plan

See `docs/plans/gitlab-workflow-sync/plan.md`.
