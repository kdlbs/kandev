# Adding a new third-party integration

Jira and Linear are the model: per-workspace credentials, a 90s auth-health poller, a settings page with status banner + reconnect CTA, link/import buttons that gate on availability. New integrations should reuse the shared shapes in this folder rather than copying either.

This file covers both halves of the playbook (backend + frontend) so the pattern is readable in one place.

## Backend (`apps/backend/internal/<name>/`)

- Mirror the package layout: `service.go`, `store.go`, `client.go`, `provider.go`, `handlers.go`, `models.go`, `poller.go`. Expose `Provide(writer, reader *sqlx.DB, secrets SecretStore, eventBus bus.EventBus, log *logger.Logger) (*Service, func() error, error)`. Pass `nil` for `eventBus` when the integration doesn't publish events; both Jira and Linear take and use it for issue-watch publishing.
- Use `internal/integrations/secretadapter` instead of writing your own upsert wrapper around `secrets.SecretStore`. The adapter satisfies any per-integration `SecretStore` interface shaped as `{Reveal, Set, Delete, Exists}`.
- Use `internal/integrations/healthpoll` for the auth-health loop. Implement the `Prober` interface (`ListConfiguredWorkspaces` + `RecordAuthHealth`) on a small adapter and let `healthpoll.New("name", prober, log)` own Start/Stop/ticker. Keep integration-specific loops (JQL polling, webhook reconciliation, etc.) separate, like jira's issue-watch loop.
- Wire the service via a per-domain `init<Name>Service(...)` helper in `cmd/kandev/services.go`, not inline in `provideServices`.
- Ship a `mock_client.go` + `mock_controller.go` next to the real client. `Provide` branches on `KANDEV_MOCK_<NAME>=true` and returns the in-memory client; `RegisterMockRoutes(router, svc, log)` mounts `/api/v1/<name>/mock/*` only when the service was built with the mock. The e2e backend fixture sets the env var so Playwright tests drive the mock via `apiClient.mock<Name>*()` helpers — see jira/linear for the layout.
- **If the watcher has a per-watch `MaxInflightTasks` cap**, implement `WatchMetadataKey()` on the integration's `WatcherSource` (`internal/orchestrator/source_<name>.go`) to return the task-metadata watch-id key — the **same constant** the source writes into `BuildTaskRequest`'s `Metadata` map (e.g. `sentry_issue_watch_id`). The throttle gate (`acquireWatcherSlot`) passes that key to `CountOpenWatcherCreatedTasks(metadataKey, watchID)`, which counts open tasks for the watch. The task repository (`internal/task/repository/sqlite/task.go`) is intentionally **agnostic of integrations** — it keys purely on the metadata key, so no repository change is needed per integration. If `WatchMetadataKey()` returns `""` the gate treats the watch as uncapped and the cap **silently never applies** (Sentry originally shipped with this gap — the cap was stored and validated but never enforced because the repository's old integration switch had no `sentry` case).

## Frontend (`apps/web/`)

- Hooks live under `hooks/domains/<name>/`, **not** `components/<name>/`.
- Use `hooks/domains/integrations/use-integration-availability.ts` and `use-integration-enabled.ts` — each integration's `useXAvailable` / `useXEnabled` should be a one-line wrapper passing the storage key + sync event + config-fetch function.
- Settings page reuses `<IntegrationAuthStatusBanner>` (`components/integrations/auth-status-banner.tsx`).
- "Auth required / reconnect" UI reuses `<IntegrationAuthErrorMessage>` (`components/integrations/auth-error-message.tsx`) — supply the integration's display name, regex check, and reconnect href.
- Link / import popovers reuse `<ValidatedPopover>` (`components/integrations/validated-popover.tsx`) — supply the icon, label, key regex, fetch function, and success callback.

## Where Jira and Linear deliberately diverge

- **Issue model:** Jira uses transitions + JQL; Linear uses state IDs + structured filters. Don't merge these schemas — the upstream APIs are genuinely different.
- **Watch filter persistence:** Jira stores the JQL string verbatim; Linear stores the structured `SearchFilter` as JSON in `filter_json` (Linear has no JQL equivalent). The orchestrator emits `NewJiraIssueEvent` / `NewLinearIssueEvent` respectively and dedups by issue key (Jira) vs identifier (Linear).
- **Health column extras:** Linear's `linear_configs` row carries an `org_slug` captured from successful probes; Jira's row does not.
- **Sentry — multiple instances per workspace:** unlike the one-config-per-workspace integrations, `sentry_configs` is keyed by an instance `id` (UUID) with a `workspace_id` column + `UNIQUE(workspace_id, name)`, so a workspace holds several named instances. Secrets are keyed `sentry:instance:<id>:token`. Issue watches carry a nullable `sentry_instance_id` FK (`ON DELETE RESTRICT`); the bound instance is immutable. The HTTP surface is instance CRUD under `/api/v1/sentry/instances?workspace_id=` (no install-wide `/config`); deleting an in-use instance is 409 `SENTRY_INSTANCE_IN_USE`. See ADR-0030.
