---
created: 2026-11-19
updated: 2026-11-19
status: complete
requirements:
  - REQ-PLUGINS-MARKETPLACE-001
system_design:
  - ../../specs/plugins/system-design/marketplace.md
legacy_specs: []
---

# Implementation Plan: Merge marketplace categories

## Overview

The official marketplace index combines categories from a plugin's tagged
release manifest and its curated registry entry. The manifest describes the
plugin. The registry can add catalog labels. This lets maintainers add a useful
catalog category without requiring a plugin maintainer to publish a new
release.

## Work order

- [x] [Task 01: Merge curated and manifest categories](task-01-merge-curated-categories.md), wave 1, done.

## Verification results

- The regression test failed before the implementation because the index
  contained only the manifest category.
- `node --test plugin-registry/*.test.mjs` — 53 passed.
- `node --test scripts/validate-public-docs.test.mjs` — 63 passed.
- `node scripts/validate-public-docs.mjs` — 55 published docs pages validated.
- `python3 scripts/list-docs.py validate` — 449 decisions and 1,984
  specifications validated.
- `python3 scripts/lint-spec-files.py --all` — passed.
- `git diff --check` — passed.
