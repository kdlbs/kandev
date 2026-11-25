---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001
---

# GitHub Repository Scope System Design

## Purpose and scope

The integration system owns the workspace's GitHub repository policy and its
settings UI. This design changes existing scope checks to support a union.
Authentication remains under [GitHub authentication](github-authentication-01.md).
Host identity remains under [GitHub Enterprise hosts](github-enterprise-hosts.md).

The implementation is based on `ea6af0d500e92a20420729a505c2b95c65707b36`.
[PR #5283](https://github.com/kdlbs/kandev/pull/5283) is present, merged at
`35c5b28cc96e327e88f1b530cf885a0c8eefaba7`. Reuse its
[picker requirement](../requirements/github-repository-picker.md) and
[picker design](github-repository-picker.md). Reconcile their mode-specific
wording with the canonical `selected` scope semantics.

The user requested a simple fix. Reuse existing fields, pickers, save flow,
query fanout, and token behavior. Add one shared membership rule and a safe
migration. New credential-broker enforcement is deferred below.

## Requirement mapping

All criteria refer to `AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001`.

| Criteria | Design section |
| --- | --- |
| .1, .2, .9 | Data and contracts, shared evaluation |
| .3, .11, .12 | Settings and pickers |
| .4 | Migration |
| .5-.7 | Existing consumers |
| .10 | Copy, reset, and installation removal |

## Data and contracts

Keep `github_workspace_settings.repo_scope_mode`, `repo_scope_orgs`, and
`repo_scope_repos`. Add `RepoScopeModeSelected = "selected"`. No new column,
table, policy framework, or parallel setting is needed.

Canonical GET responses change from `all`/`orgs`/`repos` to `all`/`selected`.
Field names and list shapes stay the same. This is an API value change:
legacy request acceptance does not preserve legacy response values. Audit every
reader that switches on the old modes, including mentions and frontend helpers.

| Request mode | Stored policy | Lists |
| --- | --- | --- |
| `all` | All | Clear both. Reject conflicting nonempty lists. |
| `selected` | Union | Patch both supplied lists. Omission/null preserves, `[]` clears. |
| Legacy `orgs` | Selected owners | Clear repositories. Reject an explicitly supplied nonempty repository list. |
| Legacy `repos` | Selected repositories | Clear owners. Reject an explicitly supplied nonempty owner list. |
| Omitted/null | Current policy | Patch supplied lists. Reject nonempty lists under effective `all`. |
| Unknown | Validation error | Commit no policy change. |

Apply the scope update atomically and preserve unrelated settings. Avoid
assigning one SQL column twice through separate mode and list handlers.
Keep existing trimming, case-insensitive deduplication, and null-as-omitted
behavior. Exact scope repositories require both owner and name.

`backendapp/settings_integration_operations.go:updateGitHubSettings` decodes the
same three fields for catalog/agent writes. It must reach the same service
validation and return the canonical response. Cover a mixed `selected` write
through that entry point as well as the ordinary settings API.

The current settings catalog descriptor has `json_type: "string"` and no
`schema.enum`. Discovery does not enumerate the modes. Update its description
to explain `all`, `selected`, and legacy input compatibility, then regenerate
the snapshot. No new enum-discovery mechanism is necessary.

## Shared evaluation

Reuse `repositoryInWorkspaceScope` in `service_routing.go`. Expose the small
pure predicate to mentions, or provide a thin exported wrapper.
For `selected`, accept a selected owner **or** an exact selected repository.
Empty selected scope denies all. Unknown nonempty modes deny access.
Legacy modes keep exclusive meaning until canonicalization clears inactive data.

Make `repoAllowedByWorkspaceScope` and existing result filters use this rule.
`mentions/provider_github.go:githubSettingsAllowRepository` keeps its workspace
and canonical-reference checks before the shared membership check.
Frontend `lib/github/repo-scope.ts` mirrors the same semantics.
Its owner-wildcard helper accepts only `all` or an explicitly selected owner.
One selected repository never authorizes its owner's other repositories.

Missing settings rows for valid workspaces retain existing `all` defaults.
Mention authorization still rejects nil or wrong-workspace settings.

## Migration

Add a focused `store_repo_scope_migration.go` helper. Call it from
`Store.initSchemaData` immediately after successful
`backfillGitHubWorkspaceOwnership()` and before the conditional
`seedLegacyWorkspaceConnections()`. The former backfills settings ownership.
The latter seeds connections, so it requires no exclusive-mode settings data.
`NewStore` completes this sequence before handlers or pollers start.

Perform these changes in one transaction:

| Existing mode | New mode | Owners | Repositories |
| --- | --- | --- | --- |
| `orgs` | `selected` | Preserve | Clear |
| `repos` | `selected` | Clear | Preserve |
| `all` or legacy empty | `all` | Clear | Clear |
| `selected` | `selected` | Preserve | Preserve |

Normalize supported mode spelling. Leave unsupported stored modes unchanged
and denied by evaluation. Migration errors abort initialization through the
existing error path. Preserve host, presets, and task credential mode.
Use portable SQL and existing transaction/rebinding helpers.

The `selected` value makes replay safe: later startup must preserve both lists.
Seed raw legacy rows in tests, because normalizing upserts hide stale entries.
One environment-gated PostgreSQL test covers migration and write behavior.
Skip the data update when an older workspace-settings table has none of the
repository-scope columns; that schema has no stored scope values to convert.

Old binaries normalize unknown `selected` to `all`. Binary-only downgrade is
unsafe. Restore the matching pre-upgrade database backup for rollback.
Mixed-version clients/writers are unsupported. A cached old SPA can submit
`orgs` and clear exact repositories under the legacy request contract.
Reload open clients after upgrade. Do not preserve an inactive list to avoid
that documented compatibility effect.

## Existing consumers

Reuse `workspace_settings_service.go` scoped query fanout. Emit existing
`org:<owner>` and `user:<owner>` branches, then exact `repo:` branches outside
those owners. Remove redundant fetch branches while retaining saved chips.
Preserve existing query terms, result filtering, provider limits, and errors.
Merge results once per canonical repository and PR/issue number.

Update `workspaceScopeFanoutTotal` for owner pairs followed by disjoint repos:
each owner contributes the maximum org/user count, and exact repo counts add.
Keep the existing unique-result lower bound. Include both lists in
`workspaceSearchScopeKey` so either change selects fresh scoped caches/cursors.

`workspaceScopeRepoFilters` supplies the union for review/issue watches without
an explicit filter. Existing final result filtering intersects explicit filters
with that union. Empty selected scope must not become an unrestricted query.
Verify `GitHubEvaluator`/`ListAutomationPRs` through the existing automation fixture.
Retain automation visibility when filtering personal search results.

## Copy, reset, and installation removal

`CopyWorkspaceSettingsToWorkspace` copies both canonical lists through existing
normalization and still excludes credentials. `copyCloneSettings` copies raw
columns through `INSERT ... SELECT`. That remains safe because migration
finishes before clone can run. Verify full clone with a test, without a new
production clone path. Preserve its distinct
[authorized connection-copy contract](../../workspaces/requirements/clone-workspace.md).

In `store_enterprise_auth.go`, change both the INSERT value `'repos'` and
ON CONFLICT assignment `repo_scope_mode='repos'` to empty `selected` policy.
Same-host reconnect retains policy.

`installationRepositorySettingsUpdater.ApplyInstallationRepositories` must
prune removed exact entries in `selected`, preserving owner grants and other
entries. Removing the final exact entry with no owners leaves empty selected
scope, which denies all. Never substitute `all` for that result.

## Settings and pickers

In `GitHubRepoScopeSection`, replace the Mode select with the existing Switch
pattern. All off shows both pickers. Submit `selected` and both lists through
the current shared save contributor. Saving All submits empty lists.
Keep local selections across an unsaved ON/OFF toggle. Preserve existing
Discard, request guards, pending edits, and failed-save retry behavior.

Preserve `repositorySource="scope-editor"` on the scope repository picker.
It uses `GET /api/v1/github/workspace-settings/repositories`, which omits saved
scope filtering while retaining personal/automation visibility. Ordinary
`GET /api/v1/github/repos` remains scope-filtered for watch and workspace consumers.
The scope organization picker must also remain unfiltered by saved scope:
do not pass `isOrgAllowed`. An exact-repository-only scope can then add an owner.
Watch organization pickers still receive the saved policy predicate.

Replace the watch selector's old-mode visibility test with owner-grant
eligibility. Keep previously saved unavailable entries removable.
Audit canonical-mode readers and fixtures. At this base, the actual E2E
`orgs`/`repos` GET assertions are in `integrations/github-workspace-settings.spec.ts`.
Also audit `helpers/api-client.ts`, `automations-pr-merged-trigger.spec.ts`, and
`task/create-task-remote-repo.spec.ts`. They contain no `repo_scope_mode`
literals at this base. Avoid unrelated fixture edits.

Keep the existing phone composition: the same settings route, vertically
stacked fields, shared draft/save state, current scroll owner, and shared picker
popovers. Reuse #5283's accessible touch controls and containment behavior.
One focused mobile spec proves mixed selection, save, and reload. No new
phone surface, viewport sizing system, or broad geometry sweep is required.
Localize changed copy in all seven real catalogs and regenerate pseudo.

## Credentials and deferred scope

`CachedInstallationCredentialProvider` already restricts named requests to
`req.RepoName`, independent of scope mode. Preserve that behavior and verify
its existing mint test. Do not change token minting, caches, or leases here.

`gitHubCredentialProvider` currently does not read workspace settings.
Adding checks to `Binding`, `RefreshScope`, and `Resolve` would create a new
access boundary, not convert an existing exclusive-mode check. Defer that
work, its new reader dependency, warm-cache reduction tests, and factory test
to a separate follow-up. Withdrawn draft AC .8 reserves its old identifier.
The issue's broader task-credential interpretation remains unresolved; this
package does not implement that new enforcement boundary.

## Verification and documentation

The [three work orders](../../../plans/github-repository-scope-union/plan.md)
own exact checks. PostgreSQL validation is centralized in work order 01.
The UI work order owns desktop/mobile evidence and a short public-docs/ADR tail.
No new metrics, policy infrastructure, broad verification phase, or separate
ADR is needed. Preserve the ownership boundaries in
[ADR 0030](../../../decisions/0030-workspace-scoped-integration-settings.md) and
[ADR 0047](../../../decisions/0047-github-authentication-ownership.md).
