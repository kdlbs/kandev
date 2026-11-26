---
id: "04-exclusion-setup-warnings-web"
title: "Show exclusion setup warnings on routing settings and workflow import"
status: done
wave: 3
depends_on:
  - "03-exclusion-setup-warnings-backend"
plan: "plan.md"
requirements:
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-005
acceptance_criteria:
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-005.3
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-005.5
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-005.6
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-005.7
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-005.8
system_design:
  - ../../specs/office/system-design/review-author-provider-exclusion.md
---

# Task 04: Show exclusion setup warnings on routing settings and workflow import

## Summary

Web half of system design §9: a shared warning list component, the routing
settings banner, the import-dialog preview warnings, the save-time warning
toast, locales, and desktop and phone E2E.

## Scope

- Types for `author_exclusion_warnings`, the PUT response, and
  import `warnings`.
- `AuthorExclusionWarningList`: a pure mapper from code to an i18n key with
  interpolated names, rendered inside `Alert` with the amber
  `IconAlertTriangle`.
- Routing page: a banner above `RoutingEnableCard`. On save, a warning toast
  replaces the success toast when the response carries warnings.
- Import dialog: the list above the confirm action, using preview `warnings`.
  The import result's warnings are shown in the completion toast.
- Keys for each code, the banner title, and the toast, in all seven catalogs.
  Use `pnpm run i18n:zh-hant` for the Traditional pair. No em dash.

## Exclusions

Workflow sync UI changes (its banner already shows warning strings verbatim),
the per-agent override form, and the #5347 editor.

## UI-02 and UI-03 (from [plan](plan.md#ui-02-provider-routing-settings-exclusion-setup-banner))

```text
| /!\ Reviewer exclusion setup                                 |
|  - Office Review / Review (reviewer): only claude-acp is     |
|    configured, so review runs wait whenever the work ran on  |
|    claude-acp.                                               |
```

The full desktop and phone previews are in the plan. Placement and list
structure are required; the wording is illustrative.

## Implementation acceptance

- Vitest: the mapper covers every code, including `check_failed`; the banner
  renders nothing for an empty list.
- E2E (chromium and mobile-chrome): with routing enabled on one provider and a
  workflow imported with `exclude_author_provider: true`, the import preview
  shows the warning and the import succeeds, and the routing page shows the
  banner. After a second provider is added and saved, the banner disappears.
  On phone, the banner has no horizontal overflow.
- i18n checks and the ratchet pass.

## Verification

```bash
cd apps/web
pnpm exec vitest run --maxWorkers=1 app/office/workspace/routing app/settings/workspace hooks/domains/office/use-workspace-routing.test.tsx
pnpm run typecheck
pnpm run i18n:check
pnpm run i18n:ratchet
pnpm e2e:run --host --shards 1 --project chromium tests/office/office-review-author-exclusion-setup.spec.ts
pnpm e2e:run --host --shards 1 --no-build --project mobile-chrome tests/office/mobile-review-author-exclusion-setup.spec.ts
```

## Likely files

- `apps/web/lib/state/slices/office/routing-types.ts`
- `apps/web/hooks/domains/office/use-workspace-routing.ts` (+ test)
- `apps/web/app/office/workspace/routing/page.tsx`, new
  `components/author-exclusion-warning-banner.tsx` (+ test)
- `apps/web/app/settings/workspace/use-workflow-import.ts`,
  `workspace-workflows-dialogs.tsx` (+ tests)
- `apps/web/src/locales/*/office.json`, `settings.json`
- `apps/web/e2e/tests/office/office-review-author-exclusion-setup.spec.ts`,
  `mobile-review-author-exclusion-setup.spec.ts`

## Risks

- The import dialog lives in workspace settings, outside Office. Keep the
  shared component free of Office store state so both pages can use it.

## Results

- `components/author-exclusion-warning-list.tsx` (shared, no Office store
  state) and `lib/types/author-exclusion.ts`. The routing page banner sits
  above the enable card. Save shows a warning toast when warnings remain. The
  import review pauses the auto-submit when the preview carries warnings, and
  has a "Review imported workflows" header when no profiles need resolving.
- Vitest covers the warning list, the routing hook (load and save), and the
  import hook (pause, then confirm; auto-submit without warnings).
- E2E: `office-review-author-exclusion-setup.spec.ts` (chromium) checks the
  import warning, a successful import, the banner, and that the banner clears
  after two providers are saved. `mobile-review-author-exclusion-setup.spec.ts`
  (mobile-chrome) checks the drawer warning and the banner with no horizontal
  overflow. Both pass.
- Copy was added in seven catalogs (Traditional Chinese generated) plus
  pseudo. i18n check and ratchet pass.
