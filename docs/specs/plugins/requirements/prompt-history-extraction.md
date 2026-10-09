---
status: active
system: plugins
created: 2026-09-23
owners:
  - kandev
---

# Prompt History Leaves Core Requirements

## Overview

Prompt History is a per-task review surface: what was asked of the agent, in
which order, when it was sent, and how long the agent worked on it. Core
previously shipped it as a built-in workbench panel with its own dockview
identity, a user-message projection in the web store, pagination hooks,
layout-profile membership, locale copy, and end-to-end coverage.

The plugin system already owns the Host contracts that let a plugin provide the
same surface through published browser APIs, and a replacement plugin exists.
Keeping a second, core-owned implementation of one surface costs a private
projection that no other core consumer uses, a competing saved-layout identity,
and two places to fix the same review workflow.

This document owns the extraction outcome: what core stops owning, which Host
contracts must stay, and what an existing installation observes. The Host
prerequisite boundary stays owned by
[Prompt History Plugin Host Prerequisites](prompt-history-extraction-host.md).
The deprecated
[Prompt History Panel Requirements](../../ui/requirements/prompt-history-panel.md)
remains the historical record of shipped behavior; this document owns the
post-removal contract.

## Terminology

- **Built-in Prompt history panel:** the core `prompt-history` dockview panel,
  its row and content components, its core user-message projection and
  pagination hooks, its layout-registry identity, and its task locale copy. It
  is offered by the desktop and Office workbench "+" menu and by the
  native-mobile `Panels` picker.
