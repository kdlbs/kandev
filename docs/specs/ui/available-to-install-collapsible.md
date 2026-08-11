---
status: shipped
created: 2026-08-11
owner: kandev
---

# Available to Install section collapsible

## Why

The Agents settings page lists every discovered-but-not-installed agent and tool under "Available to Install", each with an install card that streams live progress. For hosts with many available agents, the section pushes Installed Agents and Agent Profiles far down the page, and its heading plus description consume vertical space even when the user only wants to see the grid once or hide it while installs run.

## What

- The "Browse available agents" heading row on `/settings/agents/browse` (the page that lists every discovered-but-not-installed agent and tool, formerly the "Available to Install" section on `/settings/agents`) is a control that toggles the section's card grid between expanded and collapsed.
- The section renders expanded by default on first visit.
- When collapsed, the install cards and tool cards are removed from view; the heading row (title, description, and a chevron that rotates when expanded) remains visible.
- Install jobs started before collapsing keep streaming and completing server-side; a successful install still moves the agent under "Installed Agents" via the existing rescan, whether or not the section is expanded at that moment.
- The collapsed state is local to the page session and resets on reload or navigation away and back.
- The control is keyboard accessible (button semantics with toggling `aria-expanded`), and the whole heading row is a touch target.

## Scenarios

- **GIVEN** the agents browse page with at least one installable agent or tool, **WHEN** the page loads, **THEN** the section's card grid is visible and the chevron points down (expanded state).
- **GIVEN** the section expanded, **WHEN** the user clicks or taps the "Browse available agents" heading row, **THEN** the card grid is hidden and the chevron indicates the collapsed state.
- **GIVEN** the section collapsed, **WHEN** the user clicks or taps the heading row again, **THEN** the card grid reappears.
- **GIVEN** an install job streaming output, **WHEN** the user collapses the section, **THEN** the install continues and the agent appears under "Installed Agents" after the job succeeds.
- **GIVEN** a phone-sized viewport, **WHEN** the user taps the heading row, **THEN** the section collapses and expands without introducing document-level horizontal overflow.

## Out of scope

- Persisting the collapsed/expanded choice across reloads or as a user setting.
- Changing install, streaming, or rescan behavior.
- Applying the collapsible treatment to the Installed Agents or Agent Profiles sections.
- Changing the section's copy or card layout.
