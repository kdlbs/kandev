---
spec: docs/specs/azure-devops-integration/spec.md
created: 2026-07-17
updated: 2026-07-31
status: building
---

# Azure DevOps Integration Plan

## Scope

Implement the Azure DevOps Services integration defined in
[`../../specs/azure-devops-integration/spec.md`](../../specs/azure-devops-integration/spec.md):
workspace-scoped PAT configuration, direct REST work-item and pull-request
reads, persistent task PR links, responsive settings/browse surfaces, immediate
integration availability updates, provider-neutral remote repository selection,
server-side authenticated Azure clones, an Azure Boards view, remembered browse
preferences, rich work-item detail, provider-specific task actions, and
work-item/PR watchers. No Azure runtime path may require `gh` or `az`.

## Current State

Tasks 01-29 are implemented in this workspace. Tasks 19 and 23 were reopened
and completed for a refresh-hydration regression and settings-parity gap. The backend persists workspace
query/action overrides at `/api/v1/azure-devops/workspace-settings`, exposes
generation-safe work-item and pull-request watchers, dispatches deduplicated
tasks through the orchestrator, and performs immediate auth-health probes after
credential changes. The browse surface restores portable per-user filters,
opens a responsive read-only detail dialog/drawer, supports assignment and
board-column changes, and launches associated Kandev tasks from provider
quick-action presets. Settings exposes quick-action configuration plus watcher
create/edit/enable/disable/run/reset/delete controls.

The repair adds boot-state mapping coverage, makes the browse surface consume
workspace-resolved query presets, and aligns Azure's watcher/action/query
section order and editor semantics with GitHub. Focused Go, Vitest, desktop
Playwright, and mobile Playwright commands are recorded in Tasks 19 and 23.

## Architecture

- Add an independent `internal/azuredevops` package. Do not add Azure methods to
  `github.Client` or translate Azure records into GitHub API structs.
- Reuse Jira's workspace-scoped config/secret/health patterns and GitLab's
  source-host REST/task-review patterns.
- Persist provider-native Azure identifiers. Normalize only the summary fields
  required by shared task UI.
- Persist canonical credential-free remote URLs for provider repositories and
  normalize new `remote_url` task inputs alongside the legacy `github_url`
  compatibility field.
- Resolve Azure clone credentials by workspace inside the backend clone path.
  Never expose the PAT to task metadata, an agent environment, a persisted URL,
  or command output.
- Use Azure DevOps REST API 7.1, an injected HTTP client for deterministic
  tests, bounded response bodies, context-aware requests, and typed API errors.
- Discover board context through project teams and team backlog levels, then
  combine the selected backlog's work-item references with its board column
  metadata. Hydrate work items through the existing bounded 200-item batches.
- Keep provider writes behind a fixed server-side field allowlist. Resolve the
  selected board's column/done reference names on the backend and use an Azure
  JSON Patch `/rev` test before assignment or board-position operations. Resolve
  Assign to me from the stored PAT identity; never accept an arbitrary identity
  supplied by the browser.
- Store Azure browse preferences in backend-owned portable user settings, keyed
  by workspace. Do not introduce localStorage/sessionStorage fallback reads or
  dual writes.
- Fetch work-item detail and discussion on demand. Normalize a small allowlist
  of planning fields, sanitize provider HTML, and page comments with Azure's
  opaque continuation token.
- Add a direct revision-safe work-item assignment endpoint for detail opened
  outside a board. Keep status/column changes on the existing board endpoint,
  because only board context can validate a provider column.
- Reuse GitHub's action/default-query preset shapes and GitLab's generation-safe
  issue/review watcher lifecycle without translating Azure records into either
  provider's models.
- Probe saved credentials immediately and return the resulting health in the
  config mutation response. Keep the 90-second health poll as recovery.
- Register the service as non-fatal during backend boot and expose mock routes
  only under `KANDEV_MOCK_AZURE_DEVOPS=true`.

## Backend Touch Points

