---
status: draft
system: plugins
created: 2026-11-02
owners:
  - kandev
requirements:
  - REQ-PLUGINS-AZURE-DEVOPS-EXTRACTION-001
---

# Azure DevOps Leaves Core System Design

## Purpose and boundaries

This design removes `apps/backend/internal/azuredevops`, the Azure DevOps web
code, and every Azure DevOps branch in shared code, in one change. A second,
one-line change lists the plugin in `plugin-registry/plugins.yaml`.

It uses but does not own the plugin repository-provider ownership model
([plugins-02](plugins-02.md)) and the Git credential broker.

The plugin side is in the
[connector plugin design](../../integrations/system-design/azure-devops-plugin.md).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLUGINS-AZURE-DEVOPS-EXTRACTION-001` | [Removal inventory](#removal-inventory), [Identity release](#identity-release), [Database migration](#database-migration), [Documentation](#documentation), [Registry entry](#registry-entry) |

## Removal inventory

Inventory taken on 2026-11-02 at `f648e79f9`.

**Backend, deleted:**

- `internal/azuredevops/` (all 43 source files and tests);
- `orchestrator/source_azuredevops.go`, `orchestrator/event_handlers_azuredevops.go`;
- `mentions/provider_azure.go`;
- `integrations/cloneauth/` when nothing else imports it;
- the Azure Repos path in `agentctl/server/process/git_pr_providers.go`
  (`isAzureReposHost`, `parseAzure*`, the `az repos pr create` call); Azure
  hosts fall through to the generic unsupported-provider result.

**Backend, edited (Azure branches removed):**

- `backendapp`: `types.go`, `services.go`, `main.go`, `helpers.go`
  (including the `/api/v1/azure-devops/` workspace prefix), `mentions.go`,
  `settings_domain_dependencies.go`, `settings_integration_operations.go`,
  `settings_operations.go`, `boot_state_routes.go`, `turn_adapters.go`;
- `settingscatalog/defaults.go` (three domains and the browse-preference blob);
- `common/config/config.go` and `catalog.go` (`azureDevOps.*` keys);
- `orchestrator/service.go`, `orchestrator/executor/executor.go`,
  `executor_resume.go`, `executor_credentials.go`, `repoclone/clone.go`;
- `task/service/service_tasks.go`, `remote_repository_resolution.go`,
  `repository_selection.go`;
- `task/repository/sqlite/sidebar_task_query_base.go`,
  `sidebar_task_query_color.go`, `task_transfer_projection_catalog.go`;
- `delivery/observe.go`, `evaluator.go`, `sweep.go`, `models.go`;
- `user/models`, `user/dto`, `user/service`, `user/store`, `user/controller`
  (the `azure_devops_browse_preferences` field);
- `persistence/requiredstores/catalog.go`, `storeconformance/adapters.go`,
  `owner_actions.go`, and the upgrade fixture manifests;
- `db/sqlguard/exemptions.json`;
- `events/types.go` (the two match events);
- `profiles.yaml` (`KANDEV_MOCK_AZURE_DEVOPS`), `i18n/locales/*.json`
  (callback keys).

**Web, deleted:**

- `app/azure-devops/`, `app/settings/integrations/azure-devops/`,
  `components/azure-devops/`, `hooks/domains/azure-devops/`,
  `lib/state/slices/azure-devops/`, `lib/api/domains/azure-devops-api.ts`,
  `azure-devops-oauth-api.ts`, `lib/types/azure-devops.ts`,
  `components/icons/azure-devops-icon.tsx`;
- `src/locales/*/azuredevops.json` and the `azuredevops` entries in
  `eslint.i18n.options.mjs`, `scripts/externalize-strings.mjs`,
  `_verbatim.json`, `settings.json` (`integrationDescriptionAzureDevops`);
- `e2e/tests/integrations/azure-devops*.spec.ts`,
  `azure-saved-views*.spec.ts`, `e2e/helpers/azure-saved-views.ts`, the mock
  helpers in `e2e/helpers/api-client.ts`, and the resets in
  `e2e/fixtures/test-base.ts`.

**Web, edited:** `src/spa-routes.tsx`, `src/settings-routes.tsx`,
`src/integration-settings-route.tsx`, `lib/navigation/core-destinations.ts`,
`lib/navigation/types.ts`, `hooks/use-nav-availability.ts`,
`components/integrations/*`, `components/settings/workspaces/workspace-section-links.tsx`,
`lib/settings-discovery/*` (including the generated contracts),
`lib/settings/integration-icons.ts`, `hooks/domains/integrations/*`,
`lib/plugins/registry.ts`, `lib/plugins/registry-provider-ownership.ts`,
`lib/state/*` (store, slices index, hydrator, defaults, types),
`lib/ssr/user-settings.ts`, `lib/types/http-user-settings.ts`,
`hooks/domains/integrations/use-remote-repositories.ts`,
`components/task-create-dialog-remote-repo-*.ts(x)`,
`hooks/domains/github/use-branches-by-url.ts`,
`components/workspace-source-picker/remote-url.ts`,
`lib/utils/external-vcs-file-url.ts`,
`hooks/domains/workspace/use-external-vcs-file-link.ts`,
`lib/utils/remote-repository-browser-url.ts`,
`lib/sidebar/repository-rule-identity.ts`, `lib/metadata-utils.ts`,
`components/task/passthrough-toolbar.tsx`, `components/task/chat/chat-status-bar.tsx`,
and the shared E2E specs that touch Azure.

**Completion check.** After the change,
`rg -i "azure.?devops|azuredevops|dev\.azure\.com" apps/backend apps/web`
matches only the database migration, the provider string where plugin-neutral
code lists example providers, generated lockfiles, and tests that pin the
removal.

## Identity release

- `CORE_PROVIDER_IDS` loses `azure_devops` and `CORE_INTEGRATION_SETTINGS_IDS`
  loses `azure-devops`; `integration-settings-route.tsx` drops its
  `azure-devops` case so the plugin fallback renders.
- Backend provider checks that special-cased `azure_devops` are deleted, so
  `isPluginManagedRepository` (`executor_resume.go`) and the broker path apply
  to existing rows. Those rows already carry `provider_owner` and
  `provider_name`, which satisfy `hasProviderRepositoryIdentity`.
- `reserveBuiltinMentionIdentities` no longer reserves the Azure reference
  kinds, so the plugin's `reference_sources` register.
- The task delivery ledger stops recognizing `azure_devops` as a first-party
  provider. Plugin-linked pull requests are not ledger evidence, as for the
  Bitbucket plugin.

## Database migration

A migration in `task/repository/sqlite/base_migrations.go` runs after the boot
backup taken by `persistence.Provide`:

1. `DROP TABLE IF EXISTS` for `azure_devops_oauth_flows` (if present),
   `azure_devops_work_item_watch_tasks`,
   `azure_devops_pull_request_watch_tasks`, `azure_devops_work_item_watches`,
   `azure_devops_pull_request_watches`, `azure_devops_task_prs`,
   `azure_devops_task_work_items`, `azure_devops_configs`.
2. `DELETE FROM secrets WHERE name LIKE 'azure_devops:%'`.
3. `UPDATE repositories SET provider_host = 'https://dev.azure.com'` for rows
   with provider `azure_devops`, an empty provider host, and a clone URL under
   `https://dev.azure.com/`. The built-in integration never recorded a
   provider host, and `gitcredentials` fails closed for a non-GitHub repository
   without one, so these repositories could not take the plugin's managed
   credentials. The host comes from the row's own clone URL; rows without a
   clone URL or on another origin stay unchanged and fail closed.

All three statements are replay-safe. The `users.azure_devops_browse_preferences`
column stays in existing databases, unused, and is removed from the fresh
schema. Tests cover fresh and replayed SQLite and env-gated PostgreSQL.
`requiredstores` and `storeconformance` lose the `azure-devops` owner, and the
upgrade fixture manifest records the dropped tables.

## Documentation

- `docs/public/integrations.md`: the Azure DevOps section becomes a short
  pointer to the plugin, its repository, and the reconnect steps. Other public
  pages lose their built-in Azure DevOps mentions.
- `docs/specs/integrations/requirements/azure-devops-integration.md` becomes
  `deprecated`; its two system designs become `superseded`. Both carry a
  banner pointing to the plugin requirements.
- `apps/backend/internal/integrations/AGENTS.md` and the root engineering
  guide drop the built-in Azure DevOps notes.
- The PR title uses a breaking-change marker so the changelog says the
  integration moved to a plugin.

## Registry entry

After the plugin's first GitHub release, a one-line change adds:

```yaml
- id: kandev-plugin-azure-devops
  repo: JnManso/kandev-plugin-azure-devops
  categories: [integrations]
```

to `plugin-registry/plugins.yaml`. The registry workflows resolve the release
and verify the package.

## Failure and recovery

- Rollback is a downgrade plus restoration of the boot backup.
- An installation that upgrades before installing the plugin loses Azure
  DevOps screens until it installs the plugin; existing `azure_devops`
  repositories keep working for clone once the plugin is connected.

## Security

- Stored PATs are deleted by the migration. The plugin stores new credentials
  in its own namespace.
- The `az` path removal removes reliance on ambient `az` credentials in
  agentctl.

## Related decisions

- [Plugin-native integration settings](../../../decisions/2026-08-05-plugin-native-integration-settings.md)
