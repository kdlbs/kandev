---
status: draft
system: workspaces
requirements:
  - REQ-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001
---

# Remote Repository Registration System Design

## Purpose and boundaries

The workspace system owns the API and settings flow that turns a remote
repository locator into a workspace repository record without a task. The
design reuses, and does not fork, the verification contract that task creation
already applies to remote locators: the built-in URL parser for GitHub,
GitLab, and Azure DevOps, and the plugin system's repository inspection for
plugin providers (see the
[plugin repository task creation design](../../plugins/system-design/repository-provider-task-creation.md)).

The task system keeps task creation and workspace-source attachment. The
integration and plugin systems keep provider calls and credentials. The UI
system keeps the shared settings page chrome.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.1` | [Settings entry](#settings-entry) |
| `AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.2` | [Remote dialog](#remote-dialog) |
| `AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.3` | [Registration API](#registration-api), [Security](#security) |
| `AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.4` | [Registration API](#registration-api), [Persistence](#persistence) |
| `AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.5` | [Remote dialog](#remote-dialog) |
| `AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.6` | [Failure and recovery](#failure-and-recovery) |
| `AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.7` | [Security](#security) |
| `AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.8` | [Responsive composition](#responsive-composition) |

## Components and responsibilities

- `internal/task/handlers.RepositoryHandlers.httpRegisterRemoteRepository`
  (`repository_remote_handlers.go`) owns the HTTP route, request binding, the
  read-only workspace refusal, and error mapping.
- `internal/task/service.Service.RegisterRemoteRepository`
  (`service_remote_repositories.go`) owns authorization, locator preflight,
  and find-or-create resolution.
- The existing `preflightRepositoryInputs` and `ResolveRepositoryRef` paths
  own verification and repository lookup. This design adds no second
  resolver.
- `apps/web/app/settings/workspace/workspace-add-repository-menu.tsx` owns
  the **Add repository** menu.
- `apps/web/app/settings/workspace/workspace-add-remote-repository-dialog.tsx`
  owns the remote dialog and submits through
  `registerRemoteRepositoryAction`. It composes the existing `RemoteRepoChip`
  with `useRemoteRepositories` and `useBranchesByURL`, so provider coverage is
  identical to the New Task Remote tab.
- `workspace-remote-repository-registration.ts` builds the request body from
  a picker row: GitHub and pasted URLs send the bare locator; other providers
  also send the picker's descriptor hints.

## Data and contracts

### Registration API

`POST /api/v1/workspaces/:id/repositories/remote`

Request:

```json
{
  "remote_url": "https://git.example.test/team/api.git",
  "default_branch": "main",
  "provider": "forgejo",
  "provider_host": "https://git.example.test",
  "provider_scope": "",
  "provider_repo_id": "42",
  "provider_owner": "team",
  "provider_name": "api"
}
```

Only `remote_url` is required. Every other field is a hint from the picker.

Response: the existing `Repository` DTO. `201 Created` when this call
inserted the record, `200 OK` when the verified provider identity matched an
existing record.

The service builds one `TaskRepositoryInput` from the body with
`ResolveProviderDefaults` set, runs `preflightRepositoryInputs` (plugin
inspection replaces the input with the trusted descriptor; built-in URLs are
parsed and provider-mismatch checked), then calls `ResolveRepositoryRef`,
which reaches `FindOrCreateRepository`. The returned repository is re-read by
ID so the response reflects the persisted row.

### Settings entry

The Repositories section action is `AddRepositoryMenu`, a `DropdownMenu`
with two items. The local item calls the existing discover-dialog opener; the
remote item opens `AddRemoteRepositoryDialog`.

### Remote dialog

The form that owns the picker mounts only while the dialog is open, so
provider catalogs are requested when the user opens the dialog, not on every
settings page visit. The form holds one `TaskRemoteRepoRow`. Picker selection
fills the row's URL, provider hints, and default branch; a pasted URL fills
only the URL and is inspected through `usePRInfoByURL(workspaceId)`, the same
inspection task creation uses: when a registered plugin provider claims the
URL, its inspection descriptor (clone URL, provider identity, default branch)
replaces the bare URL on the row before submission is allowed. Branch loading
uses `useBranchesByURL(workspaceId)`, which already routes plugin providers
through their `repositories.branches` action. Confirmation is enabled only
with a non-empty URL whose inspection has settled without error. On success
the page inserts the returned repository into the saved baseline and the
rendered list; a repository the page already lists keeps its loaded entry and
scripts, since the registration response carries no scripts.

## Control flow

1. Browser: picker selection → `remoteRepositoryRegistrationPayload(row)` →
   `POST .../repositories/remote`.
2. Handler: bind JSON, refuse empty `remote_url` (400), refuse the read-only
   workspace (409, existing helper), call the service.
3. Service: `AuthorizeWorkspaceScope(repository.manage)` → preflight →
   `ResolveRepositoryRef` → `GetRepository`.
4. Handler: `201` or `200` with the DTO; errors per
   [Failure and recovery](#failure-and-recovery).

## Failure and recovery

| Condition | Result |
| --- | --- |
| Missing `remote_url` | `400`, no write |
| Unsupported host with no plugin provider, malformed URL, provider mismatch | `400` with `error_code: repository_selection_invalid`, no write |
| Plugin inspection reports not found | `404` with `repository_selection_not_found` |
| Plugin unavailable or inspection error | `503` with `repository_selection_unavailable` |
| Unknown workspace | `404` |
| Read-only Improve Kandev workspace | `409` |
| Workspace scope denied | `403` |

The dialog keeps the row and shows the returned `error` message in a
`role="alert"` region; a pending submission shows a `role="status"` line, and
the dialog refuses close requests (Cancel, Escape, overlay) until the request
settles. Registration enables `ResolveProviderDefaults`, so a GitHub locator
without a branch probes the provider's default branch; that probe is best
effort and a failure leaves the branch empty rather than failing registration.

Provider hints that disagree with a built-in locator (owner, name, provider,
host) are rejected by `validateBuiltInRemoteHints` as
`repository_selection_invalid` before resolution, so a stale or forged hint is
a client error rather than an internal failure.

## Persistence

No schema change. Records are ordinary `repositories` rows with
`source_type: provider`, the verified provider identity, `remote_url`, and
`default_branch`. Uniqueness follows the existing provider-identity lookup in
`FindOrCreateRepository`.

## Security

Browser-supplied provider fields are hints only. Plugin providers are
resolved through `RepositorySelectionResolver`, which replaces the hints with
the plugin's authoritative descriptor and sets the server-only
`TrustedProviderDescriptor` marker; built-in providers are re-parsed from the
URL. The route requires the `repository.manage` workspace scope through the
service, and the read-only workspace check runs before the service call.

## Responsive composition

The menu is the shared Radix `DropdownMenu`, which renders as an inset bottom
sheet below 640px; items carry 44px coarse-pointer minimum height. The dialog
reuses `RemoteRepoChip`, whose picker and branch pill already carry the phone
sizing used by the New Task Remote tab.

## Observability

Failed verifications log through the existing plugin action and task
selection warnings; the handler logs unexpected errors at error level with
the `task-repository-handlers` component.

## Related decisions

- [ADR-2026-07-20 provider-neutral remote repositories](../../../decisions/2026-07-20-provider-neutral-remote-repositories.md)
