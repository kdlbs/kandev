# Forgejo integration plan

## Goal

Add a workspace-scoped Forgejo integration so a Kandev workspace can connect
directly to a self-hosted Forgejo instance, import repository issues into the
Kandev task board, and link/create Forgejo pull requests from Kandev tasks.

The first release makes Kandev the execution Kanban and Forgejo the repository
system of record. It does not attempt a second, continuously mirrored Kanban.

## Decisions

- Implement a native `internal/forgejo` integration against Forgejo REST API
  v1. Do not route Forgejo through the GitHub integration: GitHub Apps,
  GitHub OAuth, and GitHub webhook contracts are incompatible.
- Do not depend on `fj`, `tea`, or any other local CLI. They remain optional
  operator/agent tools. The server integration must be portable and use HTTP.
- Use a Forgejo scoped personal access token (PAT), stored only in Kandev's
  workspace secret store. Keep Forgejo API access distinct from Git push
  credentials; do not automatically inject the PAT into an agent process.
- Use manual refresh and bounded polling in v1. Add signed Forgejo webhooks
  only after the task/issue/PR mapping is established and tested.
- Support one Forgejo origin and one token per Kandev workspace. A workspace
  may use many Forgejo repositories on that origin.
- The initial board contract is one-way import: an open Forgejo issue may
  create or link one Kandev task; Kandev task state does not close/reopen the
  Forgejo issue automatically. PR state is reflected on the linked Kandev
  task. Explicit user actions perform external mutations.

## API review and compatibility contract

Forgejo exposes a versioned REST API at `<origin>/api/v1` and publishes the
instance-specific OpenAPI document at `<origin>/swagger.v1.json`. The runtime
client must read the server version during connection test and reject an
unsupported major version with a clear error rather than assuming Gitea or
GitHub behavior.

Use `Authorization: token <PAT>` for a personal access token. The initial
token request must ask for the least privilege that supports the selected
repositories:

- `read:repository` and `read:issue` for connection, discovery, import, and
  status reads;
- `write:repository` only when the workspace enables PR creation/updates;
- `write:issue` only when the workspace enables issue comments or issue
  creation in a later milestone.

The client must use `page` and `limit`, honor the response `Link` header, and
surface `x-total-count` where the UI needs a count. It must not assume a fixed
maximum page size; Forgejo exposes the instance limit through
`/api/v1/settings/api`.

The v1 endpoint set to validate against the configured instance OpenAPI file:

- connection and identity: `GET /api/v1/user`, version/settings endpoints;
- repository discovery: the authenticated-user repository list/search routes;
- issues: repository issue list/get and issue state updates;
- pull requests: list/get, create, branch lookup, and mergeability/status
  routes needed by the task link surface;
- optional phase-two webhook registration and delivery endpoints.

Forgejo webhook deliveries identify the event with `X-Forgejo-Event`, the
delivery with `X-Forgejo-Delivery`, and carry an HMAC signature in
`X-Forgejo-Signature`. Phase two must verify the signature against a Kandev
secret and deduplicate delivery IDs before changing any task state.

## Architecture

### Backend package

Create `apps/backend/internal/forgejo/`, borrowing the boundaries—not the
GitLab data model—from `internal/gitlab/`:

- `models.go`: workspace configuration, repository, issue, pull request,
  task-link, and health DTOs. Forgejo issue/PR numbers are repository-scoped;
  all persistent links therefore include normalized origin + owner + repo +
  number.
- `client.go`: a small interface covering only v1 scope (identity, repository
  discovery, list/get issues and PRs, create PR, get PR status).
- `pat_client.go`: origin-normalized HTTP client, token auth, pagination,
  response/body limits, typed API errors, and no token logging.
- `noop_client.go` and `mock_client.go`: unavailable and deterministic test
  implementations.
- `store.go`, `store_config.go`, `store_links.go`: SQLite tables and accessors.
- `service.go`, `service_config.go`, `service_sync.go`: workspace authorization,
  configuration/test/save flow, imports, link reconciliation, and event
  publication.
- `controller.go` and `handlers.go`: workspace-scoped HTTP routes. Follow the
  newer GitLab HTTP boundary; do not revive its legacy unscoped WebSocket
  actions.

Use the existing workspace-secret adapter/pattern for the PAT. Config metadata
stores host, authenticated username, health state, timestamps, and a revision;
the token never enters JSON responses, logs, config export, or database rows.

Do not introduce a generic GitHub/GitLab/Forgejo abstraction in this PR. The
providers differ materially in auth, review, CI, and webhook semantics.
Extract a shared helper only when a concrete duplicate appears in this work.

### Persistence

Add only the tables required by the vertical slice:

- `forgejo_configs(workspace_id, origin, username, last_ok, last_error,
  last_checked_at, revision, created_at, updated_at)`;
- `forgejo_task_issues(task_id, repository_id, origin, owner, repo,
  issue_number, issue_url, title, state, last_synced_at, …)`;
- `forgejo_task_prs(task_id, repository_id, origin, owner, repo, pr_number,
  pr_url, head_branch, base_branch, state, draft, mergeable, ci_state,
  last_synced_at, …)`;
