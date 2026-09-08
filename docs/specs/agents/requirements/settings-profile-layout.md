---
status: active
system: agents
created: 2026-08-13
owners:
  - kandev
---
# Simplify the agent settings profile layout Requirements

## Overview

The agent settings page repeats the profile count and profile creation action in a separate sub-header inside every agent card. This adds a second hierarchy between the agent identity and its profiles and makes the primary action harder to associate with the agent it changes.

## Requirements

### REQ-AGENTS-SETTINGS-PROFILE-LAYOUT-001: Simplify the agent settings profile layout

**Intent:** The agent settings page repeats the profile count and profile creation action in a separate sub-header inside every agent card. This adds a second hierarchy between the agent identity and its profiles and makes the primary action harder to associate with the agent it changes.

#### Acceptance criteria

- **AC-AGENTS-SETTINGS-PROFILE-LAYOUT-001.1:** On `/settings/agents`, each configured agent card exposes its existing **New profile** action in the card header, on the right with the other agent-level actions. The action continues to open that agent's profile creation flow at `/settings/agents/<agent>?mode=create`.
- **AC-AGENTS-SETTINGS-PROFILE-LAYOUT-001.2:** An agent without a saved profile keeps one **Setup profile** action in its card header. It does not show a second profile-creation action in an empty profile area.
- **AC-AGENTS-SETTINGS-PROFILE-LAYOUT-001.3:** The profile list body no longer renders a profile-count or empty-state header row. When profiles exist, the saved profile rows appear directly in the card body. When no profiles exist, the card header remains the only setup entry point.
- **AC-AGENTS-SETTINGS-PROFILE-LAYOUT-001.4:** The installed-agent section keeps its existing terminal and refresh/rescan actions. The refresh/rescan action appears immediately before the existing agent-creation action, and the agent-creation action is the rightmost action in that toolbar. The current agent-creation behavior and translated label remain unchanged.
- **AC-AGENTS-SETTINGS-PROFILE-LAYOUT-001.5:** Existing profile rows, profile links, duplicate/delete actions, agent status badges, authentication controls, and runtime-update controls keep their current behavior.
- **AC-AGENTS-SETTINGS-PROFILE-LAYOUT-001.6:** On a full desktop viewport (at least 1024px wide), each profile row shows compact icon-only **Duplicate** and **Delete** action buttons directly in the row action area. The buttons retain translated accessible names, are small and vertically centered. Hovering or focusing either button shows a tooltip with its translated action name, and the row does not show the three-dots profile-actions trigger at this width.
- **AC-AGENTS-SETTINGS-PROFILE-LAYOUT-001.7:** Below the full desktop boundary, including compact desktop, tablet, and mobile viewports, each profile row keeps the three-dots profile-actions trigger and exposes **Duplicate** and **Delete** in its overflow menu. Inline action buttons are not rendered in these viewports.
- **AC-AGENTS-SETTINGS-PROFILE-LAYOUT-001.8:** The desktop and mobile layouts expose the same actions and outcomes. On a narrow touch viewport, card-header actions remain visible, reachable with a touch target of at least 44px, and do not create document-level horizontal overflow. The settings content keeps its existing scroll owner.

## Migrated source detail

## Why

The agent settings page repeats the profile count and profile creation action
in a separate sub-header inside every agent card. This adds a second hierarchy
between the agent identity and its profiles and makes the primary action harder
to associate with the agent it changes.

## What

- On `/settings/agents`, each configured agent card exposes its existing **New
  profile** action in the card header, on the right with the other agent-level
  actions. The action continues to open that agent's profile creation flow at
  `/settings/agents/<agent>?mode=create`.
- An agent without a saved profile keeps one **Setup profile** action in its
  card header. It does not show a second profile-creation action in an empty
  profile area.
- The profile list body no longer renders a profile-count or empty-state
  header row. When profiles exist, the saved profile rows appear directly in
  the card body. When no profiles exist, the card header remains the only
  setup entry point.
- The installed-agent section keeps its existing terminal and refresh/rescan
  actions. The refresh/rescan action appears immediately before the existing
  agent-creation action, and the agent-creation action is the rightmost action
  in that toolbar. The current agent-creation behavior and translated label
  remain unchanged.
- Existing profile rows, profile links, duplicate/delete actions, agent status
  badges, authentication controls, and runtime-update controls keep their
  current behavior.
- On a full desktop viewport (at least 1024px wide), each profile row shows
  compact icon-only **Duplicate** and **Delete** action buttons directly in the
  row action area. The buttons retain translated accessible names, are small
  and vertically centered. Hovering or focusing either button shows a tooltip
  with its translated action name, and the row does not show the three-dots
  profile-actions trigger at this width.
- Below the full desktop boundary, including compact desktop, tablet, and
  mobile viewports, each profile row keeps the three-dots profile-actions
  trigger and exposes **Duplicate** and **Delete** in its overflow menu. Inline
  action buttons are not rendered in these viewports.
- The desktop and mobile layouts expose the same actions and outcomes. On a
  narrow touch viewport, card-header actions remain visible, reachable with a
  touch target of at least 44px, and do not create document-level horizontal
  overflow. The settings content keeps its existing scroll owner.

## Scenarios

- **GIVEN** a configured agent with one or more profiles, **WHEN** the user
  opens `/settings/agents`, **THEN** the agent card header shows **New
  profile** on the right, the profile rows appear directly below it, and no
  profile-count text or separate count/action row is rendered.
- **GIVEN** an agent with no saved profile, **WHEN** the user opens
  `/settings/agents`, **THEN** the card header shows **Setup profile**, no
  profile-count or empty-state action row is rendered, and clicking the action
  opens that agent's profile creation flow.
- **GIVEN** the installed-agent section is visible, **WHEN** the user reads
  its right-side toolbar, **THEN** refresh/rescan is immediately followed by
  the agent-creation action, which is the rightmost action, while the terminal
  action remains available.
- **GIVEN** a configured agent card, **WHEN** the user clicks **New profile**,
  **THEN** the browser navigates to that agent's profile creation route and
  the existing profile rows remain unchanged.
- **GIVEN** a profile row in a viewport at least 1024px wide, **WHEN** the user
  views the row, **THEN** compact icon-only **Duplicate** and **Delete** buttons
  with translated accessible names are visible inline, vertically centered,
  and the three-dots profile-actions trigger is absent. Hovering or focusing an
  inline button shows a tooltip with the matching translated action name.
- **GIVEN** a profile row in a viewport narrower than 1024px, **WHEN** the user
  opens the profile-actions trigger, **THEN** the overflow menu contains
  **Duplicate** and **Delete**, and no inline action buttons are rendered.
- **GIVEN** a profile row with inline actions or overflow actions, **WHEN** the
  user activates **Duplicate** or confirms **Delete**, **THEN** the existing
  duplicate or delete behavior completes without changing the profile link or
  other row metadata.
- **GIVEN** a phone viewport below 768px, **WHEN** the user opens the agents
  settings page, **THEN** the card-header creation action and the section
  toolbar actions remain visible and touch-reachable, the profile rows remain
  vertically usable, and `document.documentElement.scrollWidth` does not
  exceed `window.innerWidth`.

## Out of scope

- Changing the profile editor, agent discovery, agent installation, or the
  backend/API contracts.
- Changing the duplicate or delete operation, confirmation flow, profile link,
  or store synchronization behavior.
- Renaming the existing translated **New profile**, **Setup profile**, refresh,
  or agent-creation labels.
- Changing the profile count text used by unrelated task or picker surfaces.
- Changing the settings sidebar hierarchy or the `/office/agents` surface.