- New package: `apps/backend/internal/azuredevops/`.
- Service wiring: `apps/backend/internal/backendapp/services.go`,
  `helpers.go`, and `main.go` where pollers are started.
- Repository provider parsing/discovery where provider enums are currently
  restricted to GitHub/GitLab.
- Runtime defaults: `profiles.yaml` for the E2E mock selector only.
- Workspace cleanup and task/repository validation through existing service
  interfaces rather than integration-specific SQL outside the new package.
- User settings models/DTO/store patches for
  `azure_devops_browse_preferences`.
- Work-item detail/comments/current-identity REST methods, normalized planning
  fields, and constrained assignment/column patch generation.
- Persistent task/work-item links, workspace query/action preset overrides, and
  Azure watcher/reservation stores.
- Azure watcher polling plus provider events, orchestrator watcher sources,
  self-heal, in-flight throttling, reset, and cleanup wiring.

## Frontend Touch Points

- Typed API and types under `apps/web/lib/api/domains/azure-devops-api.ts` and
  `apps/web/lib/types/azure-devops.ts`.
- Domain hooks under `apps/web/hooks/domains/azure-devops/`.
- Settings route and integration menu entry.
- `/azure-devops` browse page with a compact work-item/PR segmented view,
  desktop filter rail, and mobile filter sheet.
- `/azure-devops` Board mode becomes the default connected view. Its project,
  team, and board selectors share one board view model with a multi-column
  desktop DnD composition and a focused single-column mobile composition.
- Desktop card moves are optimistic and roll back on failure. Card editing uses
  a dialog on wider layouts and a full-height, safe-area-aware mobile surface;
  the mobile editor includes an explicit column picker instead of requiring
  touch drag.
- Task PR summary integration through a provider-tagged view model; Azure
  detail remains in Azure-specific components.
- A shared integration availability invalidation channel updates every consumer
  after configuration mutations while retaining periodic health polling.
- A shared source-control repository picker merges GitHub, GitLab, and Azure
  discovery and dispatches branch reads to the selected provider.
- Azure browse presets and saved views reuse the interaction model of GitHub and
  GitLab, with raw WIQL contained in an Advanced disclosure.
- The page hydrates portable Azure preferences before selecting defaults,
  persists only valid user changes, and falls back deterministically when a
  remembered provider ID is stale.
- Board cards and work-item rows open a desktop `Dialog`. Phone uses a dedicated
  full-height `Drawer` with a fixed header/action area and one internally
  scrolling detail/discussion body. Shared hooks own detail, assignment, move,
  quick-action, and error state.
- Work-item detail is read-only except for Assign to me, Unassign, and board
  column/split-state controls. A visible Task menu exposes workspace-configured
  actions and existing linked Kandev tasks.
- Azure settings follows GitHub's analogous order: connection, pull-request
  watches, work-item watches, quick actions, and default queries. Action and
  query editors use tabbed responsive cards, Reset, dirty highlighting, and the
  shared floating Save changes control.
- No required action may be hover-only or desktop-only.

## Implemented Design

### Watch persistence and lifecycle

- Mirror GitLab's two-kind watcher layout with Azure-native names:
  `watch_models.go`, `store_watches.go`, `service_watch_reset.go`,
  `controller_watches.go`, and `controller_watch_reset.go`.
- Persist separate work-item and PR watch tables plus separate reservation
  tables. Reservation uniqueness always includes `generation`; work-item
  identity is project/work-item ID and PR identity is
  project/Azure-repository/PR ID.
- Keep the Kandev task repository (`repository_id`) distinct from the optional
  Azure PR filter (`azure_repository_id`). Normalize poll interval to 300
  seconds when omitted and clamp values below 60. For `max_inflight_tasks`,
  omitted preserves the value, zero clears it, and a positive value sets it.
- ID-addressed controller operations first load the record, authorize its
  workspace, and return the same not-found response for absent and unauthorized
  watches. Reset routes use `/:id/reset/preview` and `/:id/reset`.
- Reset increments the generation before shared cleanup and then removes prior
  reservations. Delete marks the watch deleting and disabled before cleanup.
  Every reservation attach and release is conditioned on watch ID plus
  generation.

