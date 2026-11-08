---
status: current
system: workspaces
updated: 2026-11-08
requirements:
  - REQ-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001
  - REQ-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-002
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

Only `remote_url` is required. Provider fields are identity hints from the
picker; `default_branch` is the user's workspace setting. A supplied branch
is trimmed and validated through ordinary repository creation, while an
omitted branch uses the verified provider default. Reusing an existing
repository preserves its branch and other saved settings.

Response: the existing `Repository` DTO. `201 Created` when this call
inserted the record, `200 OK` when the verified provider identity matched an
existing record.

The service builds one `TaskRepositoryInput` from the body with
`ResolveProviderDefaults` set, runs `preflightRepositoryInputs` (plugin
inspection replaces the input with the trusted descriptor; built-in URLs are
parsed, provider-mismatch checked, and origin verified), then calls `ResolveRepositoryRef`,
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
replaces the bare URL on the row before submission is allowed. A pasted row
does not expose list-derived branch defaults until that inspection descriptor
has been applied; a user-selected branch is preserved. Branch loading
uses `useBranchesByURL(workspaceId)`, which already routes plugin providers
through their `repositories.branches` action. Confirmation is enabled only
with a non-empty URL. Pasted URLs require inspection to settle without error;
picker rows already carry complete metadata and remain savable when optional
browser lookups fail. Branch enumeration errors never block registration. A plain
built-in repository URL settles without fetching PR or issue metadata. On success
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

Self-managed GitLab origins are admitted only after the server-owned
`GitLabRepositoryOriginVerifier` confirms the exact scheme, host, and port
against the selected workspace connection. Backend wiring reads public
connection metadata through the existing GitLab service, without resolving
credentials or making a provider request. Missing or mismatched connections
fail closed with `repository_selection_invalid`; connection-read failures
produce `repository_selection_unavailable`. Only that successful verification
sets the internal `TrustedRemote` marker consumed by shared resolution.

