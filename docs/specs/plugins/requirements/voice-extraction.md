---
status: active
system: plugins
created: 2026-08-12
owners:
  - kandev
---
# Voice Mode Leaves Core Requirements

## Overview

Voice Mode is an optional way to put text into a composer. It is not part of what kandev is for, and carrying it in core cost every install a 100 MB browser ML dependency, a per-user settings block in the shared user-settings row, an unauthenticated transcription route, and a server-side OpenAI key whether or not anyone dictated a word.

## Requirements

### REQ-PLUGINS-VOICE-EXTRACTION-001: Voice Mode Leaves Core

**Intent:** Voice Mode is an optional way to put text into a composer. It is not part of what kandev is for, and carrying it in core cost every install a 100 MB browser ML dependency, a per-user settings block in the shared user-settings row, an unauthenticated transcription route, and a server-side OpenAI key whether or not anyone dictated a word.

#### Acceptance criteria

- **AC-PLUGINS-VOICE-EXTRACTION-001.1:** Voice Mode ships as [`kdlbs/kandev-plugin-voice`](https://github.com/kdlbs/kandev-plugin-voice), installed like any other plugin. Core contains no voice code.
- **AC-PLUGINS-VOICE-EXTRACTION-001.2:** The plugin reaches behaviour parity with the removed feature: the same three engines (browser speech, in-browser Whisper, server-side Whisper), insertion at the current selection with the same word-boundary spacing rule, optional auto-send, hold and toggle activation with the coarse-pointer override, cancellation on task/session change, the same `Cmd/Ctrl+Shift+M` shortcut, and the same four composers on desktop and phone: task chat, Quick Chat, task creation, new-session creation.
- **AC-PLUGINS-VOICE-EXTRACTION-001.3:** The plugin uses only published host contracts: the composer action slots, `host.storage` for per-user preferences, the owner-scoped `plugin-settings` slot for its settings form, `registerKeybinding` for the shortcut, the `/api/plugins/{id}/ui/*` asset route for its Whisper worker, and one authenticated webhook for the server relay. It adds no core-specific coupling.
- **AC-PLUGINS-VOICE-EXTRACTION-001.4:** Core keeps every generic host API the extraction needed. None of them mention voice.
- **AC-PLUGINS-VOICE-EXTRACTION-001.5:** **The OpenAI key does not move.** An operator who set `KANDEV_VOICE_OPENAI_API_KEY` re-enters the key in the plugin's settings form. Core never had a way to write into a plugin's secret storage and should not gain one for a single plugin.
- **AC-PLUGINS-VOICE-EXTRACTION-001.6:** **Per-user preferences do not move.** Everyone's engine, language, model, activation mode and auto-send return to the plugin's defaults, which match the previous core defaults except that the engine list now hides server transcription until an operator saves a key. Copying them across would require core to write into one named plugin's per-user storage, which is exactly the coupling the extraction exists to remove.
- **AC-PLUGINS-VOICE-EXTRACTION-001.7:** A stale `voice_mode` object left inside an existing `users.settings` row is inert. Nothing reads it and nothing needs to delete it.
- **AC-PLUGINS-VOICE-EXTRACTION-001.8:** **GIVEN** kandev with no plugins installed, **WHEN** a user opens task chat, **THEN** no microphone control appears and nothing in Settings mentions voice.

## Migrated source detail

## Why

Voice Mode is an optional way to put text into a composer. It is not part of what kandev is for, and
carrying it in core cost every install a 100 MB browser ML dependency, a per-user settings block in
the shared user-settings row, an unauthenticated transcription route, and a server-side OpenAI key
whether or not anyone dictated a word.

The host prerequisites for moving it out shipped separately
([voice-extraction-host](voice-extraction-host.md), ADR
[composer access and authenticated webhooks](../../../decisions/2026-08-11-composer-access-authenticated-webhooks.md)).
This spec records the extraction itself: what the plugin owns, what core stops owning, and what an
existing user has to do.

## What

- Voice Mode ships as [`kdlbs/kandev-plugin-voice`](https://github.com/kdlbs/kandev-plugin-voice),
  installed like any other plugin. Core contains no voice code.
- The plugin reaches behaviour parity with the removed feature: the same three engines (browser
  speech, in-browser Whisper, server-side Whisper), insertion at the current selection with the same
  word-boundary spacing rule, optional auto-send, hold and toggle activation with the coarse-pointer
  override, cancellation on task/session change, the same `Cmd/Ctrl+Shift+M` shortcut, and the same
  four composers on desktop and phone: task chat, Quick Chat, task creation, new-session creation.
- The plugin uses only published host contracts: the composer action slots, `host.storage` for
  per-user preferences, the owner-scoped `plugin-settings` slot for its settings form,
  `registerKeybinding` for the shortcut, the `/api/plugins/{id}/ui/*` asset route for its Whisper worker, and one authenticated
  webhook for the server relay. It adds no core-specific coupling.
- Core keeps every generic host API the extraction needed. None of them mention voice.

### Removed from core

| Removed | Replacement |
|---|---|
| `POST /api/v1/transcribe` (unauthenticated) | the plugin's `transcribe` webhook, which requires a signed-in user |
| `voice.openAIApiKey` / `KANDEV_VOICE_OPENAI_API_KEY` | the plugin's own **OpenAI API key** setting |
| `userSettings.voiceMode` (backend model, DTO, store, boot payload, frontend types) | the plugin's per-user `host.storage` entry |
| The Voice Mode section of **Settings > Task Behavior** | **Settings > Plugins > Voice Mode** |
| The `VOICE_INPUT_TOGGLE` core shortcut | the plugin keybinding, listed and rebindable in the same **Settings > Keyboard shortcuts** page |
| `@huggingface/transformers` in `apps/web` | bundled inside the plugin's Whisper worker |

### Migration

Both migration steps are manual and deliberate; neither is silent.

- **The OpenAI key does not move.** An operator who set `KANDEV_VOICE_OPENAI_API_KEY` re-enters the
  key in the plugin's settings form. Core never had a way to write into a plugin's secret storage
  and should not gain one for a single plugin.
- **Per-user preferences do not move.** Everyone's engine, language, model, activation mode and
  auto-send return to the plugin's defaults, which match the previous core defaults except that the
  engine list now hides server transcription until an operator saves a key. Copying them across
  would require core to write into one named plugin's per-user storage, which is exactly the
  coupling the extraction exists to remove.
- A stale `voice_mode` object left inside an existing `users.settings` row is inert. Nothing reads
  it and nothing needs to delete it.

## Scenarios

- **GIVEN** kandev with no plugins installed, **WHEN** a user opens task chat, **THEN** no
  microphone control appears and nothing in Settings mentions voice.
- **GIVEN** the Voice plugin installed and enabled, **WHEN** a user dictates into task chat, Quick
  Chat, task creation or new-session creation on desktop or a phone, **THEN** the transcript is
  inserted at the caret and the native submit path owns sending it.
- **GIVEN** no operator OpenAI key, **WHEN** a user opens the plugin's settings, **THEN** server
  transcription is shown as unavailable and the two browser engines still work.
- **GIVEN** an operator key saved in the plugin, **WHEN** a signed-in user dictates with the server
  engine, **THEN** the audio reaches OpenAI through the plugin's authenticated webhook and the key
  never reaches the browser.
- **GIVEN** an anonymous caller on an install with authentication enabled, **WHEN** it posts to the
  transcription webhook, **THEN** the request fails before the plugin runs and cannot spend the
  operator's budget.
- **GIVEN** a user who had Voice Mode configured before upgrading, **WHEN** they install the plugin,
  **THEN** dictation works at the plugin's defaults and their previous engine and language choices
  are not carried over.

## Out Of Scope

- Publishing the plugin to the official marketplace catalog.
- Localizing the plugin. Kandev has no host i18n API for plugins yet; the plugin keeps its copy in
  one module so a catalog can be added once that API exists.
- Migrating operator or per-user configuration automatically, for the reasons above.
