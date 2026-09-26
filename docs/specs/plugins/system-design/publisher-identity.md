---
status: draft
system: plugins
created: 2026-09-18
owners:
  - kandev
requirements:
  - REQ-PLUGINS-PUBLISHER-001
  - REQ-PLUGINS-PUBLISHER-002
  - REQ-PLUGINS-PUBLISHER-003
  - REQ-PLUGINS-PUBLISHER-004
---

# Plugin publisher identity system design

## Ownership and current evidence

Plugins owns publisher evidence, catalog projection, and native installation.
Canvases consumes this evidence through its existing preparation and receipt lifecycle.
The [marketplace design](marketplace.md) retains discovery and source-management behavior.
The [canvas distribution design](../../canvases/system-design/marketplace-sharing.md) retains workspace authorization and consent.

The initial investigation found that `plugin-registry/build-index.mjs` preferred
manifest author over repository owner, its `buildEntry` test accepted `acme/foo`
with author `kandev`, and the marketplace displayed that value as unqualified
attribution. Native package checksums and optional signatures established package
integrity but did not establish publisher identity. Direct installation and
automatic updates also had no catalog identity.

The implementation below closes those gaps. Registry entries now carry archive-
bound ownership evidence, catalog installs resolve a fresh selector through the
canonical source, native records persist provenance, and automatic updates use
the same evidence-bound path. Direct, custom-source, and legacy records remain
usable and are projected as unverified.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-PLUGINS-PUBLISHER-001 | Trust boundary; Registry production; Catalog projection |
| REQ-PLUGINS-PUBLISHER-002 | Native installation; Update continuity; Canvas integration; Migration |
| REQ-PLUGINS-PUBLISHER-003 | Presentation; Failure behavior; Security limits |
| REQ-PLUGINS-PUBLISHER-004 | Existing-version verification |

## Trust boundary

The trust root is the canonical HTTPS catalog at `marketplace.OfficialSourceURL`.
The backend grants evidence only to the built-in source fetched from that exact URL with normal TLS validation and no redirects.
Source IDs, `builtin`, document headers, source labels, and self-declared repository URLs are insufficient individually.
`KANDEV_PLUGIN_MARKETPLACE_URL` keeps its discovery function but grants no trust when it changes the URL.
Adding a custom source never adds a trust root, including a mirror of the official document.

The registry workflow establishes repository ownership through GitHub metadata and binds it to archive bytes.
The backend trusts that statement only through the canonical catalog transport.
This is registry-attested provenance, not a cryptographic signature from each publisher.
The UI explanation names this limit: the official registry verified the repository owner and package identity, not code safety.

The [publisher trust decision](../../../decisions/2026-09-18-plugin-publisher-trust.md) records this boundary and its alternatives.

## Registry production

Keep index schema version 1 with optional additive `publisher` metadata.
New official entries require complete evidence. Old index readers can ignore the new fields.
Keep `author` as declared credit, without fallback to repository owner.

Add an optional curation field `official: true` to `plugin-registry/plugins.yaml` and its schema/parser.
Only reviewed entries with this field and verified `kdlbs` ownership qualify for official designation.
Repository prefix, package ID, and author text never establish this designation.
Repository metadata must include stable numeric repository and owner IDs, owner login, and canonical full name.
Do not fall back to pointer text when the ownership lookup fails.
Require the returned owner/full name to match the pointer, case-insensitively. Repository transfers require a reviewed pointer update.

For each native plugin and canvas release:

1. Resolve the curated repository and latest GitHub release.
2. Select the exact `<id>-<version>.tar.gz` release asset. Remove native first-tarball fallback for official publication.
3. Download that asset with a bounded stream and compute its archive SHA-256.
4. Inspect the downloaded archive without executing package code.
5. Compare manifest ID/version to the pointer/release and any declared `repo_url` to the canonical repository.
6. Apply reserved-author checks and emit publisher evidence with the validated archive digest.

Native inspection reuses `pkgtar` parsing, archive bounds, and checksum validation through a new non-extracting validation entry point.
It validates every declared executable path without requiring the CI host's platform.
Add a proposed `cmd/plugin-package` inspector alongside the existing `cmd/canvas-package` inspector.
Do not use `pkgtar.Inspect` alone: it reads a manifest without complete archive verification.
Native archives retain the installer's 100 MiB compressed ceiling and `pkgtar` expanded/file limits.
Canvas inspection retains its existing smaller bounds and `ValidateDistributionPackage`.
Use a 60-second native download bound and 30-second inspector timeout, with cleanup on every outcome.