### Polling and dispatch

- One Azure poller schedules both watch kinds, selects enabled due watches, and
  uses one bounded check path for create, Run now, and periodic polling. Limit
  each provider check to 100 ordered matches and hydrate work items in Azure's
  200-ID maximum batches.
- Publish an event only after a current-generation reservation succeeds.
  Authentication, query, and provider failures update `last_error` and
  `last_error_at`, publish nothing, and retain the next polling opportunity.
- Reconcile terminal state only for reservations that own an auto-created task.
  Apply shared `auto`, `always`, and `never` cleanup without touching manual
  tasks or another generation's task.
- Add work-item and PR Azure `WatcherSource` implementations. Their
  `WatchMetadataKey` values are respectively
  `azure_devops_work_item_watch_id` and `azure_devops_pr_watch_id`; the task
  metadata must write those exact keys plus the provider identity keys defined
  in the spec.
- Wire event handlers, shared throttling, reservation release/attach,
  dependency self-heal, poller start/stop, and workspace/task cleanup into
  backend startup. Missing workflow, step, task repository, agent profile, or
  executor profile disables the watch with a user-visible error.

### Work-item detail

- Add `PATCH /api/v1/azure-devops/work-items/:id` for Assign to me/Unassign
  from search results. Reuse the existing PAT-identity lookup, allowlisted JSON
  Patch builder, revision conflict mapping, and normalized work-item response.
  Continue using the board endpoint for column and split-state changes.
- A shared detail hook loads core item and comments independently, owns comment
  continuation tokens, derives linked tasks from the workspace association
  cache, and exposes rollback-safe assignment/move mutations.
- Render Azure description HTML through the existing
  `rehype-raw`/`rehype-sanitize` approach used by provider content. Never mount
  provider HTML directly. Load comments newest-first; append older pages
  without duplicates and retry only the failed page.
- Board cards and work-item rows open the same logical detail. Desktop uses a
  modal `Dialog` over retained context. Phone uses a `100dvh` full-height
  `Drawer` with fixed header/action regions, one `min-h-0 overflow-y-auto`
  body, safe-area padding, focus return, and 44px targets.
- All contexts show assignment, Azure link, linked Kandev tasks, and the
  workspace quick-action Task menu. Only board-origin detail shows
  column/split controls. Successful quick-action creation persists the
  association before invalidating detail/task-link queries.

### Automation settings and E2E

- Add pull-request and work-item watcher sections before Quick actions and
  Default queries, matching GitHub's analogous settings order. Desktop watcher
  lists use tables; phone uses stacked cards.
  Create/edit is a desktop dialog and full-height phone drawer backed by the
  same form normalization and domain hook.
- Extend the Azure mock client/controller and E2E API helper with deterministic
  detail pages, comment continuation/failure, current identity, watcher
  matches, reservations, reset previews, and terminal-state transitions.
- Add focused desktop scenarios to the existing Azure integration spec and
  mobile scenarios to `mobile-azure-devops.spec.ts`. Implementation follows
  RED-GREEN: write the failing Playwright assertion before completing each
  corresponding UI behavior.

## Tests

- Go table tests for URL validation, PAT headers, API errors, WIQL batching,
  PR conversion, workspace isolation, persistence, and route status codes.
- Go service tests for repository/task association validation and restart
  persistence.
- TypeScript unit tests for API request/response normalization and pure filter
  or status helpers.
- Playwright desktop and `mobile-chrome` flows using the Azure mock controller:
  connect, browse work items, browse PRs, and open PR feedback.
- Go tests for provider-neutral task inputs, canonical remote URLs, and
  credential cleanup around Azure clone processes.
- Component and Playwright coverage for immediate availability, Enabled chips,
  provider grouping, preset/saved-view behavior, and mobile parity.
- Go REST/service/controller tests for team and board discovery, backlog-order
  hydration, dynamic column/done fields, allowlisted JSON Patch generation,
  revision conflicts, and mock mutation.
