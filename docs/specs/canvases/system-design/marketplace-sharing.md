---
status: draft
system: canvases
created: 2026-09-10
owners:
  - canvases
requirements:
  - REQ-CANVASES-MARKETPLACE-001
  - REQ-CANVASES-MARKETPLACE-002
  - REQ-CANVASES-MARKETPLACE-003
  - REQ-CANVASES-MARKETPLACE-004
  - REQ-CANVASES-MARKETPLACE-005
  - REQ-CANVASES-MARKETPLACE-006
---

# Canvas marketplace and sharing system design

## Purpose and boundaries

Canvases owns distribution of reusable canvas applications and creation of
workspace instances. This is one vertical contract, including Settings >
Plugins presentation. Plugins continues to own the manifest, static validator,
runtime, release artifacts, grants, and catalog machinery.

Reuse the [plugin-backed canvas decision](../../../decisions/2026-08-26-plugin-backed-web-app-canvases.md).
No new runtime or independent marketplace is introduced. The rationale for
this additive distribution profile fits this design; a separate ADR is not
needed. The [existing lifecycle](agent-authored-web-apps.md) still owns authoring,
promotion, editing, rollback, and deletion. The
[plugin web-app contract](../../plugins/system-design/isolated-web-app-contributions.md)
owns runtime isolation. This document adds portable distribution to both.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-CANVASES-MARKETPLACE-001 | Package profile; Source retention; Validation |
| REQ-CANVASES-MARKETPLACE-002 | Registry previews; UI |
| REQ-CANVASES-MARKETPLACE-003 | Export preparation; HTTP contracts; Guidance |
| REQ-CANVASES-MARKETPLACE-004 | Install preparation; Commit; Authorization |
| REQ-CANVASES-MARKETPLACE-005 | Registry; Catalog projection; Failure handling |
| REQ-CANVASES-MARKETPLACE-006 | UI; Mobile contract; Feature gates; Verification |

## Existing implementation and additive changes

- `internal/plugins/manifest.Manifest` already supports isolated `ui.web_apps`.
  `IsStaticWebAppOnly` identifies the static runtime form, but the distribution
  validator must also reject every non-canvas contribution and managed field.
- `internal/plugins/pkgtar.validateInstallManifest` requires a managed binary.
  Leave its native installation path intact. Canvas installs use
  `internal/plugins/webapp.ValidatePackage` with an additional distribution
  profile; a category is never permission to call the native installer.
- `internal/plugins/webapp.ArtifactStore.ReadFiles` reads immutable releases.
  `backendapp.canvasEditService.loadEditableCanvas` already uses it for editing.
- `backendapp.canvasAuthoringService.PublishCanvas` collects one bounded
  agentctl tar stream and stores a `webapp.Package`. Extend this path to retain
  inert build source; do not read an author's live executor during export.
- `internal/canvas.Service`, `PublishPackage`, and `internal/plugins/instances.Store`
  own durable instance/release changes. Add a transactional import entry point
  here, rather than composing browser create/publish/approve requests.
- `plugin-registry/build-index.mjs` currently reads limited flat YAML metadata
  from a GitHub tag, chooses a release tarball, and emits an advisory null hash.
  Canvas entries need package inspection and a required digest. Preserve the
  native-plugin path and do not extend its regex parser to nested canvas data.

## Package profile

Use the existing gzip-compressed tar container and root `manifest.yaml`.
Add an optional typed `distribution` block to the plugin manifest:

```yaml
id: sprint-board
api_version: 2
version: 1.0.0
display_name: Sprint board
description: A workspace board for task progress.
author: example-author
categories: [canvas]
min_kandev_version: <first-version-with-canvas-distribution>
distribution:
  schema_version: 1
  kind: canvas
  license: MIT
  source_mode: static
ui:
  web_apps:
    - key: main
      title: Sprint board
      entry: index.html
      placements: [workspace-canvas]
capabilities:
  api_read: [tasks, workflows]
```

