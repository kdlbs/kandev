---
id: "01-nested-navigation"
title: "Nest SSH management and preserve old links"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-CONNECTIONS-001
  - REQ-EXECUTORS-SSH-CONNECTIONS-004
  - REQ-EXECUTORS-SSH-CONNECTIONS-005
acceptance_criteria:
  - AC-EXECUTORS-SSH-CONNECTIONS-001.5
  - AC-EXECUTORS-SSH-CONNECTIONS-004.4
  - AC-EXECUTORS-SSH-CONNECTIONS-004.6
  - AC-EXECUTORS-SSH-CONNECTIONS-005.2
  - AC-EXECUTORS-SSH-CONNECTIONS-005.3
  - AC-EXECUTORS-SSH-CONNECTIONS-005.6
system_design:
  - ../../specs/executors/system-design/ssh-connection-manager.md
---

# Task 01: Nest SSH management and preserve old links

## Summary

Make Executors the settings entry for shared SSH connections. Preserve old
bookmarks and all existing manager query state with the current redirect helper.

## In scope

- Add canonical static route, reserve its segment in the dynamic profile matcher,
  and add central route/query helper; migrate all UI
  callers, including adoption and bastion returns. Preserve the old alias.
- Remove peer menu entry; add gated navigation on the Executors page, search
  parentage, breadcrumb ancestry, and active-menu/remembered-path behavior.
- Desktop/mobile UI tests, localization, public navigation instructions and
  coverage mapping. Update existing E2E helper URLs to the canonical path,
  retaining explicit alias tests.

## Out of scope

Contextual identity creation and executor draft continuation (Task 02), backend
changes, access changes, SSH behavior, and original package acceptance gaps.

## Acceptance

1. UI-01 works from menu, hub and search on desktop/phone, including zero
   profiles. Executors stays active and no independent SSH settings entry exists.
2. Old URLs replace history and open the same tab/record/draft/adoption/bastion
   context without mutation. Feature-off and non-admin gates apply to both routes.
3. Updated navigation docs, seven-language copy, unit checks and browser checks
   pass, including 767/768px, coarse touch targets, focus and overflow.

## UI preview

UI-01 excerpt; see [full preview and states](plan.md#ui-01-executors-owns-ssh-connections).
Structured preview follows the task's rich-output requirement.

| Region | Desktop | Phone |
| --- | --- | --- |
| Entry | Executors active in settings menu | Settings index > Executors |
| Hub | SSH connections row above current profiles | Full-width SSH connections row, at least 44px |
| Manager | Executors > SSH connections; tabs; list and detail | Back to Executors; tabs; list then focused detail |
| Unavailable | No entry; direct route shows existing unavailable state | Same restriction |

Hierarchy and reachable actions are required; illustrative labels use current
localized tokens. Reuse the single settings scroll owner and shared control
classes (28px fine pointer; 44px touch). ACs: 001.5, 004.4, 004.6, 005.3, 005.6.

## Verification

Start with failing route/menu behavior tests under `/tdd`, then implement. New
files named below must be created; a no-tests-found result is not a pass. All
commands run from repository root. Run browser projects sequentially.

```bash
(cd apps/web && pnpm exec vitest run src/settings-routes.test.ts src/settings-route-helpers.test.ts lib/settings/ssh-connection-settings-routes.test.ts lib/settings-discovery/catalog.test.ts components/settings/settings-breadcrumbs.test.ts components/app-sidebar/sections/settings/settings-tree.test.ts components/app-sidebar/sections/settings/settings-tree-render.test.tsx app/settings/executors/page.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web lint)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/ssh-navigation.spec.ts tests/settings/ssh-connections.spec.ts tests/settings/ssh-connections-disabled.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-ssh-navigation.spec.ts tests/settings/mobile-ssh-connections.spec.ts)
(cd apps/web && pnpm e2e:run --project auth tests/auth/ssh-connections-permissions.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The new navigation browser specs cover empty hub, command search, canonical
reload, alias query/hash, browser Back/Forward, gate rejection, and no redirect
mutations. Existing connection specs cover preserved bastion-return behavior.
Include pseudo-locale checks in the navigation specs. Use `backend.useEnv` and
restore feature state in `finally`. No external SSH access.

## Files likely touched

Paths are relative to `apps/web/` unless prefixed with `docs/`.

- `src/settings-routes.tsx`, `src/settings-routes.test.ts`, `src/settings-route-helpers.test.ts`
- `lib/settings/ssh-connection-settings-routes.ts` and `.test.ts` (new)
- `app/settings/executors/page.tsx`, `page.test.tsx` (new test)
- `components/app-sidebar/sections/settings/settings-menu-sections.ts` and tree tests
- `lib/settings-discovery/catalog/executors.ts`, `catalog.test.ts`
- `components/settings/settings-breadcrumbs.ts` and `.test.ts`; exact manager
  rule before the generic profile matcher
- `components/settings/ssh-connections/{settings,managed-binding,adoption}.tsx`
- `e2e/tests/settings/ssh-navigation.spec.ts`, `mobile-ssh-navigation.spec.ts` (new)
- Existing `e2e/tests/settings/*ssh-connections.spec.ts`, disabled and auth specs,
  and `e2e/helpers/ssh-identities.ts`
- `src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/` affected catalogs
- `docs/public/executors.md`, `docs/public/coverage.json`

## Dependencies

None. Build on `b386a0d3d1c`; retain its implementation. The original package's
remaining acceptance work is not a prerequisite for this UI relocation.

## Risks

Static route/profile collisions, query loss, duplicate menu entries, and loading
management hooks before gates pass. Do not let a redirect perform adoption.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/ssh-connection-manager.md), ACs above.
- [Settings design](../../specs/executors/system-design/ssh-connection-manager.md#settings-and-mobile-composition).
- Existing `SettingsRedirect`, settings catalog parent relationships, current
  executor hub, mobile settings layout, and SSH browser fixtures.

## Results

Completed locally on 2026-10-23. The manager is under Executors; its old route
redirects with query/hash preserved. The reserved segment cannot resolve as a
profile. Search, breadcrumbs, menu ownership, empty-hub entry and permission
checks are aligned. Existing adoption and bastion callers use the canonical
query helper. Public navigation docs and route coverage are updated.

RED: new route, breadcrumb, discovery hierarchy, peer-menu, empty-hub and query
cases failed before implementation. GREEN: the combined 197-test unit run and
final 57-test route/editor subset passed. Browser navigation passed at 767,
768, 1280px and touch 390/820px, including old links, history, reload, search,
pseudo-locale and no redirect mutations. Disabled/member tests cover both URLs.
See the [plan results](plan.md#verification-results) for exact aggregate counts,
environment adjustments, documentation checks and unchanged original gaps.