- TypeScript API/hook/view-model tests for dependent selector resets, board
  grouping, optimistic move rollback, conflict refresh, and normalized card
  updates.
- Playwright desktop and `mobile-chrome` flows for initial board load,
  assignment and column changes, reload persistence, mobile focused-column
  navigation, detail containment, and absence of document horizontal overflow.
- Go tests for immediate save-time auth health, work-item detail/comment
  pagination, PAT identity resolution, planning-field normalization, constrained
  patch operations, task/work-item associations, preset normalization, watcher
  generation/reservation safety, cleanup, and poll errors.
- User settings and frontend hook tests for per-user/per-workspace preference
  hydration, queued optimistic writes, stale provider fallback, and no browser
  storage dependency.
- Component tests for read-only detail, section retries, Assign to me/Unassign,
  quick-action payloads, linked-task indicators, and responsive watcher
  controls.
- Playwright desktop and `mobile-chrome` flows for restored filters, detail and
  discussion, assignment/column changes, quick task creation, immediate
  availability, and watcher create/run/reset.
- Watcher persistence tests must cover cross-workspace ID access, restart
  persistence, generation-aware reserve/attach/release, deleting state,
  reset preview cleanup sets, and workspace deletion.
- Poller/dispatch tests must cover bounded initial checks, duplicate matches
  after restart, provider failures, terminal cleanup policies, exact metadata
  keys, in-flight throttling, ownership loss, and dependency self-heal.
- Detail tests must cover independent core/comment errors, continuation-token
  append/dedup, unsafe HTML removal, search-result assignment, conflict
  rollback, board-only column controls, linked-task cache invalidation, and
  association failure.

## Verification

- `make fmt` — passed on 2026-07-31.
- `make typecheck` — passed on 2026-07-31.
- `make test` — passed on 2026-07-31.
- `make lint` — passed on 2026-07-31 (Go, web, and harness checks).
- Task files define the exact focused Go and Vitest commands for each new
  behavior; run those during TDD before the corresponding task is marked done.
- `pnpm e2e:run --host --no-build tests/integrations/azure-devops.spec.ts -- --project=chromium` from `apps/web` — passed.
- `pnpm e2e:run --host --no-build tests/integrations/mobile-azure-devops.spec.ts -- --project=mobile-chrome` from `apps/web` — passed.
- Managed desktop and mobile `pnpm e2e:run` equivalents — passed after the
  production build.
- `node --test scripts/validate-public-docs.test.mjs` and
  `node scripts/validate-public-docs.mjs` — passed on 2026-07-31.

## Risks

- Azure organization URLs are an outbound-request boundary. V1 accepts only
  canonical HTTPS `dev.azure.com/<organization>` URLs to avoid an SSRF-capable
  arbitrary host setting.
- WIQL returns references rather than hydrated work items and Azure caps batch
  retrieval at 200 IDs; ordering and partial omissions require explicit tests.
- Azure reviewer votes and branch policies do not map one-to-one to GitHub
  reviews and checks. Only summary states are shared.
- Existing task PR UI is GitHub-heavy. The implementation must extract only the
  smallest provider-tagged presentation contract required for Azure, not begin
  a broad GitHub/GitLab refactor.
- The task creation API and branch loader are GitHub-named today. Compatibility
  fields must remain accepted while internal contracts become provider-neutral.
- Azure PAT clone auth must not leak through process arguments, persisted remote
  URLs, executor metadata, structured logs, or agent-visible environment state.
- Remote executors receive credential-free clone URLs. A private Azure repo is
  guaranteed to clone through the backend materialization path; remote executor
  push/clone credentials remain separately configured.
- Existing workspace PATs may have only Work Items Read. Board reads continue
  to work, while mutations can return 403 until the user replaces the PAT with
  Work Items Read & write; the UI must preserve readable board data and show
  reconnect guidance.
- Board column and done field reference names are provider data and can differ
  by team/process. The browser sends column IDs only; the backend must derive
  and validate all provider-native patch paths and values.
