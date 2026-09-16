---
id: "curated-plugin-release-publication"
title: "Publish curated plugin releases promptly"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-MARKETPLACE-001
  - REQ-PLUGINS-MARKETPLACE-002
acceptance_criteria:
  - AC-PLUGINS-MARKETPLACE-002.1
  - AC-PLUGINS-MARKETPLACE-002.2
  - AC-PLUGINS-MARKETPLACE-002.3
  - AC-PLUGINS-MARKETPLACE-002.4
system_design:
  - "../../specs/plugins/system-design/marketplace.md"
---

# Task 01: Publish curated releases promptly

## Goal

A valid release from an already-curated plugin repository becomes discoverable
through the official marketplace within the documented 10-minute target under
normal GitHub Actions scheduling, without a Kandev source commit or a manual
registry dispatch, while curation authority, package integrity validation, and
the 06:00 UTC recovery rebuild are preserved.

## Scope

- Central release poll workflow
  (`.github/workflows/plugin-registry-release-poll.yml`) that enumerates only
  the checked-out `plugin-registry/plugins.yaml` allowlist every five minutes
  at an off-boundary minute offset and triggers the shared index rebuild
  through `workflow_call` when a newer curated release is detected.
- Release detection (`plugin-registry/check-releases.mjs`) with unit coverage
  (`check-releases.test.mjs`), accepting no repository name or release payload
  from plugin repositories.
- Index builder hardening (`plugin-registry/build-index.mjs`): per-record
  validation of the exact `<id>-<version>.tar.gz` asset, checksum and archive
  manifest identity checks, retention of the last known-good record on a bad
  release, and `package_sha256` publication.
- Package verifier command (`apps/backend/cmd/plugin-package-verify`) reusing
  the `pkgtar` checksum/manifest safety gate, plus fixture manifest versioning
  support in `cmd/plugin-pack` for deterministic tests.
- Shared `plugin-registry-pages` concurrency group across push, manual, daily,
  release-poll, and star-refresh deployments.
- Marketplace update ordering semantics for prerelease and opaque versions in
  `apps/backend/internal/plugins/manifest` and the web plugin row update
  actions.

## Out of scope

- Plugin repository releases, secrets, or workflow changes (Kandev credentials
  never leave `kdlbs/kandev`).
- Host-side digest enforcement of `index.json.package_sha256`; installs
  continue to enforce the package's internal checksums.
- A hosted webhook receiver, GitHub App, or hard real-time SLA.

## Acceptance mapping

- `AC-PLUGINS-MARKETPLACE-002.1` - five-minute allowlist-only poll, documented
  10-minute target, provider caveat, daily fallback retained.
- `AC-PLUGINS-MARKETPLACE-002.2` - exact asset, checksum/manifest gate, and
  manifest identity equality before a record is published.
- `AC-PLUGINS-MARKETPLACE-002.3` - bad release retains the prior validated
  record; provider failure retains the deployed Pages site; visible workflow
  evidence on fallback.
- `AC-PLUGINS-MARKETPLACE-002.4` - one serialized coalescing concurrency group
  shared by every official deployment path; the release signal carries no
  repository selector or release payload.

## Verification

- `node --test plugin-registry/*.test.mjs` (build-index, check-releases, and
  workflow contract suites).
- `go test ./cmd/plugin-package-verify/ ./internal/plugins/pkgtar/ ./internal/plugins/manifest/`
  in `apps/backend`.
- Web plugin settings unit suites and `pnpm run i18n:check` in `apps/web`.
- One end-to-end regression chain: served v1 catalog plus installed v1 package,
  curated v2 release, central poll visibility, rebuilt index advertising v2,
  host **Check for updates** showing the update, and a supported update install.

## Dependencies

None.

## Final verification

- Registry Node suites: 43 passed.
- Package verifier, package archive, manifest, and platform-only pack Go
  suites: passed.
- E2E fixture packaging: passed with both verifier and fixture-identity
  artifacts present.
- The focused production-build Chromium release publication and supported
  update-install flow passed.
- The focused Pixel 5 update layout, containment, and touch-target flow passed
  with retries disabled. The first mobile backend-start attempt overlapped a
  separate Playwright runner on deterministic ports; the isolated retry passed.
- `git diff --check`: passed.
