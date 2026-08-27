---
status: draft
system: agents
created: 2026-08-13
owners:
  - kandev
---
# Collapsible Agent Blocks on the Agents Settings Page Requirements

## Overview

The Settings → Agents page renders one card per installed agent (Codex, Claude, etc.), and each card contains the agent's full profile list. Agents with dozens of profiles make the page very long: to reach a lower agent a user has to scroll past every profile of every agent above it. Users need to compress the blocks they are not currently working with, while keeping each block's profile count visible.

## Requirements

### REQ-AGENTS-COLLAPSIBLE-AGENT-BLOCKS-001: Collapsible Agent Blocks on the Agents Settings Page

**Intent:** The Settings → Agents page renders one card per installed agent (Codex, Claude, etc.), and each card contains the agent's full profile list. Agents with dozens of profiles make the page very long: to reach a lower agent a user has to scroll past every profile of every agent above it. Users need to compress the blocks they are not currently working with, while keeping each block's profile count visible.

#### Acceptance criteria

- **AC-AGENTS-COLLAPSIBLE-AGENT-BLOCKS-001.1:** Each installed-agent card on `/settings/agents` SHALL render a collapse control in its header.
- **AC-AGENTS-COLLAPSIBLE-AGENT-BLOCKS-001.2:** All agent cards SHALL be expanded by default on first visit (or when no stored preference exists).
- **AC-AGENTS-COLLAPSIBLE-AGENT-BLOCKS-001.3:** Activating the collapse control SHALL hide that card's profile-list body and SHALL show the profile count in the card header instead ("N profiles" via the existing `agents:profileCount` key, or the short `agents:noProfilesYetShort` label "No profiles yet" when the agent has zero profiles — the full-sentence empty-state copy belongs to the expanded body, not a header chip). The count stays visible while the block is collapsed.
- **AC-AGENTS-COLLAPSIBLE-AGENT-BLOCKS-001.4:** Activating the control again SHALL re-expand the block and restore the profile list.
- **AC-AGENTS-COLLAPSIBLE-AGENT-BLOCKS-001.5:** The collapsed/expanded state is stored per agent name and per browser (`localStorage`), survives page reloads and app restarts, and applies only to the agent it was toggled for. Collapsing one agent never changes any other agent's state.
- **AC-AGENTS-COLLAPSIBLE-AGENT-BLOCKS-001.6:** The collapse state is frontend-only. No backend, API, or database change.
- **AC-AGENTS-COLLAPSIBLE-AGENT-BLOCKS-001.7:** **GIVEN** a fresh browser visiting `/settings/agents` with two installed agents that each have profiles, **WHEN** the page loads, **THEN** both profile lists are visible and no collapse state is stored.
- **AC-AGENTS-COLLAPSIBLE-AGENT-BLOCKS-001.8:** **GIVEN** an expanded agent card with 3 profiles, **WHEN** the user activates its collapse control, **THEN** the profile list disappears and the header shows "3 profiles".

## Migrated source detail

## Why

The Settings → Agents page renders one card per installed agent (Codex, Claude,
etc.), and each card contains the agent's full profile list. Agents with dozens
of profiles make the page very long: to reach a lower agent a user has to scroll
past every profile of every agent above it. Users need to compress the blocks
they are not currently working with, while keeping each block's profile count
visible.

## What

- Each installed-agent card on `/settings/agents` SHALL render a collapse
  control in its header.
- All agent cards SHALL be expanded by default on first visit (or when no
  stored preference exists).
- Activating the collapse control SHALL hide that card's profile-list body and
  SHALL show the profile count in the card header instead ("N profiles" via the
  existing `agents:profileCount` key, or the short `agents:noProfilesYetShort`
  label "No profiles yet" when the agent has zero profiles — the full-sentence
  empty-state copy belongs to the expanded body, not a header chip). The count
  stays visible while the block is collapsed.
- Activating the control again SHALL re-expand the block and restore the
  profile list.
- The collapsed/expanded state is stored per agent name and per browser
  (`localStorage`), survives page reloads and app restarts, and applies only to
  the agent it was toggled for. Collapsing one agent never changes any other
  agent's state.
- The collapse state is frontend-only. No backend, API, or database change.

## Data model

No backend state. One `localStorage` record under the key
`kandev:agents:collapsedBlocks:v1`, a JSON object mapping agent name to a
boolean:

```
{
  "claude": true,
  "codex": true
}
```

- Absent key, empty object, or missing agent entry → that agent is expanded
  (default).
- Invalid JSON, or a read error (quota / private mode) → treated as the default
  (all expanded); the page keeps working and never crashes.
- A failed write still applies the toggle for the current session (an in-memory
  override keeps the card in the state the user chose, so a collapsed card can
  always be expanded again and vice versa), but the preference is not persisted
  and the write error is contained at the UI boundary — never surfaced to the
  user. The next successful write makes storage authoritative again.

## Scenarios

- **GIVEN** a fresh browser visiting `/settings/agents` with two installed
  agents that each have profiles, **WHEN** the page loads, **THEN** both
  profile lists are visible and no collapse state is stored.
- **GIVEN** an expanded agent card with 3 profiles, **WHEN** the user activates
  its collapse control, **THEN** the profile list disappears and the header
  shows "3 profiles".
- **GIVEN** a collapsed card, **WHEN** the user reloads the page, **THEN** the
  card is still collapsed and the header still shows the count.
- **GIVEN** a collapsed card, **WHEN** the user activates the expand control,
  **THEN** the profile list reappears and the header no longer shows the count
  (it returns to the body's first line).
- **GIVEN** two agent cards, **WHEN** the user collapses only one, **THEN** the
  other stays expanded, and after a reload each card keeps its own state.
- **GIVEN** an agent with zero profiles, **WHEN** its card is collapsed, **THEN**
  the header shows "No profiles yet".
- **GIVEN** localStorage is unavailable (quota / private mode), **WHEN** the
  user toggles a card, **THEN** the toggle still works for the session
  (collapse and expand both apply in memory), no error is surfaced, and the
  choice simply is not persisted.

## Out of scope

- No backend persistence, no per-workspace or per-user server sync of collapse
  state.
- No "collapse all" / "expand all" bulk controls.
- No change to the agent detail page (`/settings/agents/[agentId]`); its own
  profile list is unchanged.