- `forgejo_issue_imports(workspace_id, origin, owner, repo, issue_number,
  task_id, imported_at)` with a uniqueness constraint preventing duplicate
  task creation from one external issue.

All lookups must authorize through the owning workspace/task before exposing a
link. Treat provider URLs and API payload text as untrusted display data.

### Backend wiring

Wire the provider in `internal/backendapp/services.go` and its routes beside
the GitLab route registration in `internal/backendapp/helpers.go`. Register a
90-second workspace auth-health poller using the existing shared integration
health pattern. Use the existing watcher dispatch coordinator only in a later
issue-watch milestone; manual import is sufficient for the first PR.

### Frontend

Add a Forgejo workspace settings page and API client/hooks/store slice patterned
after the GitLab connection page, but scoped to this smaller v1 surface:

- origin URL + token form with test-before-save;
- connection status, username, last successful check, and error display;
- repository picker from the configured Forgejo origin;
- issue browser with explicit **Create Kandev task** / **Link existing task**;
- task detail links for the external issue and PR, with a **Refresh** action;
- PR-create form fed by the task worktree branch and selected base branch.

The settings and task-link flows must have mobile-native navigation/drawers and
mobile E2E coverage; do not collapse the desktop table into an unusable mobile
form.

## Implementation sequence

1. **Foundation and connection**
   - Define models, origin normalization, PAT secret key, store migration, and
     mock/noop/PAT clients.
   - Implement `/api/v1/forgejo/...` workspace configuration, test connection,
     and status endpoints.
   - Validate the configured instance's OpenAPI/version endpoint during test
     connection and add HTTP-client tests for auth, pagination, malformed JSON,
     401/403/429/5xx, response limits, and origin normalization.

2. **Repository and issue import**
   - Add paginated repository/issue discovery and selected-repository state.
   - Add the issue import/link service with transactional deduplication.
   - Route task creation through the existing task service and preserve the
     external issue URL/identity in the resulting task metadata/link table.
   - Add integration tests for workspace isolation, duplicate import races,
     closed issues, and unauthorized cross-workspace access.

3. **Pull-request task linking**
   - Discover existing PRs by task worktree branch; persist and refresh links.
   - Create a Forgejo PR only after the branch is confirmed pushed by Git;
     surface a clear Git-auth error separately from API-token errors.
   - Display PR state, draft state, mergeability, and available CI roll-up on
     the task detail. Avoid merge/review/comment actions in the initial PR.

4. **Frontend and user workflow**
   - Add settings, repository picker, issue browser/import, and task-link UI.
   - Add loading/error/empty states, accessibility labels, responsive mobile
     flow, unit tests, and Playwright coverage using the mock API.

5. **Documentation and delivery**
   - Document token scopes, origin/TLS expectations, separation of API and Git
     credentials, manual-sync behavior, and the non-goals of v1.
   - Run backend unit/lint tests, web typecheck/lint/unit tests, and focused
     desktop/mobile E2E tests. Commit the implementation in reviewable slices
     and open a Kandev PR.

## Explicit non-goals for the first PR

- GitHub App compatibility, OAuth flow, or credential brokering.
- Calling `fj`, `tea`, `gh`, or `git` as the Forgejo API client.
- Bidirectional Kanban column mapping or automatic issue closure.
- Webhook-triggered task creation, CI-failure repair, review actions, or merge
  automation.
- Forgejo Actions runner administration.
- Sharing the Forgejo PAT with local, worktree, Docker, SSH, or Sprites agents.

## Follow-up milestones

1. Signed webhooks for issue/PR/push events with delivery deduplication.
2. Configurable issue-label watches using the existing watcher dispatch
   coordinator, producing Kandev tasks with explicit source links.
3. PR comments, reviews, CI/check-rollup mapping, and approval-aware task
   transitions.
4. Optional, explicitly configured workflow state mapping between Forgejo
   project boards and Kandev task states. This must remain opt-in to avoid
   conflicting sources of truth.

## Validation and acceptance criteria

- A user can connect a Forgejo v1 instance with a scoped PAT and see the
  authenticated identity without the token being returned or logged.
- A user can select an accessible repository, browse paginated open issues,
  and import one issue into exactly one Kandev task.
- A task can show/refresh its linked Forgejo issue and PR.
- After a branch is pushed with independently configured Git credentials, a
  user can create a Forgejo PR from the Kandev task.
- A failed API token, a failed Git push credential, and a failed Forgejo origin
  are shown as distinct actionable errors.
- Workspace A cannot read or mutate Forgejo connection data or links belonging
  to workspace B.
- The implementation works on desktop and mobile settings/task flows.

## Research sources

- Kandev `internal/gitlab/`: workspace config/secret/store/client/service
  boundary, backend route registration, and frontend settings/task-link flows.
- Forgejo API Usage: authentication, v1/OpenAPI location, versioning, and
  pagination — https://forgejo.org/docs/v15.0/user/api-usage/
- Forgejo token scopes — https://forgejo.org/docs/latest/user/token-scope/
- Forgejo webhooks — https://forgejo.org/docs/latest/user/webhooks/
