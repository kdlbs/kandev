---
spec: docs/specs/plugins/requirements/marketplace.md
created: 2026-08-22
status: building
---

# Implementation Plan: Correct plugin marketplace attribution

## Overview

Correct the release manifests that feed the marketplace's visible author and
repository metadata. The Kandev host registry pointer remains unchanged: the
Bitbucket entry is already a first-party `kdlbs` repository, and the YouTrack
entry must continue to point to the external contributor's repository. The
changes land in the two plugin repositories, then the existing release
workflows and registry builder verify that the catalog displays the corrected
identities.

## Repository boundaries

- `kdlbs/kandev-plugin-bitbucket` owns the first-party Bitbucket manifest.
- `ahmedbally/kandev-plugin-youtrack` owns the community YouTrack manifest.
- `kdlbs/kandev` owns only the marketplace contract and verification. No new
  author override is added to `plugin-registry/plugins.yaml`; release manifest
  metadata remains authoritative.

## Plugin repository changes

### First-party Bitbucket attribution

In `kdlbs/kandev-plugin-bitbucket/manifest.yaml`, change the author from
`kdlbs` to `kandev` and preserve the existing repository URL. Produce the next
release through that repository's existing release workflow so the latest
release tag and tarball contain the corrected manifest.

### Community YouTrack attribution

In `ahmedbally/kandev-plugin-youtrack/manifest.yaml`, change the author to the
contributor handle `ahmedbally` and change `repo_url` to
`https://github.com/ahmedbally/kandev-plugin-youtrack`. Produce a new release
through the repository's existing release workflow so the registry's latest
release lookup sees the corrected manifest.

## Tests

- **What:** The registry builder preserves a release manifest's declared author
  and emits the repository selected by the registry pointer.
  **File:** `plugin-registry/build-index.test.mjs` (existing coverage) and the
  generated `plugin-registry/index.json` after release publication.
  **How:** Run the focused Node test suite, then rebuild the live catalog and
  inspect the Bitbucket and YouTrack records for `author` and `repo_url`.
- **What:** The first-party plugin still packages successfully after its
  manifest metadata change.
  **File:** `kdlbs/kandev-plugin-bitbucket` repository tests and package gate.
  **How:** Run `make test` and `make package-host verify-package-host`.
- **What:** The community plugin still builds and passes its existing checks
  after its manifest metadata change.
  **File:** `ahmedbally/kandev-plugin-youtrack` repository tests.
  **How:** Run `go test ./...`, `go vet ./...`, and the release workflow's
  package step when publishing the new release.
- **What:** The unchanged marketplace card renders the corrected author and
  repository link values.
  **File:** `apps/web/components/settings/plugins/marketplace-entry-row.test.tsx`.
  **How:** Run the existing focused component test after the catalog records
  are rebuilt; no component or layout change is expected.

## E2E and mobile parity

The repair changes release metadata only. It does not change the marketplace
card's markup, interaction, navigation, responsive composition, or touch
targets. The existing component test plus the post-release catalog inspection
are the appropriate evidence; a new desktop or mobile Playwright flow would
duplicate unchanged UI behavior.

## Verification Results

- 2026-08-22: Bitbucket branch `fix/marketplace-author` changes only
  `manifest.yaml`, declaring `author: "kandev"` while preserving the official
  repository URL. `make test`, `make package-host verify-package-host`,
  `make fmt`, and `make vet` pass. The package gate produced
  `kandev-plugin-bitbucket-0.2.1.tar.gz` with the corrected manifest.
- 2026-08-22: YouTrack fork branch
  `fix/correct-marketplace-attribution` changes `manifest.yaml` to version
  `0.1.1`, author `ahmedbally`, and the external repository URL. README build
  references were updated for the new release version. `go test ./...`
  passes all 25 tests and `go vet ./...` passes. The temporary local SDK
  replacement and generated `go.sum` used for verification were removed.
- 2026-08-22: Kandev verification passes with 8 registry-builder tests and 3
  marketplace-entry-row tests. `pnpm install --frozen-lockfile` also passes.
- Live catalog records remain pending the two release publications. The
  existing registry pointer file was not changed; once the releases are
  published, rebuild the catalog and verify both author and repository fields.

## Implementation Waves And Parallel Candidates

Wave 1 contains two disjoint plugin-repository changes. They are marked as
parallel-safe for planning purposes, but the primary session executes them
sequentially unless the user explicitly authorizes delegation.

- [ ] [task-01-bitbucket-attribution](task-01-bitbucket-attribution.md) -
  in progress; local fix complete, release publication follows merge
- [ ] [task-02-youtrack-attribution](task-02-youtrack-attribution.md) -
  in progress; local fix complete, upstream maintainer release publication
  follows merge

Wave 2 verifies the published releases and regenerated catalog:

- [ ] [task-03-registry-verification](task-03-registry-verification.md) -
  pending; blocked until the two corrected releases are published

## Open Questions

- The YouTrack repository is controlled by an external contributor. The source
  change can be prepared in an attached branch, but merging it and publishing
  its release remain subject to that repository's maintainer access.
