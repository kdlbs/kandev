---
id: "06-browser-mobile-e2e"
title: "Desktop and mobile browser coverage"
status: completed
wave: 6
depends_on: ["03-command-palette", "05-domain-target-coverage"]
plan: "plan.md"
spec: "../../specs/ui/requirements/settings-discovery.md"
---

# Task 06: Desktop and mobile browser coverage

## Acceptance

- Production-build desktop E2E proves tree filtering, exact target focus/highlight, history, and
  guarded unsaved cross-page navigation.
- Cmd+K E2E proves granular entries are search-only and exact selection lands on its control.
- Mobile E2E proves equivalent search/landing value, Sheet dismissal, 44 px actions, internal
  scrolling, viewport containment, and no document horizontal overflow.

## Verification

- `cd apps/web && pnpm e2e:run tests/settings/settings-discovery.spec.ts`
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-settings-discovery.spec.ts`
- `cd apps/web && pnpm e2e:run tests/command-panel.spec.ts -- --grep "settings discovery"`
- `cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet`

## Files likely touched

- `apps/web/e2e/tests/settings/settings-discovery.spec.ts`
- `apps/web/e2e/tests/settings/mobile-settings-discovery.spec.ts`
- `apps/web/e2e/tests/command-panel.spec.ts`
- Focused selectors/page helpers required by those scenarios

## Dependencies

Tasks 03 and 05.

## Parallelism

Sequential; it validates the integrated feature against one production build.

## Results

- `pnpm run build` — passed; production Vite bundle generated.
- `pnpm e2e:run --host --no-build tests/settings/settings-discovery.spec.ts` — 3 passed.
- `pnpm e2e:run --host --no-build --project mobile-chrome tests/settings/mobile-settings-discovery.spec.ts`
  — 1 passed.
- `pnpm e2e:run --host --no-build tests/command-panel.spec.ts -- --grep
  "settings discovery|common aliases find home commands"` — 2 passed.
- Final lint, typecheck, i18n checks, focused unit/integration suites, and `git diff --check`
  passed.
- Public docs now explain Settings-tree and Cmd+K discovery; both public-doc validators passed.
