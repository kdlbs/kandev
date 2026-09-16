---
id: "01-plugin-previews"
title: "Extend plugin marketplace previews"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-MARKETPLACE-002
acceptance_criteria:
  - AC-PLUGINS-MARKETPLACE-002.1
  - AC-PLUGINS-MARKETPLACE-002.2
  - AC-PLUGINS-MARKETPLACE-002.3
  - AC-PLUGINS-MARKETPLACE-002.4
system_design:
  - ../../specs/plugins/system-design/marketplace.md
---

# Task 01: Extend plugin marketplace previews

## Summary

Complete the optional native-plugin preview extension that shares the registry
metadata and gallery primitives used by canvas entries. Keep preview metadata
outside plugin packages and preserve the existing native installation path.

## In scope

- Ordered optional `previews: [{url, alt}]` fields for official and custom
  plugin registry entries and generated catalog records.
- First-image cover presentation, details gallery, keyboard and touch controls,
  image-position descriptions, one-image mode, and contained image failures.
- Existing plugin rows and install actions when a listing has no previews.
- Registry-owned preview metadata, so URLs, descriptions, and order can change
  without releasing a plugin version.
- Shared gallery tests and native-plugin marketplace regression coverage.
- Public plugin marketplace and manifest guidance, the linked authoring guide,
  and plugin registry README examples.

## Out of scope

Canvas package validation, canvas installation review, screenshot requirements
for bundle creation, native plugin package changes, and a new installation path.

## Acceptance

- Official and custom plugin entries accept an ordered HTTPS preview list while
  existing entries without previews remain valid.
- Listings with previews show the first image as the cover and expose the full
  gallery; listings without previews keep their current row and install action.
- Gallery navigation works with keyboard and touch, retains installation when an
  image fails, and never executes package code in the preview.
- Preview metadata remains registry-owned and can change independently of plugin
  package releases.

## Verification

```bash
rtk bash -lc 'KANDEV_CANVAS_PACKAGE_INSPECTOR=apps/backend/bin/canvas-package node --test plugin-registry/build-index.test.mjs'
(cd apps/web && rtk pnpm exec vitest run components/settings/plugins/marketplace-preview-gallery.test.tsx components/settings/plugins/plugin-row.test.tsx)
(cd apps/web && rtk pnpm e2e:run --project chromium tests/plugins/plugins.spec.ts -- --retries=0)
(cd apps/web && rtk pnpm e2e:run --project mobile-chrome tests/plugins/mobile-plugin-settings-row.spec.ts -- --retries=0)
rtk proxy node --test scripts/validate-public-docs.test.mjs
rtk git diff --check -- docs/public/plugins-marketplace.md docs/public/plugins-manifest.md docs/public/plugins-authoring.md plugin-registry/README.md
```

Use local fixtures and the built inspector. Do not publish a registry entry or
execute a plugin while testing preview details.

## Files likely touched

- `plugin-registry/schema.json`, `build-index.mjs`, and `build-index.test.mjs`
- `apps/web/components/settings/plugins/marketplace-preview-gallery.tsx`
- Existing plugin row/details components and their focused tests
- `apps/web/components/settings/plugins/plugins-settings.tsx`
- `apps/web/lib/api/domains/marketplace-api.ts` and marketplace hooks/tests
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,pseudo}/`
- `docs/public/plugins-marketplace.md`
- `docs/public/plugins-manifest.md`
- `docs/public/plugins-authoring.md`
- `plugin-registry/README.md`

## Dependencies

The canvas marketplace plan provides the shared registry and gallery primitives;
the plugin contract remains independently traceable to the plugin design.

## Risks

Do not make previews part of a plugin manifest or release archive. Remote image
failures must not block native installation, and the details surface must never
load a runtime URL or execute package code.

## Parallelism

`sequential`

## Inputs

- Plugin marketplace requirements and the registry preview system design.
- Existing `buildEntry`, `build-index.test.mjs`, plugin rows, and native install
  dialog.
- Shared marketplace gallery behavior and the plugin documentation validation
  guide.

## Results

Implemented optional ordered plugin previews in registry and catalog records,
shared gallery details with keyboard and touch controls, image-failure fallback,
and unchanged native installation for entries without previews. Updated the
plugin marketplace, manifest, authoring, and registry README guidance.

Verification: the existing plugin marketplace regression passed 16 tests; the
registry build-index suite and shared gallery coverage passed with the canvas
marketplace validation; public documentation validation and diff checks passed.