Shared preflight owns this verification for task creation and registration.
Registration does not repeat the origin check after successful preflight.
Unhinted URL discovery, including active VCS plugins, follows the task system's
[workspace-verified admission design](../../tasks/system-design/project-repository-context.md#workspace-verified-remote-admission).
Explicit provider selections remain pinned to that provider. Multiple successful unhinted candidates produce invalid selection before writes.
The strict parser and lower resolver guard remain in place.

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
- [Validated locator updates](../../../decisions/2026-11-08-validated-repository-locator-updates.md)

## Clone URL repair

This extension is a public-surface feature. The registration endpoint remains find-or-create and does not become an update command.
No rendered web control, plugin SDK change, schema migration, or Git-origin mutation is required.
The existing compact MCP settings tools already expose `remote_url`; repair adds domain validation and preview to that path.

### Repair mapping

| Criteria | Design section |
| --- | --- |
| AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-002.1, .5 | Repair surfaces |
| AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-002.2, .3 | Identity validation |
| AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-002.4 | Repair authorization |
| AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-002.6, .7, .8 | Repair persistence |
| AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-002.9, .10 | Compatibility and recovery |

The active [checkout-default contract](../requirements/worktree-base-refresh.md), AC-001.20 through AC-001.23, remains authoritative for its two settings.
Its [design](worktree-base-refresh.md#repository-checkout-settings-persistence) retains complete and exact write semantics.
The extension here adds omission preservation only for `remote_url`.

### Repair surfaces

The additions use existing repository handlers and service authorization:

| Surface | Inspect / validate / update / readback |
| --- | --- |
| HTTP | Existing repository GET; new `POST /api/v1/repositories/:id/remote-url/validate`; existing PATCH gains optional `remote_url`; existing GET confirms storage |
| WebSocket | Existing `repository.get`; new `repository.remote_url.validate`; existing `repository.update` gains optional `remote_url`; get confirms storage |
| MCP config | Existing `get_settings_kandev`; `update_settings_kandev` with `options.validate_only: true`; ordinary update with that option false or omitted; get confirms storage |

The validate endpoint accepts `{ "remote_url": "https://github.com/org/repo.git" }`.
The WS validation payload additionally carries `id`.
All preview paths call `Service.ValidateRepositoryRemoteURLUpdate`.
That method returns a bounded preview, not an updated `Repository`:

```json
{
  "repository_id": "registration-id",
  "workspace_id": "workspace-id",
  "current_remote_url": "git@github.com:org/repo.git",
  "effective_remote_url": "https://github.com/org/repo.git",
  "provider": "github",
  "provider_host": "https://github.com",
  "provider_owner": "org",
  "provider_name": "repo",
  "validation_scope": "locator_identity",
  "changed": true
}
```

Include saved `provider_scope` and `provider_repo_id` when present.
If the old locator contains credentials or cannot be safely represented, omit `current_remote_url` and set `current_remote_url_redacted: true`.
Plugin validation reports `validation_scope: provider_inspection`; built-in validation reports syntax and identity only, without claiming reachability.
No preview writes, event publication, registration, clone, credential resolution, or task mutation occurs.

`update_settings_kandev` accepts `validate_only` only for a repository target with exactly one change: `remote_url`.
Reject a non-boolean option, mixed preview changes, and unknown options on this repository operation.
Declare this option through `settingscatalog.DomainDescriptor.Operations` and describe the field's identity constraints.
Keep the existing sensitive-tool wrapper, registry checks, target checks, and response redaction.
No new MCP repository-update tool or task base-branch-tool extension is needed.

HTTP/WS pointer null remains omission. Preview requires a nonempty string; MCP already declares the setting non-nullable.
Empty or whitespace-only replacement is invalid, rather than a clear operation.
The preview is advisory. Update always validates the submitted candidate again; preview does not supply authority or a reusable trusted descriptor.

### Identity validation

`Service.UpdateRepository` handles every supplied `RemoteURL` through the same validator before applying updates.
The validator loads the authorized saved row and uses its workspace and provider as authority.
It must not set `TaskRepositoryInput.RepositoryID`, because that would skip inspection.
It must not call `FindOrCreateRepository`, which can create or backfill records.

Pin preflight to the saved provider, host, scope, immutable provider ID, namespace/project, and name.
Reuse `preflightRepositoryInput`, `validateBuiltInRemoteHints`, the configured GitHub/GitLab origin verifiers, and `RepositorySelectionResolver` where applicable.
Never use unhinted provider discovery to reinterpret a saved registration.
Never trust provider fields supplied alongside `remote_url`.
Reject an accompanying identity/source/path field that differs from the saved row; equal values add no authority.

Before provider resolution, require a complete clone locator with no surrounding/internal whitespace, controls, user information, query, or fragment.
Reject file/local paths, URL passwords, encoded authority tricks, traversal, ambiguous escaped separators, and non-clone resource suffixes.
Use the [conventional SCP parser](repository-locators.md#conventional-scp-grammar) for built-in SCP candidates.
HTTPS candidates retain exact provider context paths and ports. Do not discard extra components through the permissive repository browser-URL parser.

Current `parseRemoteRepositoryURL` accepts self-managed GitLab only in HTTP(S) form, even with an explicit provider hint.
For a strict SCP candidate, validate identity through its `repoclone.CanonicalHTTPSCloneURL` equivalent and preserve the requested SCP bytes for storage.
Permit this repair shape only when the saved origin is default-port HTTPS and the SCP host matches it exactly under existing host normalization.
An HTTP or nondefault-port saved origin requires an explicit HTTP(S) replacement supported by its existing provider contract.
Do not widen the shared discovery parser or infer a scheme/port from the SCP host.
The runtime materialization fix remains provider-neutral and does not need this registration identity proof.

Compare the verified result to every saved identity field after preflight.
Plugin inspection currently ignores mutable browser hints, so final comparison to the saved host, namespace, and name is mandatory.
Scope and immutable ID must also match, including empty versus nonempty scope.
The existing active manifest owner, descriptor checks, generation checks, size limits, and deadline remain mandatory.
Persist a plugin's exact verified HTTPS clone URL. Never synthesize its path from owner/name.

| Saved provider / shape | Supported replacement and identity proof | Refusal |
| --- | --- | --- |
| GitHub public or configured Enterprise | HTTPS or conventional SCP; exact saved host and owner/name under existing provider comparison rules | Unknown host, changed owner/name, extra URL resource path, credentials |
| GitLab public or configured self-managed | HTTPS, or conventional SCP for a matching default-port HTTPS origin; full saved namespace/name | Missing connection, changed port/host, truncated namespace, ambiguous origin |
| Azure DevOps | Verified HTTPS clone form only when saved scope or parseable old locator proves organization/project/repository | Incomplete organization evidence or unknown saved host; no organization guessing |
| Active plugin, scoped or unscoped | Owning provider inspection; exact saved identity tuple; exact returned HTTPS clone URL, including context root | Inactive/unavailable owner, missing immutable ID, changed descriptor identity, cross-context URL |
| Local source with complete saved provider identity | Same provider rules; keep source type and local path | Unlinked local source, unknown provider host, or conflicting identity |

Missing `provider_host` remains unknown identity, including legacy rows.
This command does not repair missing provider metadata or perform rename/import operations.
When Azure organization identity cannot be established from existing evidence, refuse that shape instead of expanding this package.
Built-in parsing proves identity, not repository existence. Plugin inspection remains authoritative for plugin paths.

### Repair authorization

Use `GetRepository` and `AuthorizeWorkspaceScope(repository.manage)` in the service before provider work.
Preserve `ErrRepositoryNotFound` for denied repository updates and the existing scoped read behavior.
The service checks the owning workspace's `IsImproveKandev()` policy for supplied remote-URL validation/update.
This keeps compact MCP and internal adapters from bypassing HTTP/WS read-only guards.
Preview requires mutation authority because it inspects a proposed managed write; ordinary read-only inspection keeps existing read authority.
Keep PAT, in-session MCP owner identity, and synthetic-auth-disabled behavior intact.

Map malformed, credentialed, identity-mismatched, and unknown-identity inputs to `repository_selection_invalid` (HTTP 400).
Use existing plugin not-found/unavailable codes (404/503), service version conflict (409), and read-only conflict (409).
WS and MCP retain their transport envelopes and equivalent domain categories.
Messages and new logs must not echo proposed or invalid saved URLs, provider raw errors, or credentials.

### Repair persistence

Extend the existing ordinary repository write intent with an optional validated remote URL and an optional observed identity fence.
The fence contains repository ID, workspace, source type, local path, provider, normalized host, scope, immutable ID, namespace/project, and name.
Keep this internal; callers cannot submit a trusted fence.

For ordinary saves, omit the `remote_url` assignment unless the request supplied a validated replacement.
When a replacement is present, condition the SQL update on the observed identity and live-row status.
Do not write across an identity change during provider inspection; return the existing conflict sentinel.
No SQL transaction spans provider calls.
PostgreSQL evaluates the conditional write under statement row locking; SQLite retains its writer admission.
No new read-modify-write calculation, advisory-lock framework, or migration is needed.

Extend `repository_checkout_defaults.go` and both ordinary store entry points, including the secret-binding companion.
Capture `remote_url`, the checkout pair, and `updated_at` from the write's `RETURNING` row.
Publish the committed values only after commit succeeds.
`publishRepositoryEvent` currently omits `remote_url`; add that field for credential-free values from the committed model.
Do not echo an unsafe legacy locator through a newly added event field.
Repository DTOs already contain the URL; retain that readback and project the committed value into the update response.
On deletion, cancellation before commit, binding failure, identity conflict, or storage failure, return no successful mutation and publish no update event.
Concurrent valid replacements use the last committed explicit URL; omitted URLs preserve the current value at the statement boundary.

Keep existing exact `ExpectedUpdatedAt` storage predicates and deliberate full-model store methods.
A supplied URL through the exact service path still receives validation and the exact timestamp fence.
Do not route ordinary writes through a fallback full-snapshot method.
Other omitted fields retain their existing contracts; this is not an all-field patch conversion.
Legacy direct complete writes and find-or-create backfills remain outside the ordinary omission guarantee.

### Compatibility and recovery

Registration ID, associations, source type, local path, and provider identity remain stable.
Do not call Git, restart a runtime, alter a credential store, or touch an executor journal during repair.
Later preparation reads the corrected locator through its existing selection path.
An existing materialized checkout can still fail exact-origin reuse validation after transport changes.
That failure does not authorize retargeting its origin, discarding commits, or replacing the checkout.

Managed credential resolution already canonicalizes SSH identities in `gitcredentials.ResolveRepositoryIdentity`.
Keep that behavior and its hostile-authority tests. This feature is not a prerequisite for ordinary supported GitHub SCP credentials.
No desktop/phone composition or copy changes occur. Registered backend transport tests provide end-to-end evidence for the API workflow.

### Repair verification

Service/store tests cover same-identity updates, strict preflight, descriptor mismatch, authorization, advisory preview, identity races, and atomic bindings.
Real independent SQLite and PostgreSQL connections must cover stale ordinary saves versus URL updates in both orders.
Use causal barriers; a timestamp delay alone does not prove the interleaving.
Assert committed storage, return payload, and `repository.updated`, including failed transaction suppression.

Registered HTTP/WS tests and the actual guarded compact MCP dispatcher cover inspect, preview, update, readback, and failure parity.
Include plugin action fixtures with exact context paths and a changed descriptor between preview and update.
Do not substitute a decoder-only test or a fake service for this route evidence.

## Implementation plans

- [Settings remote registration](../../../plans/settings-remote-repository/plan.md)
- [Shared provider-neutral admission](../../../plans/office-project-self-managed-gitlab/plan.md)
- [Git repository locator repair](../../../plans/git-repository-locator-repair/plan.md)
