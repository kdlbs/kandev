---
created: 2026-09-15
status: done
requirements:
  - REQ-PLUGINS-MARKETPLACE-001
system_design:
  - ../../specs/plugins/system-design/marketplace.md
---

# Implementation plan: Register Redmine in the plugin marketplace

## Overview

Register the already released Redmine connector in the official Kandev plugin
marketplace. The registry remains a minimal curated pointer list; release
metadata, package URL, compatibility, and install integrity are resolved from
the published plugin package by the existing index builder.

## Scope

- Add one unique `kandev-plugin-redmine` entry that points to
  `yattdev/kandev-plugin-redmine`.
- Verify the published v0.3.2 release, manifest identity, compatibility floor,
  tarball digest, and internal package checksums.
- Run the existing registry test suite and live index build, then inspect the
  generated Redmine catalog record.

## Out of scope

- Publishing, retagging, or altering the Redmine plugin release.
- Changing marketplace schema, index-builder behavior, install behavior, or
  marketplace UI.
- Updating public marketplace guidance, which already documents this curation
  workflow.

## Delivery

- [x] [Task 01: Register the released Redmine package](task-01-register-redmine-package.md)

## Verification results

- The annotated `v0.3.2` tag resolves to
  `b843ce5345e8a11ade7f0d4d33386c746465a049` and its manifest declares
  `kandev-plugin-redmine` with `min_kandev_version: 0.94.0`.
- The downloaded release archive SHA-256 matches GitHub release metadata, and
  its internal `checksums.txt` validates every packaged file.
- `node --test plugin-registry/build-index.test.mjs` and
  `node plugin-registry/build-index.mjs` pass; the generated catalog contains
  exactly one Redmine record for v0.3.2.
