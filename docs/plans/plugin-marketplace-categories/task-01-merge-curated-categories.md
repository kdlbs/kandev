---
id: "01-merge-curated-categories"
title: "Merge curated and manifest categories"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-MARKETPLACE-001
acceptance_criteria:
  - AC-PLUGINS-MARKETPLACE-001.11
system_design:
  - ../../specs/plugins/system-design/marketplace.md
---

# Task 01: Merge curated and manifest categories

## Summary

Combine categories from the tagged release manifest and the curated registry
entry when the registry builds the official marketplace index.

## In scope

- Category composition in `plugin-registry/build-index.mjs`.
- A regression test in `plugin-registry/build-index.test.mjs`.
- Marketplace contract and documentation updates.

## Out of scope

- Changes to the marketplace index schema or frontend category filters.
- Changes to external plugin releases or manifests.
- Category renaming, case folding, or validation policy changes.

## Acceptance

1. The index includes every category from the manifest and curated registry
   entry.
2. Duplicate categories appear once. Manifest order is preserved, then new
   registry categories follow in registry order.
3. A regression test covers a manifest category plus an additional registry
   category and a duplicate category.

## Verification

```bash
node --test plugin-registry/*.test.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `plugin-registry/build-index.mjs`
- `plugin-registry/build-index.test.mjs`
- `plugin-registry/README.md`
- `docs/public/plugins-marketplace.md`
- Marketplace requirements and system design.
- This plan and work order.

## Results

Status: done.

The regression test reproduced the previous behavior. It expected
`["connector", "integrations"]` but the builder returned only `["connector"]`.
The builder now returns unique categories from both sources, with manifest
categories first and registry categories after them.

`node --test plugin-registry/*.test.mjs` — 53 passed. Public documentation
validation passed with 63 tests and 55 published pages. The specification
catalog and lint passed, and `git diff --check` passed.