The reserved author comparison uses Unicode NFKC normalization, case folding, removal of whitespace and default-ignorable characters, and exact comparison to `kandev` or `kdlbs`.
Only an official entry can use either reserved identity. Empty native author credit is permitted.
Canvas metadata retains its existing required-author rule.
This rule catches capitalization and invisible-character variants. It does not claim comprehensive homoglyph detection.
Archive metadata is authoritative even when the tag's manifest differs.

PR validation rejects unauthorized claims and invalid changed entries.
Scheduled publication omits invalid entries, reports the reason, and publishes valid entries without restoring a stale rejected descriptor.
A star lookup failure retains the previous star count when one exists; it yields null only when no prior count is available. An ownership lookup failure cannot yield a verified publisher.
Registry validation must use trusted inspector/workflow code when it processes untrusted registry changes.

## Catalog projection and contracts

Proposed index evidence:

```json
{
  "publisher": {
    "schema_version": 1,
    "repository_id": "123",
    "owner_id": "456",
    "login": "acme",
    "repository": "acme/board",
    "official": false
  },
  "package_sha256": "<64 lowercase hex characters>"
}
```

IDs are decimal strings, avoiding JavaScript precision loss.
Repository, login, IDs, package kind, ID, version, and digest must form a complete valid entry.
The registry derives all evidence fields. Packages cannot supply them.

Add a shared internal `plugins/provenance` package for typed evidence and projection validation.
The proposed backend-owned `publisher_identity` projection has `status: verified|unverified`, optional verified owner/repository fields, `official`, and `verified_at`.
Keep catalog input `publisher` separate from output `publisher_identity`.
Custom JSON cannot deserialize into a trusted output field through embedded DTOs.
`CatalogEntry` builds this projection after checking transport provenance and entry shape.
Malformed or incomplete publisher metadata yields an unverified listing with a source diagnostic.
An invalid package identity or malformed nonempty digest rejects the entry.
Legacy native listings without a digest remain installable but unverified.
Canvas listings retain their existing mandatory digest rule.

The frontend never derives verification from author, source name, `signed`, or URL text.
For unverified listings, publisher identity stays empty. Repository links and source labels remain informational data.
The source record supplies source ID, name, and URL, not the document's self-description.
Keep source priority and deduplication behavior unchanged, but resolve installation from the explicitly selected source.

## Native installation

Extend `InstallRequest` on `POST /api/plugins/install` with a mutually exclusive `catalog` selector:

```json
{
  "catalog": {
    "source_id": "official",
    "package_id": "example",
    "expected_version": "1.2.3",
    "expected_sha256": "<digest shown in the listing>"
  }
}
```

Keep `{"url":"..."}` and multipart upload compatible and unverified.
Reject mixed URL/catalog bodies and request-supplied publisher evidence.
For legacy native entries, the expected digest can be absent only when the resolved entry also has no digest.

Add `Service.InstallFromCatalog` and a native counterpart to the existing canvas catalog resolver.
Resolve the enabled source and exact entry on the server with a fresh fetch, bypassing the catalog TTL.
Compare expected version/digest and package kind before downloading.
The request values constrain a selection. They never establish authority.
Capture an immutable resolution containing the canonical URL, identity tuple, and backend-created evidence.

Download once into bounded temporary storage. Compute SHA-256 over those bytes and verify any catalog digest before extraction.
Validate manifest ID/version against the captured entry before stopping an old runtime or saving a record.
Only the captured resolution can supply the new record's provenance.
Preserve the existing extraction, lifecycle locks, approval review, rollback, and activation path.
Recheck installation identity and automatic-update eligibility under the lifecycle lock after the download.

Add optional host-owned provenance to `store.Record`, separate from its embedded manifest.
Store origin kind, source ID/canonical URL, archive digest, package ID/version, repository and owner IDs, login, official designation, and verification timestamp.
`FSStore` persists it with the same record write as the installed version.
Validate the tuple when reading. Missing, malformed, or mismatched evidence projects unverified.
No separate publisher database or background verifier is required.

## Existing-version verification

Add the admin-only `POST /api/plugins/:id/verify-publisher` endpoint and proposed `Service.VerifyInstalledPublisher` method.
The body contains `expected_installation_id` and `expected_version`, taken from the installed record.
The server obtains the package ID and install path from that record, not from caller-supplied file paths or URLs.
Missing records return 404. Stale expected identity returns 409 without a download.

