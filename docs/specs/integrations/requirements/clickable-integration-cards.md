---
status: active
system: integrations
created: 2026-08-21
owners:
  - Codex
---
# Clickable integration cards Requirements

## Overview

The integrations index currently makes only the title and description links navigation targets. Users expect the card itself to be the obvious entry point, especially on a touch viewport, so requiring a precise tap on text makes the settings page harder to use.

## Requirements

### REQ-INTEGRATIONS-CLICKABLE-INTEGRATION-CARDS-001: Clickable integration cards

**Intent:** The integrations index currently makes only the title and description links navigation targets. Users expect the card itself to be the obvious entry point, especially on a touch viewport, so requiring a precise tap on text makes the settings page harder to use.

#### Acceptance criteria

- **AC-INTEGRATIONS-CLICKABLE-INTEGRATION-CARDS-001.1:** Every native integration card on the global and workspace-scoped integrations index exposes the existing integration route as a full-card navigation target.
- **AC-INTEGRATIONS-CLICKABLE-INTEGRATION-CARDS-001.2:** Every plugin integration card uses the same full-card navigation behavior and preserves its existing workspace-scoped route when one is present.
- **AC-INTEGRATIONS-CLICKABLE-INTEGRATION-CARDS-001.3:** Clicking or tapping any card area that is not an interactive secondary control opens that integration's settings page through the existing in-app navigation.
- **AC-INTEGRATIONS-CLICKABLE-INTEGRATION-CARDS-001.4:** The native enable/disable switch and any plugin card action remain independent controls. Activating either control does not navigate to the integration page.
- **AC-INTEGRATIONS-CLICKABLE-INTEGRATION-CARDS-001.5:** The card navigation target has an accessible integration name and remains usable with keyboard focus and activation.
- **AC-INTEGRATIONS-CLICKABLE-INTEGRATION-CARDS-001.6:** Desktop and mobile retain the same route and control semantics. Mobile uses the existing single-column integrations index and does not add an intermediate drawer or another navigation step.
- **AC-INTEGRATIONS-CLICKABLE-INTEGRATION-CARDS-001.7:** **GIVEN** the global integrations index, **WHEN** the user clicks the body of a native Azure DevOps card outside its switch, **THEN** the app navigates to `/settings/integrations/azure-devops` through the normal SPA route transition.
- **AC-INTEGRATIONS-CLICKABLE-INTEGRATION-CARDS-001.8:** **GIVEN** a workspace-scoped integrations index, **WHEN** the user clicks the body of a native card outside its switch, **THEN** the app navigates to the matching `/settings/workspaces/<encoded-workspace-id>/integrations/<slug>` route.

## Migrated source detail

## Why

The integrations index currently makes only the title and description links
navigation targets. Users expect the card itself to be the obvious entry point,
especially on a touch viewport, so requiring a precise tap on text makes the
settings page harder to use.

## What

- Every native integration card on the global and workspace-scoped integrations
  index exposes the existing integration route as a full-card navigation target.
- Every plugin integration card uses the same full-card navigation behavior and
  preserves its existing workspace-scoped route when one is present.
- Clicking or tapping any card area that is not an interactive secondary control
  opens that integration's settings page through the existing in-app navigation.
- The native enable/disable switch and any plugin card action remain independent
  controls. Activating either control does not navigate to the integration page.
- The card navigation target has an accessible integration name and remains
  usable with keyboard focus and activation.
- Desktop and mobile retain the same route and control semantics. Mobile uses
  the existing single-column integrations index and does not add an intermediate
  drawer or another navigation step.

## Scenarios

- **GIVEN** the global integrations index, **WHEN** the user clicks the body of
  a native Azure DevOps card outside its switch, **THEN** the app navigates to
  `/settings/integrations/azure-devops` through the normal SPA route transition.
- **GIVEN** a workspace-scoped integrations index, **WHEN** the user clicks the
  body of a native card outside its switch, **THEN** the app navigates to the
  matching `/settings/workspaces/<encoded-workspace-id>/integrations/<slug>`
  route.
- **GIVEN** a plugin integration card, **WHEN** the user clicks its body outside
  the plugin action, **THEN** the app navigates to that plugin's existing
  integration settings route.
- **GIVEN** an integration card with an enabled or disabled switch, **WHEN** the
  user activates the switch, **THEN** the switch changes or drafts its state and
  the browser remains on the integrations index.
- **GIVEN** the integrations index on a phone-sized viewport, **WHEN** the user
  taps the body of a card outside its switch, **THEN** the app navigates to the
  same integration settings route without requiring a title or description tap.
- **GIVEN** a focused card navigation target, **WHEN** the user presses Enter,
  **THEN** the app opens the same integration settings route as a pointer or
  touch activation.

## Out of scope

- Changing integration routes, workspace selection, enable/disable persistence,
  plugin registration, or integration page content.
- Making the switch or plugin action navigate when its own control is activated.
- Adding a new mobile navigation surface or changing the integrations grid.
