---
status: draft
system: agents
created: 2026-08-11
owners:
  - kandev
---
# Duplicate an Agent Profile Requirements

## Overview

Profiles are the per-agent configuration unit (model, mode, config options, CLI flags, env vars, launcher prefix, auto-approve). Creating a variant of an existing profile means re-entering every field by hand today: there is no way to start from a working configuration. Users need a one-click "duplicate" that copies a profile's full configuration under a new name, so they can tweak the copy (a different model, an extra CLI flag, a sandbox launcher) without touching the source.

## Requirements

### REQ-AGENTS-PROFILE-DUPLICATE-001: Duplicate an Agent Profile

**Intent:** Profiles are the per-agent configuration unit (model, mode, config options, CLI flags, env vars, launcher prefix, auto-approve). Creating a variant of an existing profile means re-entering every field by hand today: there is no way to start from a working configuration. Users need a one-click "duplicate" that copies a profile's full configuration under a new name, so they can tweak the copy (a different model, an extra CLI flag, a sandbox launcher) without touching the source.

#### Acceptance criteria

- **AC-AGENTS-PROFILE-DUPLICATE-001.1:** Every profile row on the settings agents page (`/settings/agents`, the "Manage existing profiles by agent" list) gets a **Duplicate** action.
- **AC-AGENTS-PROFILE-DUPLICATE-001.2:** The profile settings page (`/settings/agents/<agent>/profiles/<id>`) gets the same **Duplicate** action in its header.
- **AC-AGENTS-PROFILE-DUPLICATE-001.3:** Duplicating creates a new profile that is a faithful copy of the source's configuration: name-derived new name, model, fallback model, auto-fallback, mode, config options, allow-indexing, auto-approve, CLI passthrough, CLI flags, env vars, command prefix, enabled state, and MCP config (enabled flag + servers + meta) when the source has one.
- **AC-AGENTS-PROFILE-DUPLICATE-001.4:** The copy is an independent row: it gets a new ID, `user_modified` is set, and editing or deleting it never affects the source. No session, task, watcher, automation, or routing reference points at the copy.
- **AC-AGENTS-PROFILE-DUPLICATE-001.5:** The copy is committed atomically: the row is inserted with the source's enabled state and the MCP config row is upserted in one repository transaction, so a duplicated profile inherits the source's selection state and a disabled source never becomes briefly selectable. A failure rolls back and leaves no partial copy.
- **AC-AGENTS-PROFILE-DUPLICATE-001.6:** Runtime state is never copied: the copy starts idle with no pause reason, no last-run timestamp, and a zero consecutive-failure count. Office enrichment configuration (workspace, role, budget, permissions, skills, executor preference, …) is copied when present, matching the faithful-copy intent; the settings surface only lists global (non-office) profiles, so in practice those fields are empty on the rows this feature shows.
- **AC-AGENTS-PROFILE-DUPLICATE-001.7:** The generated name is persisted user data, not UI copy: it is `<source name> Copy` (e.g. `Default` → `Default Copy`), matching the repo convention that stored names do not depend on the locale they were created in (same rule as seeded executor and repository names).
- **AC-AGENTS-PROFILE-DUPLICATE-001.8:** **GIVEN** a profile named `Default` with model, CLI flags, env vars, and a command prefix, **WHEN** the user clicks Duplicate on its row in `/settings/agents`, **THEN** a new profile row `Default Copy` appears immediately with every configuration field equal to the source, a distinct ID, and `user_modified` true.

## Migrated source detail

## Why

Profiles are the per-agent configuration unit (model, mode, config options,
CLI flags, env vars, launcher prefix, auto-approve). Creating a variant of an
existing profile means re-entering every field by hand today: there is no way
to start from a working configuration. Users need a one-click "duplicate" that
copies a profile's full configuration under a new name, so they can tweak the
copy (a different model, an extra CLI flag, a sandbox launcher) without
touching the source.

## What

- Every profile row on the settings agents page
  (`/settings/agents`, the "Manage existing profiles by agent" list) gets a
  **Duplicate** action.
- The profile settings page
  (`/settings/agents/<agent>/profiles/<id>`) gets the same **Duplicate**
  action in its header.
