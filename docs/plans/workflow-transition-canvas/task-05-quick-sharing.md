---
id: "05-quick-sharing"
title: "Simplify reviewed canvas sharing"
status: done
wave: 5
depends_on:
  - "04-canvas-rename"
plan: "plan.md"
requirements:
  - REQ-CANVASES-NAME-SHARE-002
acceptance_criteria:
  - AC-CANVASES-NAME-SHARE-002.1
  - AC-CANVASES-NAME-SHARE-002.2
  - AC-CANVASES-NAME-SHARE-002.3
  - AC-CANVASES-NAME-SHARE-002.4
  - AC-CANVASES-NAME-SHARE-002.5
  - AC-CANVASES-NAME-SHARE-002.6
system_design:
  - ../../specs/canvases/system-design/canvas-name-and-share-defaults.md
---

# Task 05: Simplify reviewed canvas sharing

## Summary

Make the active release's known distribution data visible before preparation
and shorten the path from Share to reviewed downloads.

## In scope

- Restore safe release metadata in the canvas HTTP projection. Add an
  authorized, release-bound export-defaults read that supplies the server's
  first compatible distribution version when the manifest omits one.
- Reuse the selected release's ID, version, name, description, author, source
  mode, and optional repository URL. Leave absent license and other
  author-supplied details explicit. Never guess a license.
- Recompose the desktop Dialog and phone Drawer around required gaps,
  Prepare downloads, and an editable Package details disclosure. Keep
  inventory, private-content warning, and two separate downloads.
- Localize copy in all required catalogs and update public sharing guidance.

## Out of scope

Auto-publishing, changing the active release, persistent share profiles, and
relaxing the distribution validator or download reauthorization.

## Acceptance

1. The example canvas opens Share with its package ID, current release version, description,
   and manifest author already present. License is the only author decision
   if the server supplies a valid compatibility minimum.
2. Filling the remaining gaps permits one Prepare action, then shows the
   exact release's inventory and both downloads. Editing a field or changing
   release invalidates the review and requires fresh preparation.
3. Desktop and phone flows retain drafts on validation error, announce
   loading/failure, provide Retry, and never download or publish before the
   review/explicit download action.

## Verification

```bash
(cd apps/backend && go test ./internal/canvas ./internal/backendapp)
(cd apps/web && pnpm exec vitest run components/settings/canvas-share-dialog.test.tsx hooks/domains/canvas/use-canvas-share.test.ts)
(cd apps/web && pnpm e2e:run --host --project=chromium tests/canvas/canvas-sharing.spec.ts)
(cd apps/web && pnpm e2e:run --host --project=mobile-chrome tests/canvas/mobile-canvas-sharing.spec.ts)
(cd apps/web && pnpm run i18n:check)
```

## Files likely touched

- `apps/backend/internal/canvas/distribution_export.go` and tests
- `apps/backend/internal/backendapp/canvas_distribution_routes.go` and tests
- `apps/backend/internal/backendapp/canvas_routes.go`
- `apps/web/lib/api/domains/canvas-distribution-api.ts`
- `apps/web/components/settings/canvas-share-dialog.tsx` and tests
- `apps/web/hooks/domains/canvas/use-canvas-share.ts` and tests
- `apps/web/src/locales/*/canvases.json`
- `apps/web/e2e/tests/canvas/{canvas-sharing,mobile-canvas-sharing}.spec.ts`
- Relevant `docs/public/**` sharing page

## Dependencies

Task 04 is sequenced first because Share can use the latest mutable canvas
name only when a release display name is missing. The two work orders also
touch shared host API files.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/canvases/requirements/canvas-name-and-share-defaults.md)
- [System design](../../specs/canvases/system-design/canvas-name-and-share-defaults.md)
- [Complete UI preview](plan.md#ascii-ui-preview)

## Results

Restored safe release metadata in host responses and added authorized,
release-bound export defaults with server-owned compatibility floor `0.95.0`.
Share presents missing fields first, keeps package details editable, and
retains reviewed inventory and separate downloads. Canvas/backend tests,
typecheck, localization checks, focused unit tests, and desktop/phone sharing
E2E passed.
