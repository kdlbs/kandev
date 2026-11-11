---
id: "02-plugin-connection"
title: "Connection with PAT, repositories, Git credentials"
status: in_progress
wave: 2
depends_on: 
  - "01-plugin-bootstrap"
  - "08-core-removal"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-001
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-003
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-006
acceptance_criteria:
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-001.2
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-001.3
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002.1
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002.6
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002.7
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002.8
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002.9
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002.11
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002.12
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-003.2
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-003.3
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-003.4
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-006.1
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-006.2
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-006.3
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-006.4
system_design:
  - ../../specs/integrations/system-design/azure-devops-plugin.md
---

# Task 02: Connection with PAT, repositories, Git credentials

## Summary

Port the REST client, PAT connection, settings form, health polling, repository provider, branch listing, URL inspection, and Git credential handler. The plugin runs against the sibling Kandev checkout that carries the removal (task 08 code), where `azure_devops` is claimable.

## In scope

- Port of `rest_client.go`, `client_models.go`, `auth.go`, `ValidateOrganizationURL`, `parseAzure*Remote`, and the mock client into `internal/azure` and `internal/mock`.
- `connection` state with generation, secrets `ws.<ws>.pat`, actions `connection.get/save/test/copy/delete`, i18n catalogs for all host locales, `registerIntegrationSettings` connection form (UI-01/02 without the Microsoft block).
- Health poll every 90 s and `IntegrationAuthStatusBanner`.
- `registerRepositoryProvider` (paged projects then repositories, branches, inspect of the four URL forms), `GitCredentialHandler` with bindings and the exact-scope checks.

## Out of scope

- Microsoft sign-in (task 03).
- Browse pages (task 04).

## Acceptance

- With a PAT, a local task and a worktree task clone a private Azure repository through the plugin, and the agent pushes a branch.
- Settings save, test, copy, and delete a PAT connection on desktop and phone; an existing `azure_devops` repository row from before the removal resolves to the plugin.

## ASCII UI preview