This explicit action applies to unverified native installations, including legacy, uploaded, URL-installed, and filesystem-sideloaded records.
It does not run automatically or add a canvas verification endpoint.
Resolve the exact installed ID/version through a fresh fetch of the enabled canonical official source.
Do not substitute the latest version or infer evidence from a release tag or repository URL.
If the catalog no longer lists that version, return `publisher_evidence_unavailable` (409).
This package adds no historical-release index. A plugin whose final release remains listed can verify that version indefinitely.

Download the reference archive under the existing bounds, validate its catalog digest, and run the non-extracting package validator.
Generate the expected file inventory from the validated archive, never from the installed `checksums.txt` alone.
Compare all regular files under the exact installed version directory against the expected relative paths, sizes, and content hashes.
Include the manifest, native binaries, UI files, checksum file, and optional signature file.
Reject missing or extra files, symlinks, special files, traversal, and unreadable files.
Ignore timestamps and empty directories. Apply the installer's executable-mode normalization where the platform supports file modes.
Do not exclude a directory merely because its name is `data` inside the version directory.
The runtime's separate `<pluginsDir>/<id>/data` directory, configured through `KANDEV_PLUGIN_DATA_DIR`, lies outside this comparison.

After downloading, acquire the existing lifecycle lock and reread the installed record.
Require its installation ID, version, path, and prior provenance to match the captured identity.
If the plugin is running, stop it before comparing files and saving evidence, then restart it after the operation. This closes the process-to-filesystem race while preserving its active lifecycle state after success.
Reject observed file replacement or mutation during comparison using file identity and before/after metadata checks.
The check attests the on-disk package at comparison time, not the memory of a running process.
Arbitrary concurrent host-filesystem attackers remain outside the existing host trust boundary.

Save only provenance on the current record through `FSStore`; publish the new in-memory projection only after persistence succeeds.
Retain status, counters, installation identity, configuration, auto-update preference, permissions, version, and original origin fields.
Do not invoke installation, extraction, activation, signature promotion, or capability approval.
Record `verification_method: installed_files` and the reference archive digest separately from installation origin.
Ordinary catalog installs use `verification_method: archive_download`.
Both methods use the same publisher evidence and update-continuity rules.
For migrated records with no known origin, keep `unknown`; do not invent an installation source.

File mismatch returns `installed_package_mismatch` (409). Concurrent lifecycle changes return `installed_package_changed` (409).
An unreadable package returns `installed_package_unreadable` (409), and evidence persistence failure returns `publisher_verification_failed` (500).
Catalog transport failures retain `catalog_unavailable` (503).
Each failure leaves package files, runtime, and prior attribution unchanged and removes temporary reference files.
Repeated requests for an already verified matching installation return its current projection without lifecycle changes.

In native plugin details, show `Verify publisher` beside `Unverified publisher` for administrators.
Disable the action during a request and announce progress through a localized status region.
Show the resulting badge only from the successful backend response, and preserve it after page reload.
An unavailable version explains that no trusted package exists for this installed version.
A mismatch explains that installed files differ and that the plugin was not changed.
Both states retain a retry action. Read-only members see attribution but no verification action.
After success, retain `Source: Uploaded file` or the actual original source and add `Matched against: Kandev Official` in details.
Phone details use the same inline flow with a separate touch-sized action row.

## Update continuity

Manual catalog updates and `applyAutoUpdates` call `InstallFromCatalog`.
They cannot copy old evidence or use `InstallFromURL` as an error fallback.
For a verified installed release, an automatic update requires the same source, repository ID, owner ID, and official designation.
A repository/owner transfer, source substitution, or downgrade to unverified fails with `publisher_changed`.
An explicit manual catalog install can replace the publisher and stores the new evidence.
Its button must describe the candidate publisher and show the publisher-change notice before the action.
Direct URL/upload replacement remains an explicit unverified install.

Legacy unverified installs retain existing opt-in update eligibility.
A successful catalog update can establish verified evidence for the new bytes.
Old installed-version badges always use stored evidence, even when a different publisher appears in the current listing.

## Canvas integration

Use the same catalog evidence resolver in `backendapp/canvas_distribution_routes.go` and the distribution service.
Carry backend-created evidence through `canvas.Preparation`, `InstallReview`, and `InstallReceipt`.
Upload and URL routes cannot supply it through `SourceID` or `RepositoryURL` fields.