- Azure board snapshots can exceed one work-item batch and can contain deleted
  references. Preserve backlog order, omit missing items, and keep the
  remaining board usable.
- Azure work-item types expose different estimate fields. Normalize only the
  documented planning-field allowlist and omit unavailable values instead of
  guessing a universal Effort field.
- Discussion uses a preview-version Azure endpoint and opaque continuation
  tokens. Keep paging isolated behind the Azure client contract.
- The PAT represents one Azure identity for the whole workspace; Assign to me
  means that identity, which may differ from the signed-in Kandev user's email.
- Portable preference writes can race rapid filter changes. Use one queued
  patch stream and ignore stale loads so an older response cannot overwrite the
  most recent in-memory selection.
- Watchers can fan out task creation. Reuse generation reservations, profile
  dependency checks, 60-second minimum polling, and `max_inflight_tasks`
  throttling before enabling them.

## Task Waves

Wave 1: backend foundation

- [x] [Task 01: Workspace configuration](task-01-workspace-configuration.md)
- [x] [Task 02: REST client](task-02-rest-client.md)

Wave 2: backend product reads

- [x] [Task 03: Work-item and PR services](task-03-read-services.md)
- [x] [Task 04: Task PR persistence and backend wiring](task-04-task-pr-wiring.md)

Wave 3: frontend

- [x] [Task 05: Frontend data and settings](task-05-frontend-settings.md)
- [x] [Task 06: Responsive browse and task PR UI](task-06-frontend-browse.md)

Wave 4: integrated validation

- [x] [Task 07: E2E, security review, and documentation](task-07-e2e-security-docs.md)

Wave 5: integration navigation and Azure browse UX

- [x] [Task 08: Immediate availability and integration identity](task-08-availability-and-identity.md)
- [x] [Task 09: Azure presets and saved views](task-09-azure-presets.md)

Wave 6: provider-neutral repository selection

- [x] [Task 10: Remote repository contracts and discovery](task-10-remote-repository-contracts.md)
- [x] [Task 11: Secure Azure repository materialization](task-11-secure-azure-clone.md)
- [x] [Task 12: Unified task repository picker](task-12-unified-repository-picker.md)

Wave 7: integrated validation

- [x] [Task 13: Cross-provider E2E, security review, and documentation](task-13-enhancement-validation.md)

Wave 8: editable Azure board backend

- [x] [Task 14: Board discovery and snapshots](task-14-board-discovery.md)
- [x] [Task 15: Revision-safe work-item mutations](task-15-board-mutations.md)

Wave 9: responsive board UI

- [x] [Task 16: Editable desktop and mobile board](task-16-board-ui.md)

Wave 10: board validation and documentation

- [x] [Task 17: Board E2E, docs, and verification](task-17-board-validation.md)

Wave 11: connection, preference, and work-item contracts

- [x] [Task 18: Immediate connection activation](task-18-immediate-activation.md)
- [x] [Task 19: Portable Azure browse preferences](task-19-browse-preferences.md)
- [x] [Task 20: Work-item detail contracts](task-20-work-item-detail.md)
- [x] [Task 21: Constrained work-item mutations](task-21-work-item-mutations.md)
- [x] [Task 22: Task work-item associations](task-22-task-work-item-links.md)
- [x] [Task 23: Azure provider presets](task-23-provider-presets.md)

Wave 12: watcher backend

- [x] [Task 24: Azure watcher persistence](task-24-watcher-persistence.md)
- [x] [Task 25: Azure watcher polling](task-25-watcher-polling.md)
- [x] [Task 26: Azure watcher dispatch](task-26-watcher-dispatch.md)

Wave 13: responsive frontend

- [x] [Task 27: Responsive work-item detail](task-27-work-item-detail-ui.md)
- [x] [Task 28: Azure watcher settings](task-28-automation-settings.md)

Wave 14: integrated validation

- [x] [Task 29: Azure enhancement validation](task-29-enhancement-validation.md)

Tasks within a wave are listed separately for ownership clarity but should run
sequentially in the current workspace when they touch the same package or state
composition files.
