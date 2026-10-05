---
status: current
system: integrations
created: 2026-10-05
requirements:
  - REQ-INTEGRATIONS-ENABLE-DISABLE-TOGGLE-001
---

# Integration Enable/Disable Presentation System Design

## Ownership and mapping

Integrations owns the provider connection and workspace preference used by its
Settings navigation badge. Generic tree composition remains UI-owned.

| Requirement | Design boundary |
| --- | --- |
| REQ-INTEGRATIONS-ENABLE-DISABLE-TOGGLE-001 | Workspace preferences, navigation and badge projection |

## Workspace preferences and navigation

`useIntegrationEnabled` and `useIntegrationEnabledReader` own the existing
per-workspace localStorage preference, legacy migration and storage/custom-event
subscriptions. `INTEGRATION_ENABLED_KEYS` and `INTEGRATION_ENABLED_SYNC_EVENTS`
are the shared identity catalog. No keys or persistence rules change.

The index and provider settings controls use `useDraftedIntegrationEnabled`
with the Settings save coordinator. Navigation consumes saved preferences;
drafts remain local until Save. `useSettingsMenuBranches` continues controlling
row visibility through the hide-disabled preference. Badge eligibility is a
separate projection and never removes a row itself.

## Badge projection

`useEnabledIntegrations(workspaceId)` combines each built-in provider's existing
connection predicate with `readEnabled(INTEGRATION_ENABLED_KEYS[slug], workspaceId)`
from a subscribed `useIntegrationEnabledReader`. Its memo depends on that reader
as well as connection signals, so saving a toggle or a storage event recomputes
the eligible slug set. Keep the exhaustive `Record<IntegrationSlug, boolean>`.

GitLab connection status must be read with `useGitLabStatus(workspaceId)`;
`useGitLabAvailable()` implicitly reads the active workspace and cannot identify
a different workspace's tree row. GitHub already accepts an explicit workspace.
Azure DevOps, Jira, Linear and Sentry keep their existing connection predicates.
GitHub and GitLab continue accepting authenticated OR token-configured status.
This changes badge projection only, without gating provider operations.

`IntegrationsEnabledProvider` shares one eligible set per workspace branch.
`IntegrationEnabledBadgeFor` renders the existing localized badge from this set.
Plugin badges retain the registry's workspace-scoped enabled map and subscription.
Clarify source comments that currently equate connection with enablement.

## Failure, persistence and responsive behavior

Unresolved or unavailable connection status produces no badge. Existing storage
fallbacks and migration rules remain intact. Saving toggles introduces no backend
write, health mutation, metric or request beyond existing connection reads.

The Settings sidebar consumes this projection. On phones that sidebar is
hidden; `/settings` is the navigation menu and has no equivalent badge row.
Layout, hit targets, scroll ownership and route composition remain unchanged.
The rendered change is removing or restoring the existing desktop Enabled badge.

## Validation

Hook tests cover all six built-ins, connected-but-disabled status, re-enabling,
same-tab and cross-tab events, unsaved drafts and explicit workspace isolation.
A focused rendered badge test uses the real projection with mocked connection
probes. The existing integrations toggle Playwright suite adds a saved GitLab
disable/re-enable check against the Settings tree on desktop.
This is state normalization within existing markup; the same targeted component
proof satisfies the mobile-parity exception without a new phone interaction test.

## Related decisions

- [Workspace ownership](../../../decisions/0030-workspace-scoped-integration-settings.md)
- [Settings manual Save](../../../decisions/0046-settings-route-save-coordinator.md)
