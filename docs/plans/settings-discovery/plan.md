---
spec: docs/specs/ui/requirements/settings-discovery.md
created: 2026-08-05
status: completed
---

# Implementation Plan: Settings Discovery

## Overview

Build a dedicated first-party settings catalog, then consume it from tree search and search-only
Cmd+K commands. Stable hash targets connect both surfaces to rendered controls. Work proceeds from
pure catalog/search contracts through UI integration, coverage, localization, and production-build
desktop/mobile E2E.

## Frontend

### Discovery catalog and search

Create `apps/web/lib/settings-discovery/` with typed definitions, domain-split catalogs, dynamic
resolution, deterministic Unicode-aware search, exclusions, and target/navigation helpers. Reuse
catalog route metadata from `general-nav.ts` and Settings tree groups rather than retaining parallel
deep-link lists.

### Settings tree and exact targets

Add the sticky search/result UI to `SettingsTree`. Add a provider/registration protocol under
`components/settings/` that handles initial, delayed, repeated, soft-navigation, and browser-history
targets independently of `usePathname`, whose snapshot deliberately excludes fragments. Cross-page
links retain `navigation-guard`; same-page fragments bypass leave confirmation.

### Command palette

Extend `CommandItem` with display context and search-only visibility. Generate granular commands
from the catalog, preserve the existing top-level destination and legacy aliases, and load dynamic
settings data only while the Commands palette is open. When a query is present, keep regular
matches in Commands and render search-only catalog matches under their localized Settings group.

### Coverage and localization

Index every canonical first-party settings page and stable inline control, with explicit exclusions
for transient/generated/plugin-owned UI. Resolve all copy at render time, regenerate pseudo locale,
and keep current values out of searchable terms.

### Mobile design contract

Desktop outcome: search the permanent Settings tree and land on an exact control. Phone entry point:
the existing `SettingsMobileMenu` full-height Sheet. Nearest shipped exemplar:
`components/kanban/mobile-menu-sheet.tsx` contributes fixed-header/internal-scroll geometry, while
the existing Settings Sheet remains because this is dense hierarchical navigation rather than a
short temporary choice. Shared catalog/filter/target logic serves both. Navigation Sheet and page
content each retain one existing scroll owner; controls are at least 44 px; the Sheet closes when
selection completes. `mobile-settings-discovery.spec.ts` proves equivalent value and containment.

## Tests

- Pure catalog/search unit tests cover IDs, parents, targets, canonical routes, permissions,
  dynamic encoding, exact/prefix/alias/context ranking, Unicode, and exclusions.
- Target-navigation tests cover initial/delayed/repeated hashes, same/cross-page routing, focus,
  highlight cleanup, history, and reduced motion.
- Existing Settings-tree and command-panel tests cover unchanged normal state, search-only density,
  breadcrumbs, ranking, and stable late registration.

## E2E tests

- `apps/web/e2e/tests/settings/settings-discovery.spec.ts`: desktop tree filtering, exact landing,
  Escape/clear, history, and unsaved-navigation guard.
- `apps/web/e2e/tests/settings/mobile-settings-discovery.spec.ts`: phone Sheet search, exact landing,
  44 px targets, containment, one scroll owner, and no horizontal overflow.
- `apps/web/e2e/tests/command-panel.spec.ts`: search-only absence at rest and exact setting command
  landing after typing.

## Verification results

- Focused unit/integration suite: 12 files and 85 tests passed.
- Broad settings and integration-domain suites passed.
- `pnpm run lint`, `pnpm run typecheck`, `pnpm run i18n:check`, and
  `pnpm run i18n:ratchet` passed.
- Public-doc validation passed after documenting Settings and Cmd+K discovery in Get Started.
- `pnpm run build` completed a production Vite build.
- Desktop Settings discovery E2E: 3 tests passed.
- Mobile Settings discovery E2E: 1 test passed.
- Cmd+K discovery and legacy-alias E2E: 2 tests passed.
- Cmd+K sectioning follow-up: 14 focused component tests and 1 fresh-build browser test passed;
  typecheck, focused ESLint, i18n checks, and the new-code ratchet passed.
- Settings-search polish follow-up: desktop and mobile production-browser tests passed after a
  fresh build; visual checks confirmed compact desktop density and retained 44 px phone targets.
- Settings-result motion follow-up: focused unit coverage and fresh-build desktop/mobile browser
  tests passed; the browser test proves a 160 ms row reflow and an animation-free reduced-motion
  update.

## Implementation waves

- [x] [Task 01 — catalog and search](task-01-catalog-and-search.md)
- [x] [Task 02 — tree and target navigation](task-02-tree-and-target-navigation.md)
- [x] [Task 03 — command palette](task-03-command-palette.md)
- [x] [Task 04 — general and standalone coverage](task-04-general-target-coverage.md)
- [x] [Task 05 — dynamic, integration, system, and account coverage](task-05-domain-target-coverage.md)
- [x] [Task 06 — browser and mobile E2E](task-06-browser-mobile-e2e.md)
- [x] [Task 07 — capitalized dynamic labels](task-07-capitalized-dynamic-labels.md)
- [x] [Task 08 — separate Cmd+K Settings section](task-08-command-palette-settings-group.md)
- [x] [Task 09 — compact Settings search field](task-09-compact-settings-search.md)
- [x] [Task 10 — subtle Settings result motion](task-10-settings-result-motion.md)

All tasks execute sequentially in this conversation. No subagents are authorized.