- Duplicating creates a new profile that is a faithful copy of the source's
  configuration: name-derived new name, model, fallback model, auto-fallback,
  mode, config options, allow-indexing, auto-approve, CLI passthrough, CLI
  flags, env vars, command prefix, enabled state, and MCP config (enabled
  flag + servers + meta) when the source has one.
- The copy is an independent row: it gets a new ID, `user_modified` is set,
  and editing or deleting it never affects the source. No session, task,
  watcher, automation, or routing reference points at the copy.
- The copy is committed atomically: the row is inserted with the source's
  enabled state and the MCP config row is upserted in one repository
  transaction, so a duplicated profile inherits the source's selection state
  and a disabled source never becomes briefly selectable. A failure rolls
  back and leaves no partial copy.
- Runtime state is never copied: the copy starts idle with no pause reason,
  no last-run timestamp, and a zero consecutive-failure count. Office
  enrichment configuration (workspace, role, budget, permissions, skills,
  executor preference, …) is copied when present, matching the faithful-copy
  intent; the settings surface only lists global (non-office) profiles, so in
  practice those fields are empty on the rows this feature shows.
- The generated name is persisted user data, not UI copy: it is
  `<source name> Copy` (e.g. `Default` → `Default Copy`), matching the
  repo convention that stored names do not depend on the locale they were
  created in (same rule as seeded executor and repository names).

## API surface

`POST /api/v1/agent-profiles/:id/duplicate` (interim-settings-interlock
protected, like every other profile mutation):

- Request body: none. The copy name is derived server-side as
  `<source name> Copy`.
- Response `200`: the new `AgentProfileDTO` (same shape as
  `POST /agents/:id/profiles`).
- `404` when the source profile does not exist or is soft-deleted.
- Broadcasts the existing `agent.profile.created` WebSocket notification so
  every open settings surface picks the copy up live.

The endpoint is not idempotent in the HTTP sense: each call creates one new
row (there is no idempotency key), so a retried POST after a network timeout
may create a second copy. Callers must treat duplicates as one-shot
operations; the UI guards against double-clicks. The copy performs no
reference checks because a brand-new row cannot be referenced by anything
yet.

## Scenarios

- **GIVEN** a profile named `Default` with model, CLI flags, env vars, and a
  command prefix, **WHEN** the user clicks Duplicate on its row in
  `/settings/agents`, **THEN** a new profile row `Default Copy` appears
  immediately with every configuration field equal to the source, a distinct
  ID, and `user_modified` true.
- **GIVEN** an MCP-capable agent profile with an MCP config (enabled with
  servers), **WHEN** it is duplicated, **THEN** the copy has the same MCP
  enabled state, servers, and meta under the new profile ID.
- **GIVEN** a disabled profile, **WHEN** it is duplicated, **THEN** the copy
  is also disabled.
- **GIVEN** a profile with an active session, **WHEN** it is duplicated,
  **THEN** the duplicate succeeds (unlike delete, no in-use checks apply) and
  the session keeps using the source.
- **GIVEN** the profile settings page of a profile, **WHEN** the user clicks
  Duplicate in the header, **THEN** a toast confirms the copy and the user is
  taken to the copy's settings page to edit it.
- **GIVEN** an unknown profile ID, **WHEN** the duplicate endpoint is called,
  **THEN** it returns `404` and creates nothing.

## Out of scope

- Duplicating office workspace agents through the office UI (`/office/agents`):
  office has its own agent creation flows and permission model. This feature
  targets the `/settings/agents` profile flavor. The settings duplicate
  endpoint is fail-closed against office-scoped sources: a profile with a
  non-empty `workspace_id` returns `404` (existence hidden) before any MCP
  read or write, so the instance-level settings surface can never read or
  clone another workspace's configuration. Office duplication, when added,
  belongs on the workspace-scoped office API surface.
- Renaming after duplicate: the copy is a normal profile the user renames via
  the existing profile editor.
- Bulk/queue duplication, templates, or copy-paste between agents.
- Choosing the copy name in the duplicate dialog: name is derived, then edited
  on the copy's settings page.
