---
spec: docs/specs/gitlab-integration/spec.md
created: 2026-07-20
status: done
---

# Implementation Plan: GitLab Integration Parity

## Overview

Finish the existing GitLab integration instead of replacing it. First make the
connection and client resolution workspace-owned, then connect watches to the
shared task dispatcher and complete the task-link/upstream write contracts.
User-facing watch, quick-launch, and MR review flows land after those contracts;
provider-aware MR creation and integrated desktop/mobile E2E close the build.

The legacy plan at `docs/specs/gitlab-integration/plan.md` describes the initial
scaffold and is superseded by this plan for parity work.

## Backend

### Workspace-owned connection

- Add `GitLabConfig` and `gitlab_configs` persistence in
  `apps/backend/internal/gitlab/models.go`, `store.go`, and new focused
  `store_config.go`, `service_config.go`, and `copy.go` files.
- Refactor `apps/backend/internal/gitlab/factory.go`, `provider.go`, and
  `service.go` to resolve/cache a client by `workspace_id`; no service method or
  poll may call the legacy installation-wide `Client()` path.
- Migrate the legacy `gitlab_host` system setting and `GITLAB_TOKEN` secret to a
  deterministic workspace using the Jira/Linear provider migration pattern.
- Replace `/status`, `/token`, and `/host` write semantics with workspace config,
  test, delete, and copy routes while retaining compatibility only where an
  existing internal caller still needs it during the migration.
- Require workspace scope on search, projects, feedback, presets, watches, and
  write actions in `controller.go`, `controller_watches.go`, and `handlers.go`.
- Accept normalized `http://` and `https://` self-managed origins and preserve
  the configured scheme through API, web-link, clone, and MR-create paths.
- Update `apps/backend/internal/gitlab/poller.go` and search/watch services to
  resolve the client from each watch's workspace.

### Watch dispatch

- Add `GitLabReviewWatcherSource` and `GitLabIssueWatcherSource` under
  `apps/backend/internal/orchestrator/`, implementing the existing
  `WatcherSource` reservation, request, attachment, auto-start, throttle, and
  self-heal contract.
- Replace the nullable GitLab-specific task creators in
  `event_handlers_gitlab.go` with three-line forwards to
  `WatcherDispatchCoordinator.Dispatch`.
- Extend GitLab watch rows/requests with optional repository/base branch,
  `max_inflight_tasks`, and `last_error`; validate every referenced resource is
  owned by the watch workspace.
- Add disabled-with-error behavior plus GitHub-parity reset/delete lifecycle:
  reset previews, best-effort attempts all owned tasks (including archived),
  transactionally clears dedup/last-poll state, retains the watch, and reruns
  immediately for reviews or on the next issue poll; delete best-effort deletes
  owned tasks, removes watch/dedup state, and never reruns. Ensure `e2e_reset.go`
  removes GitLab poller-owned rows before tasks.

### Task-to-MR association and quick launch support

- Add a strict configured-host MR URL parser and
  `AssociateExistingMRByURL`/`UnlinkTaskMR` services in new
  `service_task_mr_link.go` and `controller_task_mrs.go` files.
- Make association idempotent, validate task/repository workspace ownership,
  fetch live MR state before writing, and delete the linked refresh watch on
  unlink.
- Keep `SyncTaskMR` as the internal project/IID refresh path, not the public
  paste-URL contract.

### Reviewers and GitLab notification subscriptions

- Preserve GitLab numeric member IDs in the public model.
- Extend `Client` and PAT/glab/mock/noop implementations with project-member
  search, reviewer replacement, and issue/MR subscription read/write methods.
- Add workspace-scoped routes in `controller_watches.go`; refresh the MR after
  successful reviewer writes and map ineligible members to an action error.

### First-class merge request creation

- Add `prProviderGitLab` and a GitLab remote parser/creator to
  `apps/backend/internal/agentctl/server/process/git_pr_providers.go`; prefer
  `glab` and use a JSON-safe REST fallback with `GITLAB_TOKEN`.
- Extend the successful `PRCreateResult` with `provider` while preserving
  `worktree.create_pr` compatibility.
- Make `GitHandlers` dispatch successful GitLab URLs to the GitLab association
  service through `backendapp/gateway.go`, including multi-repository identity.
- Resolve the workspace GitLab host/token for clone and executor environment in
  `backendapp`, `orchestrator/executor`, and `repoclone`; never hardcode
  `gitlab.com` when the workspace has a configured host.

## Frontend

### Workspace settings

- Replace global GitLab settings calls in
  `apps/web/components/gitlab/gitlab-settings.tsx` with workspace config APIs
  and status, including host, auth method, token, test, disconnect, and
  unavailable/auth-required states.
- Add GitLab to `integration-copy-config.ts`; the settings route workspace
  switcher remains authoritative.
- Make `/gitlab` and integration availability consume status for the active
  workspace rather than global state.

### Watch settings

- Build GitLab review/issue watch dialog and table components following the
  GitHub shells and Jira/Linear workspace/resource validation patterns.
- Expose create/edit, enable/pause, run-now, reset-preview/reset, and delete in
  `gitlab-settings.tsx`, with stable desktop/mobile controls and confirmation
  for destructive reset.

### Browse quick launch and task links

- Add GitLab `QuickTaskLauncher`, matching only `provider = "gitlab"`
  repositories on the configured host and project path.
- Add preset menus to `mr-list.tsx` and `issue-list.tsx`. On MR task-create
  success, call the explicit association endpoint; issue launches retain URL
  context without a durable issue link.
- Add a `(host, project_path, mr_iid) -> tasks` hook and task indicator so the
  browse list reflects links and unlinking.

