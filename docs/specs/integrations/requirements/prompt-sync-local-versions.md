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
- **Kept local version:** the local content stored with a prompt while the
  repository version is in use.

## Requirements

### REQ-INTEGRATIONS-PROMPT-SYNC-007: Local overrides

**Intent:** A shared repository can ship a default prompt at a chosen point
(for example `@post-pr-review`) that one installation replaces with its own
version, and can return to the shared default later.

**User story:** As an operator syncing a shared prompt repository, I want to
override one synced prompt locally, so that a shared workflow step uses my
version without forking the repository.

#### Acceptance criteria

- **AC-INTEGRATIONS-PROMPT-SYNC-007.1:** Settings > Prompts shall show a
  labelled "Local override" switch on each synced prompt and each override, on
  desktop and phone, visually distinct from the edit and delete controls; on
  phones and coarse pointers its hit area shall be touch-sized. The switch
  shall stay in the same place in every row state: it is the first of the
  row's trailing controls, and the turn-off confirmation appears below the row
  header without moving it. Turning it on shall turn the prompt into an
  override with the same name and identity, starting from its kept local
  content (AC-INTEGRATIONS-PROMPT-SYNC-009.2) or else its current content,
  editable and deletable like a manual prompt by operators and agents.
- **AC-INTEGRATIONS-PROMPT-SYNC-007.2:** An override shall show an
  "Overridden" badge naming the repository file it replaces, with the switch
  on. Turning the switch off shall first ask the user to confirm using the
  repository version, saying the local version is kept; cancelling shall keep
  the override unchanged. Confirming shall return the prompt to sync ownership,
  keep its local content under AC-INTEGRATIONS-PROMPT-SYNC-009.1, and run a
  sync, so the repository version becomes the prompt's content.
- **AC-INTEGRATIONS-PROMPT-SYNC-007.3:** A sync shall leave an override's
  content and ownership unchanged when its file is present, report its name as
  overridden rather than as a warning, and leave it in place when its file is
  removed (clearing a conflict under AC-INTEGRATIONS-PROMPT-SYNC-008.7).
- **AC-INTEGRATIONS-PROMPT-SYNC-007.4:** Deleting an override shall let the
  next sync recreate the synced prompt from its file.
- **AC-INTEGRATIONS-PROMPT-SYNC-007.5:** The Prompt Sync status card shall
  list prompts that are overridden locally, separately from warnings.
- **AC-INTEGRATIONS-PROMPT-SYNC-007.6:** Override and Revert shall require
  configuration-management authority, reject prompts that are not synced or
  not overrides respectively, and never change a manual or built-in prompt.

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
  dialog shall show a review step before saving, on desktop and phone. Each
  conflict shall offer **Keep local** (the default) and **Use repository
  version**, and both versions shall be viewable; on phones and coarse
  pointers the choices shall be touch-sized. **Back** shall return to the
  fields with the draft intact. Closing the dialog shall save nothing.
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
  visible until it is resolved: a "Conflict" badge on its row; a banner on
  Settings > Prompts whose **Review** action opens the same review step; an
  indicator on the Prompts entry of the settings menu; and an indicator on the
  Settings entry of the app sidebar, on desktop and in phone navigation.
- **AC-INTEGRATIONS-PROMPT-SYNC-008.6:** Resolving a conflict with **Keep
  local** shall clear the conflict and keep the override. **Use repository
  version**, or turning the row's Local override switch off and confirming,
  shall resolve it like AC-INTEGRATIONS-PROMPT-SYNC-007.2. Resolution shall
  require configuration-management authority.
- **AC-INTEGRATIONS-PROMPT-SYNC-008.7:** When the file of an override with an
  unresolved conflict leaves the source, the conflict shall be cleared and
  the override kept.

### REQ-INTEGRATIONS-PROMPT-SYNC-009: Lossless switching

**Intent:** Choosing between the local and repository versions never throws
either one away, so a choice can be reversed.

**User story:** As an operator, I want to switch a prompt back to my local
version after trying the repository version, without retyping it.

#### Acceptance criteria

- **AC-INTEGRATIONS-PROMPT-SYNC-009.1:** Returning an override to sync
  ownership (turning the switch off, or **Use repository version**) shall
  keep its local content with the prompt.
- **AC-INTEGRATIONS-PROMPT-SYNC-009.2:** Turning the Local override switch on
  shall restore the kept local content and then discard the kept copy. With
  no kept copy, the override shall start from the current synced content.
- **AC-INTEGRATIONS-PROMPT-SYNC-009.3:** Syncs that update a synced prompt
  shall preserve its kept local content.
- **AC-INTEGRATIONS-PROMPT-SYNC-009.4:** When a synced prompt with kept local
  content leaves the source, the sync shall turn it into a manual prompt with
  the kept local content instead of deleting it. A sync shall never delete a
  built-in prompt; a built-in prompt that leaves the source shall become
  manual again with its kept or current content.

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
  descriptions shall say that only the user can change Local override.
- **AC-INTEGRATIONS-PROMPT-SYNC-010.4:** An agent write to an override,
  including one with an unresolved conflict, shall change the local version
  and leave the conflict state unchanged.

## Out of scope

- Telling agents apart from people on the HTTP API. An agent that holds the
  user's own session or personal access token is that user;
  REQ-INTEGRATIONS-PROMPT-SYNC-010 is enforced on the MCP surface agents are
  given.
- Merging the local and repository versions of a prompt.
