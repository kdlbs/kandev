---
status: draft
system: plugins
created: 2026-10-30
requirements:
  - REQ-PLUGINS-MARKETPLACE-001
  - REQ-PLUGINS-MARKETPLACE-002
owners:
  - kandev
---

# Native-plugin attribution system design

## Purpose and boundaries

Plugins owns this contract because it owns catalog metadata and installed
manifest presentation. The UI system supplies reusable settings surfaces.
The [marketplace design](marketplace.md) retains source configuration,
installation, caching, and publication ownership.

This design changes attribution for native entries in the official registry.
It keeps manifest authors as declared package data. It introduces no publisher
verification, install receipt, signature claim, persistence schema, or API field.
The [canvas distribution design](../../canvases/system-design/marketplace-sharing.md)
retains canvas author semantics and package review.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-PLUGINS-MARKETPLACE-001` | Catalog projection, Retained records, Installed metadata and responsive presentation |
| `REQ-PLUGINS-MARKETPLACE-002` | Retained records, Publication and recovery |

Applicable criteria are `AC-PLUGINS-MARKETPLACE-001.4`, `.5`, `.9`, `.10`,
and `AC-PLUGINS-MARKETPLACE-002.3`.

## Catalog projection

`plugin-registry/plugins.yaml` supplies the reviewed `owner/name` pointer.
For native plugins, `buildEntry` in `plugin-registry/build-index.mjs` projects
the owner segment into the existing catalog `author` field. Omitted `kind`
means native plugin. `kind: canvas` keeps its existing inspected-author path.

The curated pointer supplies the native identity even when GitHub metadata
returns a different login, omits the owner, or fails. Metadata still supplies
stars and update time. A repository transfer requires a reviewed pointer update.
There is no special mapping from `kdlbs` to the display name `kandev`.

Native `repo_url` remains the canonical `https://github.com/<owner/name>`
from that same pointer. The current native manifest parser already excludes
`repo_url`. Preserve that protection instead of expanding manifest authority.
Manifest display name, description, icon, categories, and compatibility remain
presentation data. Package verification remains mandatory.

`schema_version: 1` and the frontend `MarketplaceEntry` contract stay compatible.
`MarketplaceEntryRow` continues to show the catalog's `author` through
`plugins:byAuthor`. The frontend does not derive identity from an installed
manifest, arbitrary repository URL, or matching plugin ID.

## Retained records

`buildIndex` uses `trustedPriorRecord` only after its existing unique-ID,
canonical-repository, version, and package-URL checks succeed. For an eligible
native record, copy the record and replace only `author` with the current
curated owner. Do not modify the caller's prior document.

Preserve the retained package URL, version, digest, and other release metadata.
Do not count an attribution correction as a fresh release. A mixed rebuild
needs at least one successful fresh entry before publication. An all-failed
rebuild still fails before replacement, even when all prior records are eligible.
Canvas retention keeps its existing behavior.

## Installed metadata and responsive presentation

`PluginManifestCard` shows the stored `PluginRecord.author` unchanged.
The existing `plugins:manifestAuthor` translation becomes `Declared author`.
Update English, pseudo, Portuguese, Simplified Chinese, Traditional Chinese,
Japanese, and Korean catalogs. Generate the Traditional Chinese pair with
`pnpm run i18n:zh-hant`.

Keep the raw manifest, unavailable marker, signed status, and guarded repository
link semantics. A declaration does not grant publisher status. Catalog
membership or a catalog lookup must not upgrade an installed declaration into
verified evidence.

This is a content change within the existing settings page. Desktop keeps the
manifest grid. Phone keeps direct plugin-detail navigation, its single-column
manifest, and the settings scroll container. The nearest shipped exemplar is
`e2e/tests/plugins/mobile-plugin-settings-row.spec.ts`, which proves navigation
and the phone scroll owner. Localized labels must fit without document overflow.
No new interaction, overlay, fixed control, or scroll region is required.

## Publication and recovery

The official index workflow rebuilds the corrected metadata through its existing
triggers. Release polling alone need not detect an attribution-only correction,
because the release version can remain unchanged. The source change, a manual
rebuild, or the daily fallback supplies the rebuild path.

Readers see the new identity after successful Pages publication and catalog
refresh or cache expiry. A provider-wide outage keeps the older published
document until a successful rebuild. The implementation must not claim that
the older document changed during that outage.

Existing builder warnings and retention reports remain the diagnostic evidence.
No new metrics or credentials are required.

## Trust limits and alternatives

The curated owner identifies the catalog source repository. It does not prove
who built a package, verify installed files, or make a custom registry trustworthy.
Operators retain the existing explicit trust decision for additional sources.

Correcting individual manifests cannot enforce this contract. Replacing the
installed author with a matching catalog owner would assert unsupported provenance.
These alternatives therefore do not satisfy the required attribution boundary.

[PR 3793](https://github.com/kdlbs/kandev/pull/3793) proposes a broader publisher
verification contract. Reconcile this design if that work lands first. Its
unmerged documents are not the authority for current requirement IDs.

The existing [central publication decision](../../../decisions/2026-08-30-central-curated-plugin-release-polling.md)
continues to own curation and release retention. This bounded metadata correction
needs no additional architecture decision.

## Implementation plan

- [Plugin marketplace attribution](../../../plans/plugin-marketplace-attribution/plan.md)
