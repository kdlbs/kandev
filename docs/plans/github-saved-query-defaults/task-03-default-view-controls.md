---
id: "03-default-view-controls"
title: "Expose default-view controls"
status: done
wave: 3
depends_on: ["02-apply-default-selection"]
plan: "plan.md"
spec: "../../specs/ui/requirements/github-saved-query-defaults.md"
---

# Task 03: Expose Default-View Controls

## Acceptance

- Desktop and mobile saved-query lists show accessible set/clear markers that
  do not select or delete the row; failed/pending mutations are safe.
- GitLab's shared scope bar remains unchanged when optional default props are absent.
- Desktop and mobile Playwright scenarios prove persistence and application of
  independent kind defaults; mobile also proves 44px reachability and no overflow.

## Files

- `apps/web/components/integrations/presets-scope-bar-base.tsx`
- `apps/web/components/integrations/presets-scope-bar-base.test.tsx`
- `apps/web/components/github/my-github/presets-scope-bar.tsx`
- `apps/web/components/github/my-github/presets-sidebar.tsx`
- `apps/web/src/locales/en/integrations.json`
- `apps/web/src/locales/pseudo/integrations.json`
- `apps/web/e2e/tests/github/github-scope-bar.spec.ts`
- `apps/web/e2e/tests/github/mobile-github-sidebar.spec.ts`
- `apps/web/e2e/pages/mobile-github-page.ts`

## Verification

```bash
cd apps && pnpm --filter @kandev/web exec vitest run components/integrations/presets-scope-bar-base.test.tsx
cd apps/web && pnpm e2e:run tests/github/github-scope-bar.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/github/mobile-github-sidebar.spec.ts
```

## Result

- RED: component regression showed pointer-up selecting the saved row; desktop
  E2E showed the default action replacing the current query; mobile E2E could
  not find a touch-sized default action.
- GREEN: component suite passed 3 tests. Focused desktop and mobile Playwright
  scenarios passed, including future-only selection, per-kind persistence,
  44px mobile geometry, and no horizontal overflow.