The minimum-version placeholder is illustrative. Implementation records the
first compatible release using the repository's version metadata; exports must
never emit this placeholder. Preserve an author's higher minimum version.

The distribution profile requires exactly one static web app, workspace-canvas
placement, and optionally task-canvas placement. Reject binary runtimes, remote
backend endpoints, native UI bundles, tools, and all other contributions.
`distribution.kind` is an asserted profile, verified against the complete
manifest. Categories are presentation tags. Legacy native entries without this
block retain their behavior. Local canvases without it remain valid locally.

Required share metadata: valid stable package ID, SemVer, display name,
description, author, nonempty license identifier or custom-license label plus
`LICENSE.txt`, `source_mode`, and compatibility. Screenshots and preview URLs
are not part of this manifest profile.
Do not assign a license on the author's behalf. Preserve declared permissions
exactly; editing share metadata cannot add or remove capabilities. Package ID
and version are editable for export without renaming the live canvas.

`<id>-<version>.tar.gz` is the bundle. It contains runtime files, editable source,
`README.md`, license information, and a `checksums.txt` covering every other
file. `<id>-<version>-source.zip` contains the same project files, with paths
preserved, and is explicitly not accepted by the installer. It includes built
assets so authors can republish the downloaded version without a build.

Build source uses the reserved `distribution/source/` subtree. The plugin
runtime denies the entire `distribution/` subtree before file lookup, including
encoded/traversal forms. Entries and executable runtime asset references cannot
target that subtree. Registry images are external listing metadata.

## Source retention

Two explicit source modes avoid claiming a built bundle is a complete project:

- `static`: the retained HTML, CSS, JavaScript, and local assets are the editable
  application. This is the current no-build scaffold and the legacy default.
- `project`: the author places build inputs under `distribution/source/`,
  including the package manifest, lockfile, build configuration, and README
  with the command that writes runtime output into the package root. This
  source is retained in the same immutable artifact as its built output.

Extend the package file policy only inside that reserved source subtree to
allow bounded regular project files, including TS/TSX and YAML lock/config
files. Reject executable binaries, links, devices, nested archives, `.git`,
`node_modules`, credential files, and `.env` files; a placeholder-only
`.env.example` is permitted. Apply exclusions during source collection and
again during package validation. Reject an unsafe file rather than silently
producing an incomplete project. Existing `.canvas-root` remains internal and
is never exported. Authored source or images can still embed private content;
the export UI exposes an inventory and a review notice, not a claim of complete
secret detection.

Quick Chat materializes the complete artifact, including the source subtree,
and preserves source mode on republish. Update the embedded authoring guide and
golden inventory tests with the same contract. No new executor access method is
needed: local, Docker, and SSH continue to use the authenticated source stream.

A legacy retained release can export in static mode with visible wording that
these are the retained application files. If its author needs original build
inputs that were not retained, project export reports `source_unavailable` and
directs them to Edit and republish with source. Never reconstruct missing source
from a deleted task workspace or silently label minified output as project source.

## Registry previews

