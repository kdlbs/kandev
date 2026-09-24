---
id: "05-config-export"
title: "Configuration export"
status: done
wave: 5
depends_on: ["04-workspace-topbar"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-CONFIG-EXPORT-001
acceptance_criteria:
  - AC-OFFICE-CONFIG-EXPORT-001.1
  - AC-OFFICE-CONFIG-EXPORT-001.2
  - AC-OFFICE-CONFIG-EXPORT-001.3
  - AC-OFFICE-CONFIG-EXPORT-001.4
system_design:
  - ../../specs/office/system-design/config-export.md
---

# Task 05: Configuration export

## Summary

Restore the export route and deliver exact selected-file previews/downloads on desktop and phone.

## In scope

Own server manifest/serializer reuse, revision-checked selected ZIP endpoint, workspace
scope registration, authenticated blob download, state/error handling and mobile
list/preview composition. Keep full GET ZIP clients compatible. Update
 docs/public/office-config-sync.md with export navigation and selected-download
semantics, clearly distinct from config-sync mutation.

## Out of scope

Other work orders, unrelated refactors, live-instance changes and publication.

## Acceptance

- Navigation and direct export URLs render the real page with no-workspace/loading/error/retry states and safe workspace changes.
- Preview bytes and selected paths equal ZIP entries; stale revision is explicit and invalid paths/selections cannot escape the manifest or workspace.
- Phone file list/preview/back/download flows and desktop selection/download pass with parsed ZIP evidence, localized copy and no document overflow.

## ASCII UI preview

[Full preview](plan.md#ascii-ui-preview).

```text
UI-03: Preferences > Export
Desktop
[Preferences > Export                 Workspace actions]
[Workspace / selected count                    Download selected]
[File list + checkboxes | Selected file preview]

Phone: file list
[< Preferences   Export                Workspace actions]
[Workspace / selected count]
[[x] .kandev/kandev.yml                 View]
[[x] .kandev/agents/ceo.yml             View]
[Download selected]

Phone: selected file
[< Back to files    ceo.yml]
[Full-width content, contained code scroll]

Loading: [Loading export...]
No workspace: [Select a workspace]
Failed load: [Could not load export] [Retry]
Empty/none selected: [No files selected] [Download disabled]
Stale download: [Configuration changed] [Reload preview]
```

Control grouping and phone composition are required; spacing and wording are illustrative.

## Regression evidence

Add TestExportManifestMatchesZipBytes, TestExportSelectedZipRevisionConflict, TestExportSelectedZipRejectsInvalidPaths, and route-scope coverage. New export-preview tests cover late old-workspace response, zero selection, retry, failed blob fetch and object URL cleanup. E2E inspects ZIP entry names and contents, not merely a download event.

## Verification

Run from the repository root; every command is independently rooted. New test files
named below are outputs of this work order. Record red/green evidence.

```bash
(cd apps/backend && go test ./internal/office/config -count=1)
(cd apps/backend && go test ./internal/backendapp -run 'Office.*Scope|OfficeRouteScopeCompleteness' -count=1)
(cd apps/web && pnpm exec vitest run src/office-routes.test.ts app/office/workspace/settings/export/export-preview.test.tsx app/office/workspace/settings/export/export-file-tree.test.tsx lib/api/domains/office-config-export.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
make build-backend
make build-web
(cd apps/web && pnpm e2e:run --project=chromium e2e/tests/office/config-export.spec.ts)
(cd apps/web && pnpm e2e:run --project=mobile-chrome e2e/tests/office/mobile-config-export.spec.ts)
node scripts/validate-public-docs.mjs
node --test scripts/validate-public-docs.test.mjs
git diff --check
```

## Files likely touched

- `apps/backend/internal/office/config/service.go`
- `apps/backend/internal/office/config/handler.go`
- `apps/backend/internal/office/config/service_test.go`
- `apps/backend/internal/office/config/handler_test.go`
- `apps/backend/internal/backendapp/office_scope.go (only if route table requires registration)`
- `apps/web/src/office-routes.tsx`
- `apps/web/src/office-routes.test.ts`
- `apps/web/app/office/workspace/settings/export/`
- `apps/web/lib/api/domains/office-extended-api.ts`
- `apps/web/lib/api/domains/office-config-export.test.ts (new)`
- `apps/web/src/locales/`
- `apps/web/e2e/tests/office/config-export.spec.ts (new)`
- `apps/web/e2e/tests/office/mobile-config-export.spec.ts (new)`
- `docs/public/office-config-sync.md`

## Dependencies

04-workspace-topbar

## Risks

Never retain client-generated YAML as authoritative preview. Blob responses and stale workspace state can download the wrong data unless request identity is checked. Empty bundle must be defined by server manifest, not entity count (settings may still export).

## Parallelism

`sequential`

## Inputs

Read linked requirements/designs in full, the plan evidence, scoped AGENTS.md,
TDD guidance and the adjacent existing tests before changing code. UI tasks also
read mobile-parity and E2E fixture guidance.

## Results

- Restored the Office export route and made the server manifest the single preview/download authority.
- Added revision-checked selected ZIP downloads, exact `.kandev/` entries, workspace scoping, stale revision conflicts and invalid/empty selection rejection.
- Added API, route and file-tree regressions plus desktop/mobile preview/download E2E specs with ZIP entry/content inspection. Backend config tests, backendapp route scope checks, frontend typecheck/i18n and both production builds passed.
