---
id: "06-sharing-ui"
title: "Add canvas sharing controls"
status: done
wave: 6
depends_on:
  - "05-marketplace-ui"
plan: "plan.md"
requirements:
  - REQ-CANVASES-MARKETPLACE-001
  - REQ-CANVASES-MARKETPLACE-002
  - REQ-CANVASES-MARKETPLACE-003
  - REQ-CANVASES-MARKETPLACE-006
acceptance_criteria:
  - AC-CANVASES-MARKETPLACE-001.2
  - AC-CANVASES-MARKETPLACE-001.3
  - AC-CANVASES-MARKETPLACE-002.1
  - AC-CANVASES-MARKETPLACE-003.1
  - AC-CANVASES-MARKETPLACE-003.2
  - AC-CANVASES-MARKETPLACE-003.3
  - AC-CANVASES-MARKETPLACE-003.4
  - AC-CANVASES-MARKETPLACE-006.1
  - AC-CANVASES-MARKETPLACE-006.2
  - AC-CANVASES-MARKETPLACE-006.3
  - AC-CANVASES-MARKETPLACE-006.4
system_design:
  - ../../specs/canvases/system-design/marketplace-sharing.md
---

# Task 06: Add canvas sharing controls

## Summary

Expose bundle/source downloads and manual sharing instructions from published
canvas host actions. Let authors prepare distribution metadata without changing the running canvas.
Screenshots are provided later through registry URLs, never required for downloads.

## In scope

- Share entry in task/workspace host controls and workspace canvas rows.
- Release-bound metadata/source form, validation, and temporary draft retention.
  No screenshot upload, reorder, required cover, or preview URL field.
- Preparation, inventory, two downloads, expiry/stale-release recovery, cleanup.
- Information dialog/drawer with manual repository/release/direct-link and
  registry-PR instructions, copyable safe examples, and no external mutations.
- Shared domain hook, localized host copy, accessible desktop/phone E2E.
- Export-to-import browser round trip with the real installer from Task 05.

## Out of scope

Automatic screenshots, drag-only reordering, source editor, permanent publishing
profiles, external account connection, remote repository/release/PR writes.

## Acceptance

- UI-03 prepares matching bundle/source from the active release, without any
  screenshot requirement; invalid/stale/expired results retain inputs
  and cannot download silently substituted content.
- UI-04 gives complete manual instructions and copyable registry examples
  without any external mutation; downloads remain usable without code-host auth.
- Desktop/phone tests download both files, inspect their contents, import the
  bundle, and open the resulting independent canvas; help/back and gallery
  controls satisfy keyboard, localization, and touch geometry requirements.

## ASCII UI preview

Excerpt of [UI-03/UI-04 in the plan](plan.md#ui-03-share-canvas).
AC-CANVASES-MARKETPLACE-001.3, 002.1, 003.1-003.4, and 006.1-006.3.

```text
Desktop
+-------------------------------------------------------------+
| Share canvas                                        [Close] |
| Release / package ID / version / author / license           |
| Description / source mode                                  |
| [How to share]                         [Prepare downloads]  |
| Ready: file inventory, sizes, private-content reminder      |
| [Download bundle] [Download source]                        |
+-------------------------------------------------------------+

Phone
+------------------------------+
| [Back] Share canvas          | fixed
| Metadata / source fields     |
| [How to share]               | opens instruction drawer
| File inventory / sizes       |
| Check for private content.   |
| [Download bundle]            |
| [Download source]            |
|------------------------------|
| [Prepare downloads]          | safe-area footer until ready
+------------------------------+

Help drawer/dialog
1. Download source and bundle
2. Create repository; upload source
3. Publish bundle; copy direct link
4. Send link/file to another user
Optional: add image URLs to the registry entry; request listing
[Copy registry example] [Done]
```

Screenshots are not an export input. Missing project source points
to Edit and republish. One-column phone form has one scroll owner. The help
drawer has its own bounded scroll only while it is active; closing restores
focus/input state. Both downloads remain reachable after either completes.
Current source/version changes invalidate prepared output. These structures
are required; sample labels and spacing are illustrative.

## Verification

```bash
(cd apps/web && rtk pnpm exec vitest run hooks/domains/canvas/use-canvas-share.test.ts components/settings/canvas-share-dialog.test.tsx components/settings/canvas-share-help.test.tsx components/settings/canvas-host-components.test.tsx components/settings/workspace-canvases-page.test.tsx lib/api/domains/canvas-distribution-api.test.ts)
(cd apps/web && rtk pnpm run typecheck)
(cd apps/web && rtk pnpm exec eslint components/settings/canvas-share-dialog.tsx components/settings/canvas-share-help.tsx components/settings/canvas-host-components.tsx components/settings/workspace-canvases-page.tsx hooks/domains/canvas lib/api/domains/canvas-distribution-api.ts)
(cd apps/web && rtk pnpm run i18n:check)
(cd apps/web && rtk pnpm run i18n:ratchet)
(cd apps/web && rtk pnpm e2e:run --project chromium tests/canvas/canvas-sharing.spec.ts -- --retries=0)
(cd apps/web && rtk pnpm e2e:run --project mobile-chrome tests/canvas/mobile-canvas-sharing.spec.ts -- --retries=0)
rtk git diff --check
```

Use file/download fixtures and browser geometry; no external hosting credentials.
Generate Traditional Chinese/pseudo catalogs through the repository generators
during implementation, then run the checks above.

## Files likely touched

- `apps/web/components/settings/canvas-share-dialog.tsx`, tests (new)
- `apps/web/components/settings/canvas-share-help.tsx`, tests (new)
- `apps/web/hooks/domains/canvas/use-canvas-share.ts`, tests (new)
- `apps/web/components/settings/canvas-host-components.tsx`, host route/wrappers
- `apps/web/components/settings/workspace-canvases-page.tsx`
- `apps/web/lib/api/domains/canvas-distribution-api.ts`, tests
- All real locales and generated pseudo catalog
- `apps/web/e2e/tests/canvas/canvas-sharing.spec.ts`,
  `mobile-canvas-sharing.spec.ts`, shared distribution fixtures (new)

## Dependencies

Tasks 01-05, especially export endpoints and the real bundle import UI.

## Risks

Exported application files can contain private data. The UI must
show the inventory/review notice without claiming automated sanitization.
Do not auto-invalidate source downloads by dismissing the form after the first
download. Download and clipboard browser fallbacks must work in current host
environments. Keep instructions about GitHub registry versus other-host links clear.

## Parallelism

`sequential`

## Inputs

- Design: Export preparation, HTTP contracts, UI composition, Mobile contract.
- Existing host lifecycle action drawer and shared copy/download helpers.
- Plan UI-03/UI-04 and the /e2e fixture/cleanup guidance.

## Results

Implemented host and workspace-row Share actions, responsive share dialog/drawer,
release-bound bundle/source preparation and downloads, private-content review
notice, and manual repository/release/registry instructions. Sharing accepts no
screenshot input and keeps previews separate from package permissions.

Verification: the focused frontend suite passed 21 tests, desktop sharing E2E
passed, mobile sharing E2E passed, and the export/preparation backend race tests
passed 3 tests.

Review remediation: share-dialog dismissal now cancels prepared exports, and
stale asynchronous downloads cannot overwrite the current dialog state.
