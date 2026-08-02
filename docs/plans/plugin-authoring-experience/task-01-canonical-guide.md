---
id: "01-canonical-guide"
title: "Canonical plugin guide"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/plugins/authoring-experience.md"
---

# Task 01: Canonical plugin guide

## Acceptance

1. `docs/public/plugins-authoring.md` contains the lifecycle, layouts, plugin
   shapes, security/capabilities, storage table, complete current matrices, six
   recipes, exact validation/package/test workflow, and common mistakes.
2. The guide links to authoritative source rather than redefining it, identifies
   unavailable APIs, and references only real maintained files/repositories.
3. The workflow clearly separates source checks, packaging/checksum generation,
   install-time validation, and disposable-instance smoke tests.

## Verification

```sh
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
rg -n "registerTaskPanel|registerTaskMenuAction|host.storage|RichText(Editor|ReadOnly)|Kanban" docs/public/plugins-authoring.md
rg -n "kandev-plugin-(hello|github)" docs/public/plugins*.md docs/plugins-example.md
```

The searches are audits: future-surface matches must be explicit
unavailability notes, and missing-repository matches should be absent.

## Files likely touched

- `docs/public/plugins-authoring.md`
- `docs/public/plugins.md`
- `docs/public/plugins-manifest.md`
- `docs/public/plugins-marketplace.md`
- `docs/public/extending-kandev.md`
- `docs/public/coverage.json` only if required for navigation/evidence, without
  changing experimental status

## Results

- Rewrote the canonical public guide with lifecycle, package/data layout,
  current frontend/backend matrices, six recipes, layered validation workflow,
  and common mistakes.
- Removed references to absent external example repositories and kept the
  plugin status experimental.
- `node --test scripts/validate-public-docs.test.mjs` — 58 passed.
- `node scripts/validate-public-docs.mjs` — validated 41 published docs pages.