### MR review surface

- Add GitLab MR detail/section components using existing feedback, files,
  commits, approvals, discussion, merge, label, and assignee endpoints plus the
  new reviewer/subscription endpoints.
- Route task detail/dockview PR panels by provider and add linked GitLab MRs to
  auto-open, picker, changes-panel, and context behavior without regressing
  GitHub PRs.
- Use "merge request" labels for GitLab while keeping protocol/internal PR names
  where compatibility requires them. All actions remain reachable on mobile.

## Tests

- **Workspace isolation and migration:** `internal/gitlab/store_config_test.go`,
  `service_config_test.go`, `controller_config_test.go`, and
  `backendapp/gitlab_service_test.go` cover two hosts/tokens, deterministic
  legacy migration, copy-without-watches, invalid config preservation, and
  cross-workspace denial, including preserved HTTP and HTTPS schemes.
- **Watch dispatch:** `orchestrator/source_gitlab_test.go` and
  `event_handlers_gitlab_test.go` exercise reserve -> create -> attach,
  release-on-failure, auto-start params, throttle, and self-heal. GitLab
  service/controller/store tests cover resource validation and reset.
- **Task MR linking:** GitLab parser/service/store/controller tests cover
  self-managed URLs, wrong-host rejection, idempotence, multi-repo identity,
  unlink, and workspace isolation.
- **Provider writes:** PAT/glab/mock/noop/controller tests cover numeric reviewer
  IDs, empty reviewer replacement, subscription read/write, issue/MR endpoint
  selection, and sanitized failures.
- **MR creation:** agentctl process and handler tests cover HTTPS/SSH/self-managed
  remotes, glab/REST selection, default/explicit target branch, draft output,
  partial push failure, and GitLab callback association.
- **Frontend units:** API, slice, hook, settings, watch dialog/table, launcher,
  row indicator, MR detail, task dockview, and provider terminology tests cover
  the matching observable spec scenarios.

## E2E Tests

- Add GitLab mock seed/control APIs to
  `apps/backend/internal/gitlab/mock_controller.go` and
  `apps/web/e2e/helpers/api-client.ts`; reset all GitLab config/watch/link state
  in `apps/backend/internal/backendapp/e2e_reset.go`.
- Desktop `apps/web/e2e/tests/gitlab/gitlab-parity.spec.ts`: connect two
  workspaces, prove isolation, quick-launch/link/unlink, open MR review, set a
  reviewer, reply/resolve, and toggle MR/issue subscriptions.
- Desktop `apps/web/e2e/tests/gitlab/gitlab-watches.spec.ts`: create and run
  review/issue watches, prove exactly-once task creation, pause behavior, and
  reset confirmation.
- Desktop `apps/web/e2e/tests/gitlab/gitlab-mr-creation.spec.ts`: create an MR
  from a task changes panel, verify provider terminology and automatic link.
- Mobile `apps/web/e2e/tests/gitlab/mobile-gitlab-parity.spec.ts`: cover browse
  quick launch, watch controls, linked-MR detail/actions, and MR creation at the
  mobile viewport with no horizontal overflow or inaccessible controls.

## Implementation Waves

Wave 1:

- [x] [Task 01: Workspace connection](task-01-workspace-connection.md) - done

Wave 2 (parallel after Task 01):

- [x] [Task 02: Watcher dispatch](task-02-watcher-dispatch.md) - done
- [x] [Task 03: Task MR linking and quick launch](task-03-task-mr-linking-launch.md) - done
- [x] [Task 04: Reviewers and subscriptions](task-04-reviewers-subscriptions.md) - done

Wave 3:

- [x] [Task 05: Watch settings UI](task-05-watch-settings-ui.md) - done

Wave 4 (parallel after Wave 2; Task 06 does not share Task 07 runtime files):

- [x] [Task 06: MR review UI](task-06-mr-review-ui.md) - done
- [x] [Task 07: Merge request creation runtime](task-07-mr-creation-runtime.md) - done

Wave 5:

- [x] [Task 08: E2E, docs, and verification](task-08-e2e-docs-verification.md) - done

Tasks 03 and 04 must put new HTTP handlers in separate focused controller files;
their only shared route-registration edit should be integrated by the parent at
the end of Wave 2. Task 05 finishes settings before Wave 4. Tasks 06 and 07 own
task-review UI and runtime/create UI respectively; neither should refactor shared
GitHub components.

## Verification

Targeted commands are listed in each task. Final integrated verification runs
in this order:

```bash
rtk make fmt
rtk make typecheck
rtk make test
rtk make lint
cd apps/web && rtk pnpm e2e:run tests/gitlab/gitlab-parity.spec.ts tests/gitlab/gitlab-watches.spec.ts tests/gitlab/gitlab-mr-creation.spec.ts
cd apps/web && rtk pnpm e2e:run --no-build --project mobile-chrome tests/gitlab/mobile-gitlab-parity.spec.ts
rtk node --test scripts/validate-public-docs.test.mjs
rtk node scripts/validate-public-docs.mjs
```

## Risks

- The global-to-workspace client refactor touches every GitLab call path. Keep a
  single `clientForWorkspace` boundary and reject missing scope rather than
  retaining mixed global/workspace behavior.
- GitLab discussion and reviewer capabilities vary by version/tier. Test against
  REST v4 contracts and surface unsupported writes without hiding readable MR
  state.
- GitHub task review UI is deeply named around PRs. Add provider routing and
  GitLab-specific components before extracting shared abstractions; broad UI
  unification is outside this build.
- `glab` is optional in agent executors. The REST fallback must parse remote
  host/project safely, redact credentials, and never send a token to a host that
  does not match the workspace connection.