See UI-01 and UI-02 (without the Microsoft block) in the [plan](plan.md#ascii-ui-preview). This work order owns the rendered result of those views.

## Verification

Commands run from the Kandev repository root; the plugin worktree is its sibling.

```bash
(cd ../JnManso-kandev-plugin-azure-devops-main && go test -race ./... -count=1)
(cd ../JnManso-kandev-plugin-azure-devops-main && npm test && make package)
```

## Files likely touched

- `../JnManso-kandev-plugin-azure-devops-main/internal/azure/`
- `../JnManso-kandev-plugin-azure-devops-main/internal/plugin/`
- `../JnManso-kandev-plugin-azure-devops-main/internal/store/`
- `../JnManso-kandev-plugin-azure-devops-main/ui/src/settings/`

## Dependencies

Task 01-plugin-bootstrap, Task 08-core-removal

## Risks

- None

## Parallelism

`sequential`

## Inputs

- [System design](../../specs/integrations/system-design/azure-devops-plugin.md)
- `apps/backend/AGENTS.md`, `apps/web/AGENTS.md`, `docs/public/plugins-authoring.md`, and the `/create-kandev-plugin`, `/e2e`, and `/mobile-parity` skills where relevant

## Results

- 2026-11-05: implemented on plugin branch `feat/bootstrap` (uncommitted), on top of task 01.
  - `internal/azure`: the built-in `rest_client.go`, `client_models.go`, `board_models.go` and their tests ported unchanged apart from the package name; `ValidateOrganizationURL`, `TestConnectionResult`, and the validation helpers moved into `config.go`. New: `ListAllRepositories` (organization-wide), `GetRepository`, `ParseRemoteURL` for the four remote forms (`dev.azure.com`, `<user>@dev.azure.com`, `ssh.dev.azure.com:v3`, `<org>.visualstudio.com` with optional `DefaultCollection`), and `CanonicalCloneURL`.
  - `internal/store`: workspace state `connection` (organization, default project, `authMethod: pat`, `reauthRequired`, `generation`) and a separate `health` record keyed by generation, so the poller never overwrites settings. Secret `ws.<workspaceId>.pat`.
  - `internal/plugin`:
    - Actions `connection.get/save/test/copy/delete`, `projects.list`, and `workspaces.describe`. `workspaces.describe` supplies the names for the copy picker, because the frontend SDK exposes only workspace IDs.
    - Save writes the secret, then the state, and restores the previous secret if the state write fails. It increments the generation only when the organization or token changes, then records health immediately.
    - Copy runs on the target workspace. It copies the organization and default project, never the token, and sets `reauthRequired` unless the target already holds a token for the same organization. The action context verifies only one workspace, so copying a secret from a body-supplied source would bypass authorization.
    - A health poller runs every 90 s over the workspaces from `Host.Workspaces()`.
  - Repository provider: `repositories.list` (enabled repositories, query filter, sorted by project and name, cursor bound to organization and generation), `repositories.inspect`, and `repositories.branches`.
    - Descriptors use `provider_scope = <org>/<project>`, the repository GUID, the canonical clone URL, and the default branch.
    - Branches fall back to project and name for rows created by the built-in integration (`RepositoryNameResolver`).
  - Git credentials: `ResolveGitCredential` and `GetGitCredentialBinding` check provider, task, session, and repository IDs, host `dev.azure.com`, and the connected organization. The workspace repository row must be `azure_devops`, and its recorded remote must name the same organization, project, and repository as the requested path. The handler returns `kandev` and the PAT for 10 minutes and binds to `azure-devops-credential:<generation>`.
  - Template recipe aligned with the current host contract: no-match inspect returns `{"matched": false}` (a `null` repository is rejected by the host), and descriptors carry `provider_repository_id`.
  - Manifest: `capabilities` `state`, `secrets`, `api_read: [workspaces, repositories]`; ten workspace-scoped actions; `repository_providers: ["azure_devops"]`.
  - UI:
    - `registerIntegrationSettings("azure-devops")` with organization URL, PAT, default project, test, save or update, remove (dialog on desktop, drawer on phone), copy from another workspace, `IntegrationAuthStatusBanner`, a reauth alert, and a 30 s refresh.
    - `registerRepositoryProvider("azure_devops")` through `createRepositoryProvider`, extracted from the recipe.
    - Catalogs for `en`, `pt-pt`, `zh-cn`, `zh-hk`, `zh-tw`, `ja`, `ko`, and `pseudo`.
- Verification:
  - `go test -race ./...` passes.
  - `make test` passes, including Vitest 51 tests with catalog parity, placeholders, no em dash, and translated values.
  - `make check-format vet`, `make verify-package`, and `npm audit` (0 vulnerabilities) pass; `go mod tidy` is stable.
  - Smoke on a disposable Kandev built from the removal branch, with the host-platform package installed through `/api/plugins/install`:
    - The plugin is active; `connection.get`, `workspaces.describe`, `repositories.list` (409 unconfigured), `repositories.inspect` (`matched: false`), save, validation (400), and delete behave through the host.
    - A save against real Azure DevOps with a dummy token for a nonexistent organization recorded `lastError` "organization was not found (404)", and the banner showed it.
    - Settings page screenshots: desktop before and after save, remove dialog, and phone (controls stacked full width).
- Open for this task's acceptance (owner, on `dev.azure.com/contoso`): with a real PAT, a local and a worktree task clone a private repository and the agent pushes; an `azure_devops` repository row created by the built-in integration lists branches and clones through the plugin.
- 2026-11-09: enable switch added after manual testing found the built-in slider had not been carried over.
  - `ui/src/integration-enabled.tsx`: store over `host.storage` (workspace scope) and the `registerIntegrationSettings` `action`.
  - Limit: a plugin cannot hide its main sidebar entry, recorded in the requirement's known differences. Kandev hides the settings menu row itself.
  - Verification: Vitest, `go test -race ./...`, and `make e2e` (13 of 13, with a desktop save-and-reload case and a phone card case).
- 2026-11-10: the connection form saves through the native Save changes bar (AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002.12) after manual testing found the Update button always active, silent about a changed organization, and unclear about what it saved.
  - `connectionFormState` in `ui/src/connection-model.ts` gives dirty, savable, and the reason; the Update button is gone, and a changed organization of a Microsoft account shows a sign-in message.
  - Verification: Vitest 100 tests (8 new form-state tests), `go test ./...`, `make check-format vet`, and `make e2e` 14 of 14. The PAT test saves through the bar and checks the token message. The Microsoft test checks the organization message and saves a default project through the bar.
- 2026-11-10 (owner test on `contoso`): a task clone failed with "Azure DevOps Git credential unavailable". Kandev had used a provider repository it recorded without a clone URL (the replacement for a repository whose checkout lies in a task worktree), and the credential check compared the requested path only with the clone URL. The check now falls back to the repository's provider host, project, and name in the connected organization; a row with neither still gets nothing. `TestResolveGitCredentialForARepositoryRecordedWithoutACloneURL` failed before the change and passes after it; `TestResolveGitCredentialRejectsAMismatchedRepositoryIdentity` covers no identity, no or another host, and another project or repository. Kandev's replacement row now also keeps the clone URL (task 08).
