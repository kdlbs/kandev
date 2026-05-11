# GitLab Integration — Implementation Plan

**Spec:** `./spec.md`
**Issue:** [#820](https://github.com/kdlbs/kandev/issues/820)

## 1. Architecture decisions

**One package, mirror don't merge.** A new `internal/gitlab/` package mirrors
the layout of `internal/github/`. Do **not** introduce a unifying
`codehost.Client` interface in v1. The GitHub package is ~14k LOC and encodes
deep GitHub-specific knowledge (search query DSL, review states, mergeable
states, action presets). Premature abstraction would hurt both packages.
Defer the unifying interface to v2 — once the GitLab package is real, the
shared shape is concrete instead of guessed.

**Two clients, both shipped in v1.** Implement `pat_client.go` against
GitLab REST API v4 (Go HTTP client, similar shape to
`internal/github/pat_client.go`) **and** `glab_client.go` that shells out
to the `glab` CLI (similar shape to `internal/github/gh_client.go`).
`factory.go` selects: mock env var → `glab` CLI if installed and
authenticated → PAT (env var, then secrets store) → noop. Same selection
order as the GitHub package so users with `glab` already configured pick
up zero-config, and PAT remains the path for headless / corp-managed
environments.

**Auth: token-based, per-workspace host.** Token in `secrets.SecretStore`
under name `GITLAB_TOKEN` (env var `GITLAB_TOKEN` also accepted). Self-managed
host URL stored as a per-workspace setting (`gitlab_host`, default
`https://gitlab.com`). Per-workspace because different workspaces commonly
target different GitLab instances (work vs personal).

**Data model: parallel tables, shared field semantics.** New tables
`gitlab_mr_watches`, `gitlab_review_watches`, `gitlab_issue_watches`,
`gitlab_action_presets` parallel to their `github_*` counterparts. The
existing `Repository.Provider` field already supports `"gitlab"`; no schema
change there. PR/MR data is **not** unified — they're sibling concepts with
different fields (e.g. GitLab pipelines vs GitHub checks, GitLab approvals
vs GitHub reviews).

**`pr` skill: provider-aware, single skill.** Update `.agents/skills/pr/`
to detect the repo provider from `git remote` and branch on host. For GitLab
repos, use REST API directly via `curl`+`GITLAB_TOKEN` — no `glab`
dependency in the agent container.

## 2. File map

### Backend — new package `internal/gitlab/`

Mirrors `internal/github/` (~40 files) but deliberately smaller in v1. Files
to create:

- `models.go` — `MR`, `MRReview` (approvals), `MRComment`, `Pipeline`,
  `MRFeedback`, `MRStatus`, `MRSearchPage`, `Issue`, `IssueSearchPage`,
  `Project`, `RepoBranch`, `MRWatch`, `ReviewWatch`, `IssueWatch`. Shape
  parallel to `github/models.go` so frontend stores can share component
  families.
- `client.go` — `Client` interface (parallel to GitHub's, but with
  GitLab nouns: `GetMR`, `FindMRByBranch`, `ListAuthoredMRs`,
  `ListReviewRequestedMRs`, `SubmitMRApproval`, `ListPipelines`,
  `GetMRDiscussions`, `ResolveDiscussion`, `CreateMRDiscussionNote`).
- `pat_client.go` — REST v4 implementation. Per-instance host URL.
  Uses `PRIVATE-TOKEN` header for auth. Methods needed:
  - Projects: `/projects?membership=true`, `/projects/:id`, `/projects/:id/repository/branches`.
  - MRs: `/projects/:id/merge_requests`, `/merge_requests` (search),
    `/projects/:id/merge_requests/:iid`,
    `/projects/:id/merge_requests/:iid/discussions` (read + post + resolve),
    `/projects/:id/merge_requests/:iid/approvals`,
    `/projects/:id/merge_requests/:iid/changes`,
    `/projects/:id/merge_requests/:iid/commits`.
  - Pipelines: `/projects/:id/pipelines`, `/projects/:id/pipelines/:id/jobs`.
  - Issues: `/issues` (search), `/projects/:id/issues/:iid`.
  - User: `/user` (auth check + login).
- `pat_client_test.go` — table-driven tests with `httptest.Server` per
  endpoint group, mirroring `github/pat_client_test.go`.
- `glab_client.go` — shells out to `glab` CLI for the same `Client`
  interface. Mirrors `internal/github/gh_client.go`: `GLabAvailable()`
  detects the binary, `IsAuthenticated()` runs `glab auth status`, and
  each method composes a `glab api …` or `glab mr …` invocation,
  parsing JSON stdout. Where `glab` lacks a direct command (e.g. specific
  search filters), fall through to `glab api` against the REST endpoint.
- `glab_client_test.go` — uses `exec.LookPath` shimming and a
  fake `glab` binary on `PATH` (same pattern as `gh_client_test.go`).
- `mock_client.go` + `mock_client_test.go` + `mock_controller.go` —
  E2E mock surface, gated by `KANDEV_MOCK_GITLAB=true`. Uses the
  `internal/integrations` shared shapes? **No** — copy the GitHub pattern
  (the mock currently lives outside `internal/integrations`). Match
  whichever pattern the team has standardized on (Linear/Jira use
  `internal/integrations`; GitHub does not). For consistency with
  GitHub-the-code-host, follow the GitHub pattern.
- `noop_client.go` — null-object fallback when unauthenticated.
- `factory.go` — `NewClient(ctx, secrets, host, log)`. Order: mock env
  var → `glab` CLI (if installed and authenticated for the configured
  host) → PAT (env var `GITLAB_TOKEN`, then secrets store key
  `GITLAB_TOKEN`) → noop.
- `service.go` — coordinator parallel to `github.Service`. Accepts the
  per-workspace host. Methods: status, MR list/search, MR feedback (incl.
  discussions), watches CRUD, action presets CRUD, sync. **Use
  `internal/integrations/healthpoll`** for the auth-health loop — that's
  the shared utility Jira/Linear already use, and the GitHub package
  predates it. New code should pick the better pattern.
- `service_*_test.go` — tests for issue, status, sync, watches, search
  cache (parallel to GitHub's split).
- `service_task_events.go` — subscribes to task events and resyncs when
  MRs are linked. Mirror GitHub.
- `store.go` — SQLx persistence for the four new tables + watches/presets
  read/write. Migrations next.
- `store_test.go` — multi-project / multi-workspace round-trip.
- `controller.go` — HTTP routes under `/api/v1/gitlab/*`:
  `GET /status`, `GET/POST/PUT/DELETE /mr-watches`,
  `GET/POST/PUT/DELETE /review-watches`,
  `GET/POST/PUT/DELETE /issue-watches`,
  `GET /mrs/:projectId/:iid/feedback`, etc.
- `handlers.go` — WS dispatcher registrations
  (`ActionGitLabStatus`, `ActionGitLabMRList`, …).
- `poller.go` — review-watch + issue-watch background loop. Reuse
  `healthpoll.Poller` for auth health; keep MR/issue polling in this
  package (different cadence, payload-shaped).
- `connectivity.go`, `constants.go`, `ttl_cache.go` — verbatim parallels.
- `provider.go` — `Provide(writer, reader *sqlx.DB, secrets SecretStore,
  eventBus bus.EventBus, log *logger.Logger) (*Service, func() error, error)`.

### Backend — schema migrations

- `apps/backend/internal/task/repository/migrations/NNNN_gitlab_tables.sql` —
  create `gitlab_mr_watches`, `gitlab_review_watches`,
  `gitlab_issue_watches`, `gitlab_action_presets`. Use `gitlab_*` prefix
  on every column to avoid collisions when the unifying refactor lands.
- `apps/backend/internal/workspace/...` — add `gitlab_host` column to the
  workspace settings table (or, if workspace settings are JSON, add the
  field to the JSON shape). Default `https://gitlab.com`.

### Backend — wiring

- `cmd/kandev/services.go` — new `initGitLabService(...)` helper that
  resolves the per-workspace host (or a global default for unscoped
  callers) and wires the service.
- `cmd/kandev/helpers.go` — register routes via `gitlab.RegisterRoutes`
  and `gitlab.RegisterMockRoutes`.
- `internal/repoclone/protocol.go` — replace the `gitlab.com` hardcode
  with a host parameter. `CloneURL` signature gains `host string`. Update
  call sites in `internal/agent/lifecycle/executor_*.go` and
  `internal/orchestrator/executor/executor_execute.go` to pass the
  workspace's `gitlab_host` when `provider == "gitlab"`.
- `internal/agent/credentials/env_provider.go` — already lists
  `GITLAB_TOKEN` in `knownAPIKeyPatterns`. No change required.
- `internal/agent/lifecycle/default_scripts.go` — extend the default
  prepare script so HTTPS clones for GitLab use
  `https://oauth2:${GITLAB_TOKEN}@<host>/<owner>/<name>.git`. Existing
  GitHub equivalent is the model.

### Frontend — new pages and components

- `apps/web/app/gitlab/page.tsx` + `gitlab-page-client.tsx` — mirror of
  `app/github/`. SSR fetches `/api/v1/gitlab/status`, hydrates store.
- `apps/web/app/settings/integrations/gitlab/page.tsx` — mirror of
  `settings/integrations/github/page.tsx`. Hosts the
  `GitLabIntegrationPage` panel: connection status banner, host URL
  field, PAT field, reconnect CTA.
- `apps/web/components/gitlab/` — domain components. The MyGitHub
  component family (`my-github/pr-list`, `my-github/issue-list`,
  presets sidebar, search bar, action presets, save-preset dialog,
  pagination, list toolbar, quick task launcher) is large and tightly
  coupled to the GitHub data shape. Two viable approaches:
  - **(A) Extract shared components.** Move generic pieces (search bar,
    pagination, save-preset dialog, list toolbar, presets sidebar
    skeleton) into `components/code-host/` parameterised on item shape.
    Wire both `github` and `gitlab` to them. Higher upfront cost, no
    duplication.
  - **(B) Copy-and-adapt.** Duplicate the components folder, swap PR
    types for MR types. Lower upfront cost, two parallel codebases to
    maintain.
  - **Recommendation: (A) for shells, (B) for body.** Extract the
    container components (sidebar, toolbar, pagination, save dialog) —
    they're already provider-agnostic in shape. Copy the row renderers
    (`pr-row.tsx` → `mr-row.tsx`) where the visible fields differ
    (approvals vs reviews, pipelines vs checks).
- `apps/web/lib/state/slices/gitlab/` — new slice mirroring
  `slices/github/`. Same hydration patterns.
- `apps/web/hooks/domains/gitlab/` — `use-gitlab-status`, watches,
  search, action presets. Symmetric to `hooks/domains/github/`.
- `apps/web/lib/api/domains/gitlab-api.ts` — REST client.
- `apps/web/lib/types/gitlab.ts` — `GitLabMR`, `GitLabIssue`, etc.

### Frontend — kanban top-bar

- `apps/web/components/kanban/kanban-header.tsx` — add
  `GitLabTopbarButton` next to `GitHubTopbarButton`. Render only when
  GitLab is configured for the active workspace (gate on
  `useGitLabAvailable`).
- `apps/web/components/kanban/kanban-header-mobile.tsx` — same.

### Agent skill — `pr`

- `.agents/skills/pr/SKILL.md` — add a "GitLab repos" section
  explaining the detection (`git remote get-url origin` matches
  `:gitlab|/gitlab`) and the GitLab MR-creation flow. Prefer `glab` CLI
  if available in the agent container; fall back to REST API via `curl`
  with `GITLAB_TOKEN`. Keep `gh` as the GitHub path.
- `.claude/skills/pr/SKILL.md` symlink — already symlinked, no change.

### WebSocket actions

- `pkg/websocket/actions.go` (or wherever GitHub actions are declared) —
  add `ActionGitLabStatus`, `ActionGitLabMRList`, `ActionGitLabMRGet`,
  `ActionGitLabMRFeedbackGet`, watches (review/issue/MR), pipeline
  status, action presets, stats.

## 3. Phases

Each phase ends with `make fmt` then `make typecheck test lint` (backend)
and `pnpm typecheck test lint` (web).

### Phase 1 — Backend client + auth (no UI)

Goal: the backend can list one user's MRs over REST.

- `internal/gitlab/{models,client,pat_client,glab_client,noop_client,
  factory,connectivity,constants}.go` plus tests.
- Mock client + controller. Wired under `KANDEV_MOCK_GITLAB=true`.
- Provider + `cmd/kandev/services.go` `initGitLabService`.
- Status WS handler + `/api/v1/gitlab/status` HTTP route.
- Migration for the new tables (kept empty-but-present so future phases
  can write).
- Tests: PAT client per-endpoint, noop client, factory selection,
  controller status route.

### Phase 2 — Repo cloning + agent env

Goal: a GitLab repo can be cloned and the agent has the token.

- `repoclone/protocol.go` host parameter. Call-site updates.
- `lifecycle/default_scripts.go` HTTPS-with-token rewrite for GitLab.
- Workspace `gitlab_host` setting (read/write API + UI field deferred
  to Phase 3).
- Tests: protocol unit test for SSH/HTTPS variants on custom host.
  Lifecycle prepare-script test for token injection.

### Phase 3 — Settings page + connection UX

Goal: a user can connect/reconnect a GitLab account from the UI.

- `app/settings/integrations/gitlab/page.tsx` + panel.
- API endpoints: get/set host, set/clear token.
- Reuse `<IntegrationAuthStatusBanner>` and
  `<IntegrationAuthErrorMessage>` from `internal/integrations` if those
  shells fit (they were built for Linear/Jira; check shape compat first).
- Tests: settings panel render + token-save flow (vitest).

### Phase 4 — MR list page + watches

Goal: full feature parity with the GitHub page for browse + watch.

- Extract shared shells from GitHub MyGitHub components into
  `components/code-host/`.
- New `components/gitlab/my-gitlab/` — row renderers, search-bar
  preset definitions, action-presets resolution.
- `app/gitlab/page.tsx` + client.
- Watch CRUD WS handlers + UI.
- `gitlab` slice + hooks.
- Poller for review-watches and issue-watches (uses
  `healthpoll.Poller` for auth-health; MR/issue polling in
  `gitlab/poller.go`).
- Tests: store slice, hooks, save-preset, search.

### Phase 5 — MR review surface (#820 core)

Goal: the agent UI shows MR feedback (discussions, approvals, pipeline)
and the agent can post/resolve discussions.

- `GetMRDiscussions` + `CreateMRDiscussionNote` + `ResolveDiscussion`
  in `pat_client.go`.
- `MRFeedback` payload + `WS ActionGitLabMRFeedbackGet`.
- Frontend: in the existing PR-feedback panel that lives next to a
  task, branch on `provider`. Reuse the comments/threads UI; map
  GitLab discussions onto the same shape (discussion = thread, notes =
  comments, `resolved` flag).
- Tests: pat_client discussions round-trip, frontend feedback panel
  with a GitLab fixture.

### Phase 6 — `pr` skill update + #820 wiring

Goal: clicking "Create PR" on a GitLab task opens a real MR.

- `.agents/skills/pr/SKILL.md` — GitLab branch.
- The kanban "Create PR" button already runs the `pr` skill on the
  task; no new wiring needed. Verify the surfaced MR URL flows back
  through the existing PR-watch path (Phase 4 watches consume it).
- Manual smoke: open a kandev workspace pointing at a GitLab repo,
  edit, click Create PR, watch the MR appear, comment from gitlab.com,
  watch the agent see and resolve the comment.

### Phase 7 — Cleanup + docs

- Remove `// TODO: GitLab support is not yet implemented` from
  `repoclone/protocol.go`.
- Update `CLAUDE.md` "Repo Layout" and the integrations section to
  list `internal/gitlab/`.
- Add the new spec/plan to `docs/specs/INDEX.md` (already done) and
  flip status to `shipped` once merged.

## 4. Risks / open points

- **API shape mismatches.** GitLab's MR discussions are nested
  (discussion → notes → resolved); GitHub's review comments are flat
  with `in_reply_to`. Mapping into the existing comments/threads store
  needs care — discussions are the natural unit on GitLab, not
  individual comments. Mitigation: GitLab discussions become first-class
  in the store, the GitHub side is mapped as "discussions of one note".
  Defer if it explodes scope; v1 can render two distinct components.
- **Action-preset query syntax.** GitHub presets use the GitHub search
  DSL (`is:pr is:open author:@me`). GitLab MR search uses query
  parameters (`?author_username=…&state=opened`). The presets feature is
  GitHub-shaped; v1 ships GitLab presets as a constrained set
  (assignee/author/reviewer × open/merged) without full DSL parity.
- **Self-managed TLS.** Self-hosted GitLabs sometimes use private CAs.
  `pat_client.go` uses the default `http.Client`. A workspace setting
  `gitlab_insecure_skip_verify` is the lazy answer; the right answer is
  reading `SSL_CERT_FILE`. v1 reads `SSL_CERT_FILE`; flag the
  self-signed escape hatch as a follow-up.
- **Per-workspace vs global host.** Per-workspace adds per-call
  resolution overhead and complicates non-workspace-scoped callers
  (review/issue pollers). Mitigation: the poller iterates workspaces
  and uses each workspace's host; the MyGitLab page is workspace-scoped
  by construction.
- **`glab` CLI parity.** `glab` lacks direct commands for some search
  filters that REST exposes; the CLI client falls back to `glab api`
  for those, which means the `glab` binary version matters less than
  the underlying REST endpoint version. Pin a minimum `glab` version
  in `GLabAvailable()` and surface the version mismatch in the
  status WS payload so the settings page can warn.
- **Frontend component extraction (Phase 4).** Risk of refactor churn
  on GitHub during the extraction. Mitigation: extract
  one-component-at-a-time behind matching tests; revert the extraction
  if scope creeps and just copy-and-adapt instead.
- **Mock harness scope.** GitHub's mock has 14k LOC of behavior to
  emulate; GitLab's mock can ship with a thin slice (status, MR list,
  MR feedback, watch round-trips). The Playwright e2e suite is the
  forcing function — match what tests need, not full parity.

## 5. Out of scope (per spec)

Webhook ingestion, GitLab CI editing, GitLab-Issues-as-tracker,
group-wide dashboards, OAuth login, Bitbucket. Two natural follow-ups
once shipped: webhook ingestion (kills the pollers) and the unifying
`codehost.Client` interface (now that two real clients exist).
