---
id: "03-repository-discovery-ux"
title: "Update repository discovery user experience"
status: completed
wave: 3
depends_on:
  - "01-desktop-folder-selection-boundary"
  - "02-runtime-aware-discovery-cache"
plan: "plan.md"
requirements:
  - "REQ-WORKSPACES-LOCAL-REPOSITORIES-002"
  - "REQ-WORKSPACES-LOCAL-REPOSITORIES-003"
acceptance_criteria:
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-002.3"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-002.4"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-002.5"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-002.11"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-002.12"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-002.13"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-002.15"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-003.2"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-003.3"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-003.4"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-003.5"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-003.7"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-003.8"
system_design:
  - "../../specs/workspaces/system-design/local-repositories.md"
---

# Task 03: Update Repository Discovery User Experience

## Summary

Move all repository-selection consumers to one discovery hook and activation
coordinator. Select the folder interface from client capability.

## In scope

- Add per-tab activation leases and document-visibility handling.
- Add selection, confirmation, refresh, reconnect, and removal actions.
- Use the native adapter in Tauri and the HTTP browser in ordinary browsers.
- Migrate all current repository-selection consumers.
- Add five-locale copy, desktop-viewport web E2E, and mobile web E2E.
- Reuse the Add Workspace Sources `Drawer` for the phone composition.

## Out of scope

- Automate the native macOS picker or privacy dialog in Playwright.
- Add a general desktop filesystem browser.
- Duplicate discovery state for mobile presentation.

## Acceptance

- Unit tests prove that one coordinator owns leases, stale refresh, and tab visibility.
- Stubbed-picker E2E proves desktop selection, cache, migration, and recovery flows.
- Mobile E2E proves the same outcome in the existing drawer with no horizontal overflow.

## Verification

```bash
cd apps/web && rtk pnpm exec vitest run components/task-create-dialog-effects.test.ts app/office/projects components/task/add-workspace-sources components/automations
cd apps/web && rtk pnpm run typecheck
cd apps/web && rtk pnpm run i18n:check
cd apps/web && rtk pnpm e2e:run tests/task/repository-discovery-consent.spec.ts
cd apps/web && rtk pnpm e2e:run --project mobile-chrome tests/task/mobile-repository-discovery.spec.ts
```

Inspect a phone screenshot. Check a single internal scroll owner, safe-area
clearance, 44-pixel actions, focus return, and document-width containment.

## Files likely touched

- `apps/web/components/task-create-dialog-effects.ts`
- New shared workspace discovery hook and activation coordinator
- `apps/web/components/task/add-workspace-sources/`
- `apps/web/app/settings/workspace/workspace-repositories-client.tsx`
- `apps/web/app/office/projects/`
- `apps/web/components/automations/config-section.tsx`
- `apps/web/components/folder-picker.tsx`
- Workspace API clients, unit tests, E2E tests, and locale catalogs

## Dependencies

- Task 01 supplies the native adapter.
- Task 02 supplies root and snapshot APIs.

## Risks

- Independent visibility effects can recreate background scans.
- A browser on a desktop backend can receive the wrong picker if policy and
  client capability use one flag.
- A phone layout can lose actions if it compresses the desktop dialog.

## Parallelism

`sequential`

## Inputs

- REQ-WORKSPACES-LOCAL-REPOSITORIES-002 and 003.
- Add Workspace Sources mobile `Drawer` as the nearest shipped exemplar.
- Current task, settings, automations, and Office discovery consumers.

## Results

Completed.

- Added the per-tab lease and visibility coordinator, migrated task, settings,
  Office, automation, and workspace-source consumers, and exposed manual
  refresh and recovery controls.
- Added the native Tauri folder picker boundary, browser HTTP picker behavior,
  desktop root controls, responsive mobile composition, and all locale keys.
- Verification passed: focused Vitest tests (45 tests), web typecheck, web
  lint, i18n checks, and the desktop Chromium and mobile Chromium discovery E2E
  tests against rebuilt backend and web artifacts.
