---
status: draft
system: agents
created: 2026-08-01
owners:
  - kandev
---
# Disable an Agent Profile Requirements

## Overview

A profile sometimes needs to be taken out of rotation — a model is being retired, an agent CLI is misbehaving, or a workspace is migrating. Today the only way to stop a profile being picked for new work is to delete it, which blocks on active sessions and destroys its configuration. Users need a reversible "off switch" that hides the profile from new-task selection while leaving existing sessions untouched.

## Requirements

### REQ-AGENTS-PROFILE-DISABLE-001: Disable an Agent Profile

**Intent:** A profile sometimes needs to be taken out of rotation — a model is being retired, an agent CLI is misbehaving, or a workspace is migrating. Today the only way to stop a profile being picked for new work is to delete it, which blocks on active sessions and destroys its configuration. Users need a reversible "off switch" that hides the profile from new-task selection while leaving existing sessions untouched.

#### Acceptance criteria

- **AC-AGENTS-PROFILE-DISABLE-001.1:** An agent profile has an `enabled` state, defaulting to enabled.
- **AC-AGENTS-PROFILE-DISABLE-001.2:** The profile settings page (`/settings/agents/<agent>/profiles/<id>`) shows an **Enabled** toggle in the page header. Toggling it is part of the normal profile save flow (dirty state, save button, same persistence as other profile fields).
- **AC-AGENTS-PROFILE-DISABLE-001.3:** The agent profiles list on `/settings/agents` (under "Manage existing profiles by agent") shows the same toggle per profile row. Toggling it saves immediately and updates the row without navigating.
- **AC-AGENTS-PROFILE-DISABLE-001.4:** A disabled profile is not offered as a choice when creating a new task, a new subtask, a new session on an existing task, a handoff session from an existing session, or a quick chat.
- **AC-AGENTS-PROFILE-DISABLE-001.5:** A disabled profile is still listed in settings (with its toggle visible), still editable, and still shown as the label of any existing session that uses it. Existing sessions (running or not) are unaffected: they keep their profile and can still be resumed.
- **AC-AGENTS-PROFILE-DISABLE-001.6:** Disabling a profile is always allowed — unlike deleting, it never conflicts on active sessions, watchers, or routing tiers.
- **AC-AGENTS-PROFILE-DISABLE-001.7:** Pre-selection defaults (last-used profile, workspace default, workflow default, first compatible profile) never resolve to a disabled profile when a new task/session dialog opens; the dialog falls back to an enabled candidate. A workflow that locks a disabled profile shows no compatible agent until the profile is re-enabled or the workflow is changed.
- **AC-AGENTS-PROFILE-DISABLE-001.8:** **GIVEN** an enabled profile, **WHEN** the user turns off the Enabled toggle in the profile settings header and saves, **THEN** the profile's `enabled` is `false`, the header shows the toggle off, and the save persists across reload.

## Migrated source detail

## Why

A profile sometimes needs to be taken out of rotation — a model is being
retired, an agent CLI is misbehaving, or a workspace is migrating. Today the
only way to stop a profile being picked for new work is to delete it, which
blocks on active sessions and destroys its configuration. Users need a
reversible "off switch" that hides the profile from new-task selection while
leaving existing sessions untouched.

## What

- An agent profile has an `enabled` state, defaulting to enabled.
- The profile settings page (`/settings/agents/<agent>/profiles/<id>`) shows an
  **Enabled** toggle in the page header. Toggling it is part of the normal
  profile save flow (dirty state, save button, same persistence as other
  profile fields).
- The agent profiles list on `/settings/agents` (under "Manage existing
  profiles by agent") shows the same toggle per profile row. Toggling it
  saves immediately and updates the row without navigating.
- A disabled profile is not offered as a choice when creating a new task, a
  new subtask, a new session on an existing task, a handoff session from an
  existing session, or a quick chat.
- A disabled profile is still listed in settings (with its toggle visible),
  still editable, and still shown as the label of any existing session that
  uses it. Existing sessions (running or not) are unaffected: they keep their
  profile and can still be resumed.
- Disabling a profile is always allowed — unlike deleting, it never conflicts
  on active sessions, watchers, or routing tiers.
- Pre-selection defaults (last-used profile, workspace default, workflow
  default, first compatible profile) never resolve to a disabled profile when
  a new task/session dialog opens; the dialog falls back to an enabled
  candidate. A workflow that locks a disabled profile shows no compatible
  agent until the profile is re-enabled or the workflow is changed.

## Data model

`agent_profiles` table (SQLite, `apps/backend/internal/agent/settings/store`):

```sql
enabled  INTEGER NOT NULL DEFAULT 1   -- 1 = selectable for new work, 0 = hidden from selection
```

Existing rows backfill to `1` via the column default. The column is
independent of `deleted_at` (soft delete) and of the office `status` column
(`idle`/`working`/...) — disabling is a kanban-flavour selection flag, not a
lifecycle state.

## API surface

`PATCH /api/v1/agent-profiles/:id` accepts an optional `enabled` boolean in
the request body and returns the updated profile including `enabled`. All
profile listing responses (`GET /api/v1/agents`, `GET
/api/v1/agent-profiles/:id`) include `enabled` on each profile, `true` when
unset for backward compatibility. Disabled profiles remain present in every
listing endpoint — filtering happens only where profiles are offered as
selection choices.

## Scenarios

- **GIVEN** an enabled profile, **WHEN** the user turns off the Enabled toggle
  in the profile settings header and saves, **THEN** the profile's `enabled`
  is `false`, the header shows the toggle off, and the save persists across
  reload.
- **GIVEN** a profile with `enabled: false`, **WHEN** the user opens the new
  task dialog, **THEN** the agent profile selector does not list it.
- **GIVEN** a profile with `enabled: false`, **WHEN** the user opens the new
  session / new subtask / handoff / quick-chat selectors, **THEN** the
  selector does not list it.
- **GIVEN** a profile with `enabled: false`, **WHEN** the user opens
  `/settings/agents`, **THEN** the profile still appears under "Manage
  existing profiles by agent" with its Enabled toggle off, and toggling it on
  makes it selectable again without a page reload.
- **GIVEN** a profile with `enabled: false`, **WHEN** an existing session that
  uses the profile resumes, **THEN** the session starts with that profile and
  its label still renders.
- **GIVEN** a profile with `enabled: false` that is the workspace default,
  **WHEN** the new task dialog autopicks a profile, **THEN** the dialog
  selects an enabled profile instead.
- **GIVEN** a profile with `enabled: false` that is locked by a workflow,
  **WHEN** the new task dialog opens for that workflow, **THEN** the selector
  does not pre-select it and task creation is gated on choosing an enabled
  profile.
- **GIVEN** a profile with `enabled: false` and active sessions, **WHEN** the
  user toggles it back on, **THEN** it becomes selectable immediately with no
  session-side effects.

## Out of scope

- Office workspace agents (`/office` routes, `office.agentProfiles` slice).
  Office rows are separate workspace-scoped profiles with their own
  `status`/pause semantics; this toggle applies to the
  `/settings/agents` profile flavor.
- Enforcing `enabled` on the backend task/session start endpoints: an API
  caller that explicitly passes a disabled profile ID can still start a
  session. Only selection surfaces hide it.
- Renaming, reordering, or styling changes to profile pickers beyond hiding
  disabled entries.
