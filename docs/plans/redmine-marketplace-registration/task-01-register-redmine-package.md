---
id: "01-register-redmine-package"
title: "Register the released Redmine package"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-MARKETPLACE-001
acceptance_criteria:
  - AC-PLUGINS-MARKETPLACE-001.3
  - AC-PLUGINS-MARKETPLACE-001.6
system_design:
  - ../../specs/plugins/system-design/marketplace.md
---

# Task 01: Register the released Redmine package

## Scope

Add the released Redmine plugin repository to the canonical marketplace source.
The entry must match the manifest identity and retain the index builder as the
source of package, compatibility, and integrity metadata.

## Acceptance

- The official catalog discovers one Redmine listing with release metadata from
  `yattdev/kandev-plugin-redmine`.
- Browse metadata and the one-click install target resolve to the verified
  v0.3.2 package and its declared Kandev compatibility floor.
- The change does not modify unrelated registry entries, release artifacts, or
  marketplace behavior.

## Verification

```sh
node --test plugin-registry/build-index.test.mjs
node plugin-registry/build-index.mjs
```

Inspect the generated record for `kandev-plugin-redmine`, then verify the
downloaded package digest and internal `checksums.txt` against the release.

## Results

The registry entry resolves the v0.3.2 Redmine package with
`min_kandev_version: 0.94.0`. The downloaded archive digest matches GitHub
release metadata, and all internal package checksums pass.