Keep archive SHA-256 distinct from `webapp.Package.Digest`, which identifies normalized package content.
Bind both to the preparation, then bind provenance to the created release ID in the installation transaction.
Extend `canvas/repository.go` receipt schema upgrades with nullable provenance JSON and release ID.
`commitCanvasInstallation` in `canvas/authoring.go` writes this association in the existing transaction.
Old receipts default to unverified. Commit replay returns the same stored evidence.

The active release receives a verified projection only when its ID and content digest match that receipt.
A locally edited release has no receipt evidence and projects unverified.
Selecting the unchanged original imported release can restore its historical attribution.
Exports contain declared author credit but no host-owned provenance.
Reimporting an exported file stays unverified, even if its bytes match a previous release.
Keep permission review, workspace authorization, receipt replay, and artifact cleanup unchanged.

## Presentation

Use one proposed `PluginPublisherIdentity` component and typed view model across marketplace and installed views.
Place it in `components/settings/plugins/plugin-publisher-identity.tsx` with a matching `.test.tsx` file.
Show publisher first, then its verification label, source, and separately labeled declared author credit.
Official releases show `Publisher: kdlbs` and `Official Kandev`.
Community verified releases show the actual login and `Verified publisher`.
Other releases show `Unverified publisher` and `Declared author: <value>` when present.
Package integrity and `signed` remain separate facts.
Replace the canvas review's ambiguous `Package verified` wording with a package-integrity statement.

Affected surfaces include `marketplace-entry-row`, `plugin-row`, `plugin-manifest-card`, canvas catalog cards/details, and `canvas-install-dialog`.
Installed canvas attribution follows the active-release projection where those surfaces expose package origin.
The shared installed/catalog hooks pass the full catalog selector instead of only `package_url`.
Preserve retry controls, loading indicators, installed settings links, and member read-only behavior.

Phone attribution is inline in the existing settings card flow, with wrapped rows and a separate action row.
The nearby exemplar is `mobile-plugin-updates.spec.ts` and its installed settings row.
Canvas details retain the focused route and review surface from the canvas distribution design.
No new overlay or hover disclosure is necessary for this short content.
The existing page/detail body remains the single scroll owner.
Keep source and repository links touch-accessible, at least 44px on coarse pointers, with ordinary desktop density elsewhere.
Trust labels remain fully visible. Long values wrap and never displace those labels.
All labels, notices, errors, and accessible names use the existing five-language localization workflow.

## Failure behavior and observability

Native selector failures use stable error codes: `catalog_unavailable` (503), `catalog_selection_stale` (409), `publisher_changed` (409), and `package_identity_mismatch` (400).
Keep existing install error responses compatible and add an optional machine-readable code.
Canvas routes map equivalent failures into their existing distribution error envelope.
A failed fresh fetch never falls back to the URL installer or cached evidence.
An in-progress resolution can finish against its captured digest. A later catalog publication cannot change that target.
Installed historical evidence survives outages and source removal. It does not represent a live revocation check.

Log source ID, package ID/version, result code, and expected/actual digest for relevant failures.
Do not log raw manifest author strings, URL queries, download credentials, or package content.
Use existing source diagnostics and updater failure reporting. No new telemetry service is necessary.

## Migration and delivery

Deploy additive readers before the enriched official index when releases can be ordered independently.
Old indexes and installed records initially remain unverified. Never backfill verification by ID or URL alone.
An explicit existing-version comparison can establish evidence without a new release or reinstall.
Enable enriched registry publishing only after host enforcement and UI attribution are ready.
Do not modify published plugin repositories as part of this package.
List existing invalid releases as release follow-up requirements instead of rewriting their authors automatically.
This is a correction to trust handling, with no new runtime feature flag or profile changes.

The [implementation plan](../../../plans/plugin-publisher-identity/plan.md) owns delivery order and test evidence.
Historical canvas and preview work-order results remain historical. This plan owns the new publisher checks.

## Security limits

The host OS, local record store, official registry workflow, GitHub repository ownership, and canonical HTTPS endpoint remain trusted.
This package does not defend against a compromised Kandev registry or host administrator.
Mutable release assets are safe only while their bytes match the captured digest.
Source loss or later delisting does not stop installed code or erase historical evidence.
Verification never changes capability permissions, workspace access, canvas consent, or `Signed`.
