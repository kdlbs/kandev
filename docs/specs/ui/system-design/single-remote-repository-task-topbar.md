---
status: draft
system: ui
requirements:
  - REQ-UI-REMOTE-REPO-TOPBAR-001
---

# Single remote repository task topbar system design

## Purpose and boundaries

The task detail projection already receives a `Repository` with `source_type`,
`provider`, `provider_owner`, `provider_name`, and `remote_url`. This design
changes only task topbar presentation. `repositorySlug` continues to provide
the stable `owner/repo` identity used by task cards and filters.

## Requirement mapping

| Requirement                     | Design sections                                                                                      |
| ------------------------------- | ---------------------------------------------------------------------------------------------------- |
| `REQ-UI-REMOTE-REPO-TOPBAR-001` | Repository projection, Desktop breadcrumb, Phone composition, Browser URL and fallback, Verification |

## Repository projection

`resolveTaskProps` in `apps/web/components/task/task-page-content-helpers.ts`
keeps `repositoryLabel` for existing consumers. It additionally derives a
topbar-only descriptor when `task.repositories` contains exactly one entry,
the resolved primary `Repository` matches that entry, and its `source_type` is
`provider` (the persisted source type for a provider-backed remote repository).
The descriptor carries the short display name (`provider_name`, then
`name` as fallback), full `repositorySlug` for accessible context, provider,
and a validated browser URL or `null`. Do not infer eligibility from the active
session or a slash in the label. Keep this descriptor ephemeral; no API,
persistence, or workspace contract changes are needed.

`task-page-inner.tsx` passes the descriptor to the desktop `TaskTopBar` and,
through `TaskLayout`, to `SessionMobileTopBar`. Existing `repositoryLabel`
continues to serve local and multi-repository paths.

## Desktop breadcrumb

`TaskTopBar` builds the existing project ancestor first. For an eligible
repository, its final parent crumb uses the short name, provider icon, and
external URL. The task title stays the current editable title crumb. Extend
`PageTopbar`'s `ParentCrumb` rendering narrowly to support an optional icon and
external-link attributes. Preserve the existing truncation, project ancestor,
and collapsed-parent behavior, including icon and link when the repository is
the last visible parent. External links use native `<a>` behavior with
`target="_blank"` and `rel="noopener noreferrer"`.

Use `RemoteRepositoryProviderIcon` so GitHub, GitLab, Azure DevOps, and
registered plugin providers use their existing semantic icon. A provider
without a registered icon uses the current generic repository fallback.

## Phone composition

The nearest shipped exemplar is `SessionMobileTopBar`: it has a title-triggered
task picker and a second line for branch context. Keep the back control and
task picker. For the eligible single remote repository, render the provider
icon and short repository name as a separate link before the task picker, with
the separator between them. The picker retains the task name, chevron, and
branch/diff summary. The link must not be nested inside the picker button.
Constrain the repository link before the task title, preserve the existing
header's single scroll-free row, and give the link at least a 44px touch
target. No drawer or saved mobile preference is needed for a direct external
destination. Local and multi-repository tasks keep the current secondary-line
repository label. The same descriptor and URL decision serve both viewports.

## Browser URL and fallback

Use the saved `remote_url` as the source of the browser destination only for
the first-party providers `github`, `gitlab`, and `azure_devops`, whose clone
URL path maps to the repository browser page. Accept only an HTTPS URL with a
hostname, no username/password, query, or fragment; remove one terminal `.git`
suffix and trailing slash from its path. Do not guess a website URL from an
SSH/git locator or synthesize plugin-specific routes. The repository-provider
plugin contract supplies a clone URL but no browser-page URL, so plugin
providers render a static crumb unless that contract later adds an authoritative
browser destination. For example, Bitbucket Server's
`/scm/TEAM/fixture.git` clone route does not identify its
`/projects/TEAM/repos/fixture` browser page. This also prevents a credential or
unsupported scheme from becoming a topbar link. The backend remains responsible
for the stored credential-free clone URL; the UI validates the navigation
target at render time.

The link's visible label is short, while its localized accessible name names
the provider and full repository identity. The icon is decorative inside that
named link. Add copy in all required locale catalogs.

## Verification

Unit tests cover exactly-one/remote eligibility, local and multi-repository
fallback, short name, provider icon, valid and invalid URL handling, the
Bitbucket Server clone-route fallback, plugin-provider fallback, registry
changes after mount on desktop and phone, and the external anchor attributes.
Desktop and mobile Playwright scenarios assert that a supported first-party
remote link has the expected browser URL while the task title remains usable.
The phone scenario also checks a touch target of at least 44px, truncation, and
no horizontal document overflow. A focused rendered check at desktop and Pixel
5 widths compares the final structure with the plan preview.

## Related contracts

- [Mobile task chrome](mobile-task-chrome.md) owns the existing phone task
  picker and compact header.
- [Task navigation](task-navigation.md) owns first-party task links; this
  external repository link retains browser navigation.

## Implementation plans

- [Single remote repository task topbar](../../../plans/single-remote-repository-task-topbar/plan.md)
