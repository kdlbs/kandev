---
id: "03-marketplace-discovery"
title: "Extension discovery"
status: in_progress
wave: 3
depends_on: ["02-viewer-api"]
plan: "plan.md"
requirements:
  - REQ-PLUGINS-FILE-VIEWERS-004
acceptance_criteria:
  - AC-PLUGINS-FILE-VIEWERS-004.1
  - AC-PLUGINS-FILE-VIEWERS-004.2
  - AC-PLUGINS-FILE-VIEWERS-004.3
  - AC-PLUGINS-FILE-VIEWERS-004.4
  - AC-PLUGINS-FILE-VIEWERS-004.5
system_design:
  - ../../specs/plugins/system-design/file-viewers.md
---

# Task 03: Extension discovery

## Summary

Propagate viewer extensions from released manifests to catalog entries. Connect
unsupported-file actions to exact extension filtering and return-to-file navigation.

## In scope

Own registry schema/build, DTO/query, Browse filter chips, fallback UI, and
operator documentation. Validate package declarations independently of listing claims.

## Out of scope

Other work orders, publication, and changes to unrelated editor behavior.

## Acceptance

- Extension tags derive from structured metadata; malformed and wildcard claims are rejected.
- Discovery sends no file identity/content to registry sources and never auto-installs.
- Desktop/mobile install-return, offline, incompatible, disabled, and no-match cases pass.

## UI composition preview

UI-03: Unsupported file and discovery. File panel displays format, Find viewer
plugin, and existing download/external-open. Browse displays an extension chip,
matching entries, and normal explicit Install actions. Phone uses the existing
full-page marketplace composition and a Back action to the file. Distinct
states: no match, offline catalog, incompatible, installed but disabled, and ready.

See [combined previews](plan.md#ui-composition-previews). Apply the AC IDs above.

## Verification

Run from the Kandev repository root unless specified otherwise. New suite names
are implementation deliverables, not existing test evidence.

```bash
node --test plugin-registry/build-index.test.mjs
(cd apps/backend && go test ./internal/plugins/marketplace ./internal/plugins/manifest -count=1)
(cd apps/web && pnpm exec vitest run components/task/unavailable-file-viewers.test.tsx lib/plugins/file-viewer-selection.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/plugins/file-viewer-marketplace.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/plugins/mobile-file-viewer-marketplace.spec.ts)
node scripts/validate-public-docs.mjs
```

## Files likely touched

plugin-registry/schema.json; plugin-registry/build-index.mjs;
apps/backend/internal/plugins/marketplace/; apps/web/components/settings/plugins/;
apps/web/components/task/file-binary-viewer.tsx; docs/public/plugins-marketplace.md.

## Dependencies

02-viewer-api

## Risks

Old indexes lack structured formats. Treat absence as unknown, never infer support from prose.

## Parallelism

Sequential. No delegation authorized.

## Inputs

- [Requirements](../../specs/plugins/requirements/file-viewers.md).
- [Design](../../specs/plugins/system-design/file-viewers.md).
- [Security matrix](../../reviews/file-viewers-security.md).
- Existing registration, approval, file-reader, and marketplace tests beside the scoped source.

## Results

Implementation is present. See the [implementation record](../../reviews/file-viewers-implementation.md)
for completed checks and remaining validation gates. No publication is authorized.