Canvas entries in both official and custom marketplace sources require one to
eight ordered `previews: [{url, alt}]` objects. The same optional field is
available for plugins. The shared shape and gallery behavior belong to the
[plugin registry preview contract](../../plugins/system-design/marketplace.md#registry-preview-images).

Each URL is an absolute HTTPS address, at most 2048 characters, without
credentials or fragments. Each alt description has 1-300 non-whitespace
characters after trimming. The URL should directly return a PNG, JPEG, or WebP
image; content-type and availability failures use the browser's image fallback.
Prefer commit/release-pinned URLs for accuracy, but changing listing images
does not require a new canvas version.

Image URLs belong to registry pointers and generated source documents, not to
the manifest or bundle. Bundle export, source download, upload/link inspection,
and confirmation have no screenshot requirement or screenshot upload step.
There is no image collection, decoding, re-encoding, package extraction, or
Pages media mirroring in this delivery. Ordinary application image assets
remain ordinary bundle files.

The first listing image is the cover. Preserve registry order through the
generated index and UI. Validate list/URL shape during registry admission and
custom-source parsing. Do not reject a valid install because a remote preview
is temporarily unavailable. A listing image is informational and must never
supply permissions, executable content, or evidence of package authenticity.

## Validation and bounded preparation

Keep current web-app limits: 10 MiB compressed package, 25 MiB expanded data,
512 files, 5 MiB per file, 64 KiB manifest, and 240-byte paths. Authoring transfer
retains its existing 30 MiB wire bound. The source ZIP has a separate 30 MiB
output ceiling; it is generated, never nested in the installable tarball.

Distribution validation requires checksum coverage, normalized unique paths,
all declared files, source-mode structure, compatibility, and
the static canvas profile. An archive hash verifies identity, not publisher
trust. No package hooks, scripts, or application code run in inspection.

Add one small preparation store owned by the canvas distribution service.
It stages files under a dedicated Kandev-home temporary directory, with opaque
random IDs bound to the authenticated user, target workspace, operation,
reviewed digest, and expiration. No token grants access without authentication.
Allow at most two preparations per user and 256 MiB of staged data per install;
reserve before receiving bytes and release on every failure. Preparations expire
after 15 minutes. Remove on cancellation/expiry and clean abandoned files on
startup; a restart invalidates uncommitted preparations. Use request-lifetime
leases during download/commit so cleanup cannot remove in-use files.

This is temporary review/download state, not another installed-package database.
The persistent installation receipt described below handles commit retries.

## Export preparation

1. Authorize access to the canvas workspace and confirm an active, valid,
   available release. Bind the request to `expected_release_id`.
2. Read that immutable artifact using `ReadFiles`; verify its digest.
3. Accept bounded author metadata; screenshots are not an export input.
   Seed the form from the manifest. Retain the current session's inputs after
   validation errors; do not create a permanent share-profile settings system.
4. Build a temporary distribution copy, generate README/checksums, and validate
   it with the same distribution validator used by imports and registry CI.
5. Return metadata, inventory, sizes, digest, and the two
   authenticated download endpoints. Both formats derive from this one copy.

Preparing or downloading does not publish a canvas release or mutate runtime
state. Before serving either download, reauthorize and compare the active
release to the captured one. Stale release or changed form input requires
preparation again. Deletion or revoked access prevents subsequent downloads.
The dialog preserves unsaved values while open; closing discards this temporary
form and best-effort deletes its preparation. Imported packages already retain
their distribution metadata, so subsequent sharing starts from those values.

## Install preparation

The Canvases section offers two inputs: file upload and a direct HTTPS bundle
URL. It does not accept a repository URL as an alternate source form. Public
download redirects are supported with a maximum of five validated redirects.
No credential-bearing URL, forwarded Kandev cookie, or ambient provider token
is sent. Bound connect/read/total time and bytes. Reject non-public destinations
including loopback, private, link-local, and metadata networks at DNS resolution
and connection time, also after redirects. E2E injects a local test transport;
production has no environment switch that disables destination validation.

Catalog installation sends `{source_id, package_id, expected_version,
expected_sha256}`. The server resolves the configured catalog entry itself and
rejects stale version/digest selection. It never trusts a browser-supplied
catalog package URL or permission list. Download and inspect the selected
tarball; the staged bytes are then used for preview and confirmation. Direct
URL/file installs compute their own digest and identify the source as unlisted.

Inspection returns actual manifest permissions via `canvas.ManifestPermissions`,
compatibility, license, provenance, and the requested workspace. Registry
previews stay attached as separately labeled listing metadata.
The catalog details page may show a published summary first, but the final
review always uses inspected package bytes. Opening details never executes a
preview iframe or grants permissions.

## Commit and persistence

`InstallPrepared` is a new canvas service method. It verifies the preparation's
user, workspace, expiry, digest, and explicit confirmation. Reauthorize the
workspace and enforce canvas/storage limits inside the mutation boundary.

Create canvas metadata, a workspace plugin instance, immutable release,
workspace grants, and an installation receipt in one database transaction.
Use the existing instance-store transaction patterns and artifact reservations.
Write/reconcile the validated artifact before committing references; a failed
transaction schedules or performs safe unreferenced-artifact cleanup without
removing a digest still referenced by another instance.

The instance keeps `plugin_id = kandev.canvas`; the package's stable identity
belongs to release metadata and import provenance. Do not add static canvases
to the native plugin registry or auto-update poller. A workspace install has
no synthetic origin task or creator session. Empty state is created by absence
of state rows. Existing runtime issuance enforces current workspace authority.

Add one canvas-owned `canvas_install_receipts` table with unique preparation
ID, user ID, canvas ID, workspace ID, package ID/version, archive digest,
optional source ID/repository URL, origin kind (`registry`, `url`, `upload`),
and timestamp. Persist only sanitized public provenance; discard signed URL
query strings and fragments. The receipt is inserted in the same transaction
as the instance and also serves as durable import provenance.

Retrying a committed preparation returns its original canvas after authorization,
including after restart. Concurrent confirmations serialize on that receipt ID.
A deliberate second installation uses a new preparation and creates a copy.
Canvas removal removes its receipt; an old request with neither receipt nor
live preparation cannot resurrect the canvas. Add fresh/migrated/replayed
SQLite schema tests and the repository's gated PostgreSQL coverage where supported.

After commit, emit the existing workspace-scoped canvas lifecycle invalidation
and return a canvas route. Keep event payloads content-free. Existing editing
may replace a local release, but preserves original import provenance. Display
the original version as provenance, not as a claim that an edited copy still
matches upstream. V1 has no update/replacement endpoint or new update badges.

## HTTP contracts

These proposed routes use the current `/api/v1/canvases` browser API. All require
authenticated human workspace authority, not canvas runtime tokens or agent MCP.

| Method and route | Input and result |
| --- | --- |
| POST `/:canvasID/exports` | JSON metadata and `expected_release_id`; returns prepared metadata and two download links |
| GET `/exports/:id/bundle` | Reauthorized `.tar.gz` attachment |
| GET `/exports/:id/source` | Reauthorized `.zip` attachment |
| POST `/install-preparations` | Multipart file or JSON URL/catalog reference, plus `workspace_id`; returns immutable review |
| GET `/install-preparations/:id` | Reauthorized review metadata |
| POST `/install-preparations/:id/confirm` | Reviewed digest and explicit approval; returns installed canvas or prior receipt |
| DELETE `/preparations/:id` | Idempotently release an owned temporary preparation |

Use typed DTOs and stable error codes: `canvas_not_found`, `invalid_package`,
`source_unavailable`,
`incompatible_version`, `package_digest_mismatch`, `review_expired`,
`review_stale`, `invalid_download_url`, and existing quota codes. Foreign and
missing preparations return the same not-found result. Attachments use safe
filenames, `nosniff`, and private/no-store cache policy. JSON never contains
package source, host paths, credentials, or executable HTML.

## Registry publication

Extend `plugin-registry/plugins.yaml` entries with optional `kind: canvas`;
absence means the current native-plugin route. Keep `id` and `repo` (GitHub
owner/name) as the curation source. Canvas authors submit this entry by PR.
The repository must have a public release with the exact
`<id>-<version>.tar.gz` asset. Do not use the existing first-tarball fallback
for canvases, because a source archive can otherwise be chosen accidentally.

Add a small Go command, `cmd/canvas-package`, exposing bounded offline inspection
through the production validator. The Node index builder downloads the exact
release asset and invokes this trusted inspector with argument arrays. Do not
execute anything from the contributor repository or package. The inspector
outputs a safe JSON package descriptor. It does not fetch listing images.
The index workflow builds the inspector from trusted
base code for untrusted registry PR validation; no deployment credential is
available to this validation path.

Validate registry ID against manifest ID, release tag against manifest version,
and declared repository URL (when present) against the listed repository. All
package metadata, permission summaries, and hashes come from the validated
archive, not a separate branch manifest. Preview URLs and order come from the
registry entry. Strict PR validation rejects
invalid added/changed canvas entries. Scheduled publication reports invalid
entries as skipped/degraded according to the existing builder behavior and
continues with valid entries; it never publishes an invalid canvas descriptor.

Validate `previews` on canvas registry pointers and copy the list directly
into the generated catalog. Keep image hosting under the listing maintainer's
control. Do not add media output to the Pages workflow. Maintainers can revise
previews without rebuilding the package. The proposed complete pointer schema is
[`registry-entry.schema.json`](../../../plans/canvas-marketplace/registry-entry.schema.json).

## Catalog projection

Keep the current index document and add optional per-entry `kind`, `previews`
(`url`, `alt`), `permissions`, `license`, and `source_mode`.
For canvas entries, `package_sha256` and these presentation fields are required.
Legacy entries with missing kind decode as native plugins. Unknown kinds or
incomplete canvas entries cannot be installed. Add a `kind` query without
removing category filtering. Reserve `canvas` as the displayed category for
validated canvas entries; relabeling a native package cannot change its kind.

The catalog endpoint accepts an optional authorized `workspace_id` when the
requested kind is canvas. Source fetching/cache remains shared, but instance
annotations are computed per request after authorization. Query receipts joined
to live canvas metadata for that workspace only. Return instance count and
authorized routes; never globally join static canvases by package ID into
`InstalledForMarketplace`. Label existing copies with Open (or a copy picker)
and offer Add another copy. Preserve native installed/update behavior.

Catalog images use validated HTTPS URLs from the configured index, without a
referrer, in contained raster image elements. Preserve previews for registry
review, but label them as listing previews, separate from inspected package
permissions. Upload/direct-link review omits the gallery completely. Do not
find a registry by package ID to attach potentially unrelated images. Failed
remote images show a fallback and leave valid installation available.

The same shared gallery serves native plugin details. Plugins without images
keep their existing row and direct install action. A plugin with images offers
View details alongside its current install action. This is covered by the
Plugins-owned preview requirement; the canvas design does not redefine native
plugin permissions or installation.

## Authorization and feature gates

Workspace owners can export their canvases and install static canvases into
their own workspaces. Apply `task.Service.AuthorizeWorkspaceAccess` at new
service boundaries and again on confirmation/download. Installation never
inherits grants from the author or another local instance. Reuse synthetic
identity behavior when auth is disabled.

Keep native plugin install and marketplace-source mutation admin-only. Split
the frontend's current `useIsAdmin` action gate so it does not hide workspace
canvas actions. A details preview grants no permission. An install action
explicitly approves the entire displayed declaration; later permission growth
uses the existing release approval flow.

All new canvas entry points require the existing `features.canvases` gate.
Settings > Plugins and its catalog also retain the current plugin-page gate.
When canvases are off, the catalog filters out canvas entries before returning
results, no preparation/receipt lookup is performed, and the frontend mounts no
canvas hooks or routes. Share from an existing canvas uses the canvas gate and
does not require native-plugin admin authority. No new flag or profile change.

## UI composition

Settings > Plugins keeps Installed and Browse and adds a Canvases tab. It has
an explicit workspace selector, search/sort, and Install canvas action with
upload/direct-link inputs. Cards show cover, name, author, version, and concise
description. Card activation opens details, never an immediate installation.
Use the existing settings route with a query-selected canvas detail, retaining
search/workspace when Back closes it. Add Browse shared canvases on workspace
Canvases settings, carrying that workspace into the tab.

Details shows title, author, version, description, license, repository link,
gallery, and grouped requested permissions. The selected image is contained,
not cropped, with next/previous and thumbnail selection. Enlarging an image
stays within the current details surface. Review & install downloads/inspects
the package, then replaces the body with authoritative package review. The
primary confirmation names the target workspace. Success offers Open canvas.

Share sits in host-owned canvas actions and workspace canvas rows. It shows
active release, identity/version/author/license fields, source mode, and How
to share. Prepare downloads validates the inputs and reveals file inventory,
final sizes, and the two download buttons. There is no screenshot field.
Form edits invalidate prepared output. Validation failures retain inputs.
Read-only inventory and the private-content reminder remain visible beside
download actions.

The helper is an information dialog, not an automation wizard. It explains:
download source and bundle; create a repository on the chosen host; upload the
source; publish/upload the bundle; share the direct downloadable file URL or
send the file; submit the registry pointer with at least one preview image URL for public
GitHub-hosted listings. Explain how custom registries use the same `previews`
field and how plugins may also supply optional previews.
Provide a copyable `plugins.yaml` entry only once repository owner/name is
known, with visible placeholders otherwise. Never put credentials in commands.
GitLab/Bitbucket instructions describe manual hosting for direct links; their
repositories are not accepted by the GitHub-only official registry in v1.

## Mobile contract

- Entry points are the Plugins settings page and existing workspace/canvas
  host actions. The closest shipped domain exemplar is
  `components/settings/canvas-host-route.tsx` with its host action drawer;
  `components/kanban-with-preview.tsx` supplies direct detail navigation.
- On a phone, show one-column cards and a focused full-height details surface.
  Keep Back/title fixed, one scrolling body, and the install action in a
  safe-area-aware footer. Do not mount a desktop split view under the phone UI.
- Share uses a full-height form because metadata, inventory, and validation need
  depth. The short How to share action opens an inset bottom drawer with
  bounded internal scrolling. Returning preserves the share draft.
- Use `useResponsiveBreakpoint`, shared Dialog/Drawer primitives, and existing
  28px desktop / at-least-44px touch control sizing. No hover-only instructions,
  swipe-only gallery, or download-only desktop path.
- Domain hooks own input state, review generation, selected workspace, image
  selection, and requests across both presentations. Ignore late responses
  after input, workspace, detail, or preparation identity changes.
- Use dynamic viewport height, keyboard/safe-area clearance, focus return, and
  one scroll owner per active surface. All interface copy uses i18next; author
  text remains data. Show localized status/error regions for asynchronous work.

## Failure handling and observability

An invalid export leaves the active release untouched. An invalid import creates
no instance. On DB failure, compensate unreferenced artifact writes; on lost
confirmation response, replay the receipt. After restart, pending previews expire
visibly while installed canvases remain usable. Workspace deletion/canvas
removal invalidates preparation access and triggers normal artifact cleanup.
No failure silently falls back to native install, mutable latest code, or a
different release. Catalog outages preserve direct-file/link entry points.

Log operation, actor/workspace/canvas IDs where authorized, result code, byte
counts, digest, and duration. Omit source, screenshot content, titles, tokens,
and URL queries. Use content-free diagnostics; add no usage telemetry service.

## Verification and delivery

The [implementation plan](../../../plans/canvas-marketplace/plan.md) maps each
criterion to proposed unit, integration, registry, and desktop/mobile tests.
Tests must prove export-to-import round trips, project-source recovery after
executor cleanup, digest-bound review, concurrent retry receipts, isolation,
registry URL validation, screenshot-free export/import, plugin galleries,
gate-off behavior, and native installer compatibility.

This package extends completed `plugin-backed-canvases` work and the shipped
parts of `plugin-backed-canvases-ux-follow-up`. It does not change their recorded
test results or mark the outstanding external ACP evaluation complete. New
source-guide changes must preserve that package's one-core-read contract.