- **Replacement plugin:** the external
  [`kandev-plugin-prompt-history`](https://github.com/Fclem/kandev-plugin-prompt-history)
  package registered in `plugin-registry/plugins.yaml`. It provides the review
  surface as a plugin task panel.
- **Host conversation façade:** the capability-gated browser contract defined by
  [Prompt History Plugin Host Prerequisites](prompt-history-extraction-host.md)
  (`host.conversation.*`, `host.ui.PromptMentionText`, task-panel registration
  and its navigation capability).
- **Retired panel identity:** the fixed panel id and component name
  `prompt-history`, and the `MobileSessionPanel` member of the same name.

## Requirements

### REQ-PLUGINS-PROMPT-HISTORY-EXTRACTION-001: Prompt History Leaves Core

**Intent:** Kandev's built-in Prompt history panel and the core-only code that
feeds it are removed, while every generic Host contract a replacement plugin
needs stays available, so one implementation of the surface remains and it is
the plugin's.

**User story:** As a Kandev user, I want the prompt review surface to come from
one implementation so that it behaves consistently and core carries no
single-consumer feature state.

#### Acceptance criteria

- **AC-PLUGINS-PROMPT-HISTORY-EXTRACTION-001.1:** Core contains no built-in
  Prompt history panel. The desktop and Office workbench "+" menu
  (`AddPanelMenuItems`) offers no Prompt history row, the native-mobile `Panels`
  picker offers no Prompt history option, no dockview component or panel
  identity named `prompt-history` is registered or renderable, and the core
  panel components, row components, panel host, and their tests are gone.
- **AC-PLUGINS-PROMPT-HISTORY-EXTRACTION-001.2:** Core contains no
  prompt-history-only client state or derivation. The web store has no
  user-message prompt projection with its own pagination cursor, loading
  metadata, generations, or live-event fan-out; no prompt-page hook, no prompt
  pagination sentinel consumer, and no prompt-entry builder for that panel
  remains. Shared transcript machinery (message list, transcript pagination,
  turn state, scroll targeting) is unchanged in behavior.
- **AC-PLUGINS-PROMPT-HISTORY-EXTRACTION-001.3:** The task locale catalogs
  (`en`, `ja`, `pt-pt`, `pseudo`, `zh-cn`, `zh-hk`, `zh-tw`) drop the panel's
  copy keys (`task:promptHistory`, `task:promptHistoryPromptLabel`,
  `task:promptHistoryPromptLabelGeneric`, `task:promptHistoryEmpty`,
  `task:expandPrompt`, `task:collapsePrompt`) and the now-unused compact elapsed
  unit keys (`common:mShort`, `common:hShort`, `common:dShort`), and every
  catalog stays complete under `pnpm run i18n:check`, including the
  `pt-pt/_verbatim.json` declarations. Copy that the transcript still uses
  (`task:loadingOlderMessages`, the duration unit keys, saved-prompt copy) stays.
- **AC-PLUGINS-PROMPT-HISTORY-EXTRACTION-001.4:** Every generic Host contract
  the panel consumed stays in core and keeps its current public shape: the
  browser conversation façade (`host.conversation.useSessionMessages`,
  `useSessionTurns`, `useMessageFavorite`), `host.ui.PromptMentionText`,
  capability-gated task-panel registration with its message-navigation
  capability, plugin-panel placement on desktop and native mobile, native
  transcript scroll targeting, and the browser-local message-favorite store.
  None of them gains a prompt-history-specific name, branch, or option.
- **AC-PLUGINS-PROMPT-HISTORY-EXTRACTION-001.5:** The message contract keeps the
  durable per-session prompt ordinal (`prompt_index` / public `promptIndex`) and
  the user-only message filter used to page prompts. Removing the built-in panel
  does not remove fields, routes, or filters that the replacement plugin needs
  to render prompt numbers, durations, and prompt-only pages.
- **AC-PLUGINS-PROMPT-HISTORY-EXTRACTION-001.6:** The replacement plugin reaches
  behavioral parity with the removed surface through published contracts only:
  prompts of the task's active session newest first, absolute `#N` ordinals
  that survive deletion, agent-work durations, saved-prompt alias chips,
  read-only favorite state, automatic older-page loading, navigation to a
  prompt in the transcript, and a full-height native-mobile surface reached from
  the `Panels` picker.
- **AC-PLUGINS-PROMPT-HISTORY-EXTRACTION-001.7:** Saved layouts, saved layout
  profiles, per-environment layouts, and maximized-layout state written by an
  earlier version restore without the retired panel and without an error, an
  empty panel, or a broken group: every other panel, group, and size is
  preserved, the retired entry is dropped, and no user action is required. A
  maximized group that still contains a surviving panel keeps its maximized
  state; a maximized group whose only panel was the retired one loses that
  group's maximized state and nothing else. A saved default profile that
  references the retired panel keeps applying: it is not rejected, demoted to an
  ignored legacy record, or replaced by the built-in default. The retired panel
  identity is not reused for a different panel.
- **AC-PLUGINS-PROMPT-HISTORY-EXTRACTION-001.8:** Transcript surfaces that
  reused prompt-history presentation keep working and no longer reference the
  removed panel: the per-user-prompt turn duration in the message action row
  ([REQ-UI-PROMPT-TURN-DURATION-001](../../ui/requirements/prompt-turn-duration.md)),
  saved-prompt alias chips across the transcript and anchored last-prompt bar
  ([REQ-UI-PROMPT-ALIAS-001](../../ui/requirements/prompt-alias-rendering.md)),
  the visible pagination stop at prompt `#1`, and scroll-to-last-prompt
  navigation.
- **AC-PLUGINS-PROMPT-HISTORY-EXTRACTION-001.9:** **GIVEN** Kandev with no
  plugins installed, **WHEN** a user opens a task workbench on desktop or a
  phone, **THEN** no Prompt history entry appears in the "+" menu or the
  `Panels` picker, no tab, title, or empty state mentions it, and the rest of
  the workbench (Chat, Plan, Todos, Changes, Files, terminals, canvases, and
  installed plugin panels) is unaffected. On a phone, the `Panels`
  bottom-navigation entry is shown only while a task canvas or an enabled plugin
  panel can populate its sheet; a task with neither no longer offers an entry
  whose only former content was the removed panel.
- **AC-PLUGINS-PROMPT-HISTORY-EXTRACTION-001.10:** Residual data left by the
  removed feature is inert. A stale `prompt-history` entry inside a saved
  layout, a saved profile, a per-environment layout, or browser-side maximized
  state is ignored while the rest of that record is used, browser-local favorite
  state stays readable by the Host façade, and nothing deletes a saved layout,
  profile, or favorite during the upgrade. Browser-side maximized state may be
  discarded when it cannot survive without the retired panel; that is the only
  removed residual and it carries no user content.

## Out of scope

- Implementing, packaging, publishing, or versioning the replacement plugin.
  That repository owns its own code, parity work, and releases.
- Removing or changing the durable prompt sequence, `prompt_index`, the
  `author_type=user` message filter, message or turn storage, or the
  task-description initial-prompt fallback. These serve task ownership and the
  Host façade, not the panel.
- Layout redesign, new panels, and any replacement for the removed panel's
  placement beyond the plugin's own choice of panel id.
- Migrating users' saved layouts into the plugin's panel id, and carrying the
  panel's browser-local state into the plugin.
- Public documentation of the external plugin's settings and behavior.

## Related contracts

- [Prompt History Plugin Host Prerequisites](prompt-history-extraction-host.md):
  owns the Host boundary this extraction preserves. This requirement supersedes
  `AC-PLUGINS-PROMPT-HISTORY-HOST-005.4` (core removal deferred to a later
  package), the saved-layout-ID migration clause of
  `AC-PLUGINS-PROMPT-HISTORY-HOST-001.5`, the core-ownership clause of
  `AC-PLUGINS-PROMPT-HISTORY-HOST-006.7`, and the removed core panel in
  `AC-PLUGINS-PROMPT-HISTORY-HOST-005.3`'s parity comparison, which is
  re-anchored to the in-repo fixture suites and to
  `AC-PLUGINS-PROMPT-HISTORY-EXTRACTION-001.6`. The rest of those criteria still
  govern.
- [Prompt History Panel Requirements](../../ui/requirements/prompt-history-panel.md):
  owns the product behavior being moved out of core.
- [Plugin requirements](plugins.md): own task-panel registration, placement, and
  mobile panel composition.
