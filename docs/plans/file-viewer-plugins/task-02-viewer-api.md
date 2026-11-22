---
id: "02-viewer-api"
title: "Viewer API and host interaction"
status: in_progress
wave: 2
depends_on: ["01-bounded-file-access"]
plan: "plan.md"
requirements:
  - REQ-PLUGINS-FILE-VIEWERS-001
  - REQ-PLUGINS-FILE-VIEWERS-003
  - REQ-PLUGINS-FILE-VIEWERS-005
  - REQ-PLUGINS-FILE-VIEWERS-006
acceptance_criteria:
  - AC-PLUGINS-FILE-VIEWERS-001.1
  - AC-PLUGINS-FILE-VIEWERS-001.2
  - AC-PLUGINS-FILE-VIEWERS-001.3
  - AC-PLUGINS-FILE-VIEWERS-001.4
  - AC-PLUGINS-FILE-VIEWERS-001.5
  - AC-PLUGINS-FILE-VIEWERS-001.6
  - AC-PLUGINS-FILE-VIEWERS-003.4
  - AC-PLUGINS-FILE-VIEWERS-003.5
  - AC-PLUGINS-FILE-VIEWERS-005.3
  - AC-PLUGINS-FILE-VIEWERS-005.4
  - AC-PLUGINS-FILE-VIEWERS-006.1
  - AC-PLUGINS-FILE-VIEWERS-006.2
  - AC-PLUGINS-FILE-VIEWERS-006.3
  - AC-PLUGINS-FILE-VIEWERS-006.4
  - AC-PLUGINS-FILE-VIEWERS-006.5
  - AC-PLUGINS-FILE-VIEWERS-006.6
  - AC-PLUGINS-FILE-VIEWERS-006.7
system_design:
  - ../../specs/plugins/system-design/file-viewers.md
---

# Task 02: Viewer API and host interaction

## Summary

Add manifest declarations, SDK registration, lifecycle-safe resolver, and a fixture
viewer. Integrate desktop/mobile file panels, permission UI, and host plugin settings.

## In scope

Own declarations, SDK, host UI, Ask/Allow/Deny selector, localized copy, fixture,
and host public docs. Publish the authoring guidance from the design together with
this API; update manifest reference and PLUGIN-API.md. Include executor E2E proof.

## Out of scope

Other work orders, publication, and changes to unrelated editor behavior.

## Acceptance

- Matching viewers mount only after applicable admission and unload on context/policy change.
- Desktop/mobile approval, settings, restoration, and dirty-buffer preservation pass E2E.
- SDK contract, fixture, public guide, and cleanup example agree; all required locales pass.

## UI composition preview

UI-01: File preview, desktop and phone. Entry: opening a supported file.
Desktop uses the existing file panel with model viewport, dimensions strip,
Open with, Fit, and zoom. Phone uses a full-height file surface, readable
dimensions and bottom reachable controls. Workspace files load automatically;
errors offer Retry without automatic fallback. No GPU renders before permission.

UI-02: External approval and settings. Entry: explicit external-file open under
Ask. Desktop dialog: plugin identity, execution, resolved path, size, then
Preview once and Cancel. Phone drawer: scrollable wrapping metadata body and
fixed safe-area actions. Settings has a labelled Ask/Allow/Deny selector with
Ask selected initially and a concise trust explanation. Visible restored tabs
repeat admission. Only external files under Ask prompt. Cancel dismisses that
prompt; loading and ready previews stop through file close or navigation.

See [combined previews](plan.md#ui-composition-previews). Apply the AC IDs above.

## Verification

Run from the Kandev repository root unless specified otherwise. New suite names
are implementation deliverables, not existing test evidence.

```bash
(cd apps/backend && go test ./internal/plugins/manifest -count=1)
(cd apps/packages/plugin-sdk && pnpm test && pnpm run typecheck)
(cd apps/web && pnpm exec vitest run lib/plugins/registry-file-viewers.test.ts lib/plugins/file-viewer-selection.test.ts hooks/use-file-preview-policy.test.ts hooks/use-plugin-file-preview.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/plugins/file-viewer.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/plugins/mobile-file-viewer.spec.ts)
# Required executor suite is not delivered or certified yet.
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --shards 1 --project containers tests/docker/file-viewer-executors.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

apps/packages/plugin-sdk/src/index.ts; apps/web/lib/plugins/;
apps/web/components/task/file-editor-panel.tsx; file-tab-content.tsx;
mobile/mobile-file-viewer-panel.tsx; apps/web/components/settings/plugins/;
apps/backend/internal/plugins/manifest/; apps/backend/cmd/plugin-fixture/;
docs/public/plugins-authoring.md; plugins-manifest.md; docs/plans/plugins/PLUGIN-API.md.

## Dependencies

01-bounded-file-access

## Risks

Native trust must remain explicit. Browser approval cannot attest human presence against malicious same-origin code.

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

### PR review follow-up

Regression coverage now includes replacement of an unavailable saved viewer,
returning from text to a single or multiple viewers, discarded late text reads,
repository links on unsupported files, and late registration for restored tabs.
Policy writes reset changed digests before revision validation. Lifecycle
revocation preserves lock order. Missing unrelated exclusions do not prevent
reads, while existing excluded symlink targets remain protected.

Desktop discovery return uses the persisted environment layout, including the
active file panel and its repository parameters. It does not enqueue a second
file selection. The phone layout uses the expiring pending selection because it
has no dockview API. The return helper tests cover both branches, and the browser
flow checks the marketplace round trip on desktop and phone.

Review follow-up also covers read-failure restoration, viewer preferences that
do not add file tabs, policy reload after uncertain writes, and metadata-response
failure cleanup. Marketplace tests use a controlled external registry with the
real Kandev catalog endpoint. Shared repository and policy state is restored in
failure-safe cleanup. AC-PLUGINS-FILE-VIEWERS-001.6 is verified by the shared rich
preview tests and installed-package desktop/phone tests.
