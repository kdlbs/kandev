---
status: active
system: integrations
created: 2026-11-20
owners:
  - kandev
---

# Prompt Sync Local Versions Requirements

## Overview

[Prompt Sync](prompt-sync.md) makes a repository directory the source of truth
for saved prompts. This document covers the prompts that live beside that
source: a local override of a synced prompt, a prompt whose name a repository
file also uses, the local version kept while the repository version is in
use, and what agents may do with synced prompts.

The contract is separate from reconciliation because its actor is the person
choosing between two versions of one prompt, and its lifecycle (override,
conflict, revert, restore) outlives individual sync runs. Reconciliation in
[Prompt Sync](prompt-sync.md) applies these rules when a run meets such a
prompt.

## Terminology

- **Override:** a local, editable prompt that replaces a synced prompt of the
  same name; sync leaves its content alone.
- **Conflict:** a valid prompt file whose name a manual prompt, a built-in
  prompt, or a still-conflicted override already uses.
- **Other version:** the version a prompt does not use now, stored with it:
  the kept local version of a synced prompt, or the repository version of an
  override.

## Requirements

### REQ-INTEGRATIONS-PROMPT-SYNC-007: Local overrides

**Intent:** A shared repository can ship a default prompt at a chosen point
(for example `@post-pr-review`) that one installation replaces with its own
version, and can return to the shared default later.

**User story:** As an operator syncing a shared prompt repository, I want to
override one synced prompt locally, so that a shared workflow step uses my
version without forking the repository.

#### Acceptance criteria

- **AC-INTEGRATIONS-PROMPT-SYNC-007.1:** Settings > Prompts shall show, on
  each synced prompt and each override, the same version control as the
  review step (AC-INTEGRATIONS-PROMPT-SYNC-008.2): "Local" and "Repository"
  labels on either side of one switch, on for the repository version, with
  the accessible name "Use the repository version of @name". It shall be the
  first of the row's trailing controls in every row state, visually distinct
  from edit and delete, with a touch-sized hit area on phones and coarse
  pointers. Changing it shall be a pending edit like changing a prompt's
  text, marked on the row as unsaved, and the page's save bar appears.
  Changing it shall never open or close the row. A row on Repository shall
  show and hide its repository version read-only, with the file where it is
  edited, from a View (eye) control; a row on Local shall show and hide its
  local version, editable at once, from **Edit**; there shall be no
  read-only view of the local version. An open row switched to Local shall
  therefore become editable without a save. Any number of prompts, synced,
  overridden or manual, may be switched and edited before one **Save**,
  which writes each prompt's choice together with any edit to it, as one
  atomic write per prompt; every prompt with an unsaved switch or edit shall
  be marked unsaved until saved or reset, including while its row is
  closed. **Reset** shall restore every pending switch and edit, and leaving
  the page shall warn about them like any other unsaved edit. When a prompt
  with an unsaved edit leaves the list (for example, a sync removes its
  file), its text shall be kept: the page shall name it, **Save** shall
  create it as a new prompt, and **Reset** shall discard it. The prompt's
  name is owned by its repository file and shall not be editable. Saving
  Local shall turn
  the prompt into an override with the same name and identity, starting
  from its kept local content (AC-INTEGRATIONS-PROMPT-SYNC-009.2) or else its
  current content, editable and deletable like a manual prompt by operators
  and agents.
- **AC-INTEGRATIONS-PROMPT-SYNC-007.2:** An override shall show an
  "Overridden" badge naming the repository file it replaces, with the control
  on Local. Saving Repository shall return the prompt to sync ownership, keep
  its local content (including unsaved edits to it) under
  AC-INTEGRATIONS-PROMPT-SYNC-009.1, use the repository version, and run a
  sync. Saving either way shall resolve a conflict on the prompt.
- **AC-INTEGRATIONS-PROMPT-SYNC-007.3:** A sync shall leave an override's
  content and ownership unchanged when its file is present, report its name as
  overridden rather than as a warning, and leave it in place when its file is
  removed (clearing a conflict under AC-INTEGRATIONS-PROMPT-SYNC-008.7).
- **AC-INTEGRATIONS-PROMPT-SYNC-007.4:** Deleting an override shall let the
  next sync recreate the synced prompt from its file.
