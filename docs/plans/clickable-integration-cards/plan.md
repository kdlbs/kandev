---
spec: docs/specs/integrations/requirements/clickable-integration-cards.md
created: 2026-08-21
status: complete
---

# Implementation Plan: Clickable integration cards

## Overview

Update the shared integrations index card composition so each card has one
accessible, full-card `AppLink` to the route it already exposes. Keep the
native switch and plugin action above that navigation layer as independent
interactive controls. Validate the shared component with its unit test and
prove native navigation on both desktop and mobile Playwright projects.

This is a frontend-only change. It does not add an API, state shape, route, or
persistence boundary.

## Frontend

### Native and plugin integration cards

Update `apps/web/components/integrations/integrations-index-page.tsx`:

- Keep the existing `rootHref` and per-card `href` derivation for global and
  workspace-scoped routes.
- Replace the separate title and description links with one full-card `AppLink`
  overlay that has the integration label as its accessible name.
- Keep the visible icon, label, and description as card content while allowing
  pointer events to reach the overlay for all non-control card areas.
- Place the native enable control and plugin `action` in an interactive layer
  above the overlay so toggling or activating them cannot navigate.
- Preserve existing `data-testid` card identifiers, grid geometry, hover state,
  plugin error boundary, and translated description content.

### Unit coverage

Update `apps/web/app/settings/integrations/page.test.tsx` to assert that native
and plugin cards expose the expected accessible link and route, and that the
native switch still drafts its state without calling navigation.

## Backend

None.

## Tests

- **What:** Native and plugin cards expose the existing global and
  workspace-scoped routes through a full-card navigation target, while the
  switch remains non-navigating.
  **File:** `apps/web/app/settings/integrations/page.test.tsx`
  **How:** Render the index with the existing navigation spy, inspect the card
  link `href`, activate it, and separately activate the switch.
- **What:** A pointer click on the native card body navigates from the desktop
  integrations index.
  **File:** `apps/web/e2e/tests/integrations/integrations-index-card-navigation.spec.ts`
  **How:** Open the index, click a card body away from the switch, and assert the
  workspace-scoped integration URL and heading.
- **What:** A touch tap on the native card body navigates from the mobile
  integrations index.
  **File:** `apps/web/e2e/tests/integrations/mobile-integrations-index-card-navigation.spec.ts`
  **How:** Use the configured `mobile-chrome` project, tap a card body, and
  assert the same workspace-scoped route and integration page content.

## E2E Tests

- **Scenario:** Global or workspace index, body click on a native card, route
  changes to that integration settings page.
  **File:** `apps/web/e2e/tests/integrations/integrations-index-card-navigation.spec.ts`
  **What to verify:** The URL includes the active workspace and integration
  slug, and the destination heading is visible.
- **Scenario:** Phone-sized workspace index, body tap on a native card, route
  changes without a title-only interaction.
  **File:** `apps/web/e2e/tests/integrations/mobile-integrations-index-card-navigation.spec.ts`
  **What to verify:** The mobile project reaches the same integration settings
  route and visible page heading.

## Verification Results

Implementation and verification completed.

- Focused integration index unit suite: 12 passed.
- Targeted ESLint for changed frontend and E2E files: passed.
- Web TypeScript check with `NODE_OPTIONS=--max-old-space-size=8192`: passed.
- Desktop blank-body navigation E2E: 1 passed.
- Mobile blank-body navigation E2E: 1 passed.
- Existing desktop enable/disable toggle E2E: 1 passed.

The managed E2E runner rebuilt the backend and production Vite assets for each
project run and completed its fixture setup and teardown successfully.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Card navigation](task-01-card-navigation.md)

Wave 2:

- [x] [Task 02: Navigation E2E coverage](task-02-navigation-e2e.md)

Tasks are sequential because the E2E selectors and route assertions depend on
the final card markup.

## Open Questions

None.