- **AC-INTEGRATIONS-PROMPT-SYNC-007.5:** The Prompt Sync status card shall
  list prompts that are overridden locally, separately from warnings.
- **AC-INTEGRATIONS-PROMPT-SYNC-007.6:** Saving a prompt's versions shall
  require configuration-management authority, reject a prompt that is neither
  synced nor an override, reject a save based on an obsolete revision of the
  prompt (including one a concurrent sync changed), and never change a manual
  or built-in prompt.

### REQ-INTEGRATIONS-PROMPT-SYNC-008: Name conflicts

**Intent:** A repository prompt that shares a name with a prompt already in
the installation is never resolved silently; the user decides which version
wins, and an undecided conflict stays visible until they do.

**User story:** As an operator connecting a prompt repository, I want to see
which of my prompts the repository also defines and choose per prompt which
version to use, so that sync never hides or replaces my prompts without me
knowing.

#### Acceptance criteria

- **AC-INTEGRATIONS-PROMPT-SYNC-008.1:** Saving the Prompt Sync dialog shall
  first check the entered source without storing or changing anything. The
  check shall list each conflict: a valid prompt file whose name matches a
  manual prompt, a built-in prompt, or an override with an unresolved
  conflict. It shall return each conflict's name, file path, local content,
  and repository content. A failed check shall keep the dialog open, save
  nothing, and show the sanitized error.
- **AC-INTEGRATIONS-PROMPT-SYNC-008.2:** When the check finds conflicts, the
  dialog shall show a review step before saving, on desktop and phone. It
  shall explain once what the local and repository versions mean, then list
  one short row per prompt: its name and file path, a labelled switch between
  the local version (the default) and the repository version, and a control
  that shows both versions. On phones and coarse pointers the switch and
  control shall be touch-sized. **Back** shall return to the fields with the
  draft intact. Closing the dialog shall save nothing.
- **AC-INTEGRATIONS-PROMPT-SYNC-008.3:** Saving from the review step shall
  store the configuration and run a sync that applies each choice. **Keep
  local** shall make the prompt an override of that file with no conflict.
  **Use repository version** shall make it a synced prompt with the
  repository content, keeping its local content under
  AC-INTEGRATIONS-PROMPT-SYNC-009.1. If the sync finds conflicts that the
  check did not list, the dialog shall stay open on the review step for them
  until each is resolved.
- **AC-INTEGRATIONS-PROMPT-SYNC-008.4:** A sync that finds a conflict without
  a choice for it (an automatic sync, **Sync now**, or a file added after
  setup) shall make the existing prompt an override of that file with an
  unresolved conflict. It shall keep the prompt's name, identity, and
  content, and report the name as a conflict in the sync result, not as a
  warning. Built-in prompts shall be handled exactly like manual prompts.
- **AC-INTEGRATIONS-PROMPT-SYNC-008.5:** An unresolved conflict shall stay
  visible until it is resolved: a "Conflict" badge on its row; a
  warning-styled banner at the top of Settings > Prompts with a title giving
  the number of conflicts, the conflicting prompt names, and a primary
  **Review conflicts** action that opens the same review list; an indicator
  on the Prompts entry of the settings menu; and an indicator on the Settings
  entry of the app sidebar, on desktop and in phone navigation.
- **AC-INTEGRATIONS-PROMPT-SYNC-008.6:** Resolving a conflict with **Keep
  local** shall clear the conflict and keep the override. **Use repository
  version**, or saving the row with its control on either version, shall
  resolve it like AC-INTEGRATIONS-PROMPT-SYNC-007.2. Resolution shall
  require configuration-management authority.
- **AC-INTEGRATIONS-PROMPT-SYNC-008.7:** When the file of an override leaves
  the source, its conflict and stored repository version shall be cleared and
  the override kept.
- **AC-INTEGRATIONS-PROMPT-SYNC-008.8:** A prompt file whose content equals
  the existing prompt's content is not a conflict. The check shall not list
  it, and the sync shall adopt a manual or built-in prompt as a synced prompt
  without asking. A conflicted override whose file now matches its content
  shall become a synced prompt.

### REQ-INTEGRATIONS-PROMPT-SYNC-009: Lossless switching

**Intent:** Choosing between the local and repository versions never throws
either one away, so a choice can be reversed.

**User story:** As an operator, I want to switch a prompt back to my local
version after trying the repository version, without retyping it.

#### Acceptance criteria

- **AC-INTEGRATIONS-PROMPT-SYNC-009.1:** Returning an override to sync
  ownership (turning the switch off, or choosing the repository version)
  shall keep its local content with the prompt, and use the stored repository
  version at once when there is one. An override whose file left the source
  has no repository version, so choosing Repository for it shall be rejected
  and its local version kept; Settings shall show "Removed from repository"
  in place of its switch, and it stays editable.
- **AC-INTEGRATIONS-PROMPT-SYNC-009.2:** Switching a synced prompt to Local
  shall show and, when saved, use the kept local content, keeping the
  repository version as the other version. With no kept copy, the override
  shall start from the current synced content.
- **AC-INTEGRATIONS-PROMPT-SYNC-009.3:** Syncs that update a synced prompt
  shall preserve its kept local version. Every override whose file is present
  shall keep the current repository version, so both versions are known
  however the prompt became an override.
- **AC-INTEGRATIONS-PROMPT-SYNC-009.4:** When a synced prompt with kept local
  content leaves the source, the sync shall turn it into a manual prompt with
  the kept local content instead of deleting it. A sync shall never delete a
  built-in prompt; a built-in prompt that leaves the source shall become
  manual again with its kept or current content.
- **AC-INTEGRATIONS-PROMPT-SYNC-009.5:** Removing the configuration while any
  synced prompt or override has a local and a repository version that differ,
  however it got there, shall first show a review step listing every such
  prompt, on desktop and phone, in the same compact form as
  AC-INTEGRATIONS-PROMPT-SYNC-008.2. Each row shall start on the version the
  prompt uses now. **Back** shall return without removing anything.
  Confirming shall release every prompt under
  AC-INTEGRATIONS-PROMPT-SYNC-004.5 with the chosen version in the same step.
  When no prompt has two versions and no prompt came only from the
  repository (AC-INTEGRATIONS-PROMPT-SYNC-009.6), removal shall keep its
  single confirmation. A failure to list the prompts shall remove nothing and
  report the error.
- **AC-INTEGRATIONS-PROMPT-SYNC-009.6:** When any synced prompt came only from
  the repository (it is not built-in and has no kept local version), the
  removal step shall offer one checkbox, off by default, to delete those
  prompts instead of keeping them. It shall state how many prompts it deletes
  and name them, and note that an `@name` mention of a deleted prompt stops
  expanding. Only the named prompts shall be deleted; one synced after the
  step opened shall be released instead. Removal shall advance the revision
  of every released prompt, so a save based on an earlier revision is
  rejected. Prompts with a local version follow their row's choice and
  built-in prompts are never deleted. Deleting and releasing shall run in the
  same transaction, before the configuration is deleted.

### REQ-INTEGRATIONS-PROMPT-SYNC-010: Agent access to synced prompts

**Intent:** Agents can explain how synced prompts work, but cannot take a
prompt out of the repository's control.

**User story:** As an operator, I want an agent that tries to edit a synced
prompt to tell me how to override it, instead of silently changing a copy
that has no effect.

#### Acceptance criteria

- **AC-INTEGRATIONS-PROMPT-SYNC-010.1:** An agent MCP write to a synced
  prompt shall be rejected with an error stating that sync owns the prompt,
  that a local change would have no effect, that the user can turn on Local
  override for it in Settings > Prompts, and that agents cannot change that
  setting.
- **AC-INTEGRATIONS-PROMPT-SYNC-010.2:** MCP prompt list and read results
  shall report each prompt's source, whether it has an unresolved conflict,
  and, for synced prompts and overrides, guidance an agent can relay to the
  user about editing it.
- **AC-INTEGRATIONS-PROMPT-SYNC-010.3:** No MCP tool shall override a synced
  prompt, revert an override, or resolve a conflict. The MCP tool
  descriptions shall say that only the user can switch a prompt between its
  local and repository versions.
- **AC-INTEGRATIONS-PROMPT-SYNC-010.4:** An agent write to an override,
  including one with an unresolved conflict, shall change the local version
  and leave the conflict state unchanged.

## Out of scope

- Telling agents apart from people on the HTTP API. An agent that holds the
  user's own session or personal access token is that user;
  REQ-INTEGRATIONS-PROMPT-SYNC-010 is enforced on the MCP surface agents are
  given.
- Merging the local and repository versions of a prompt.
