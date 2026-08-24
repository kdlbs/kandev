---
status: deprecated
system: ui
created: 2026-08-11
owners:
  - kandev
---
# Voice Mode In Task Behavior Requirements

## Overview

Voice input changes how users interact with tasks, but its controls currently occupy a separate top-level Settings destination. Keeping task interaction preferences together makes Voice Mode easier to discover in context and reduces the main Settings menu without changing saved behavior.

## Requirements

### REQ-UI-VOICE-MODE-TASK-BEHAVIOR-001: Voice Mode In Task Behavior

**Intent:** Voice input changes how users interact with tasks, but its controls currently occupy a separate top-level Settings destination. Keeping task interaction preferences together makes Voice Mode easier to discover in context and reduces the main Settings menu without changing saved behavior.

#### Acceptance criteria

- **AC-UI-VOICE-MODE-TASK-BEHAVIOR-001.1:** **Task Behavior** contains the existing Voice Mode section after the task-action and message-queue sections.
- **AC-UI-VOICE-MODE-TASK-BEHAVIOR-001.2:** The Settings main menu has no standalone **Voice Mode** row.
- **AC-UI-VOICE-MODE-TASK-BEHAVIOR-001.3:** Settings search continues to find the Voice Mode section and each voice control, and selecting a result opens **Task Behavior** at the matching section or control.
- **AC-UI-VOICE-MODE-TASK-BEHAVIOR-001.4:** The former `/settings/voice-mode` route is removed without a compatibility redirect.
- **AC-UI-VOICE-MODE-TASK-BEHAVIOR-001.5:** Voice settings keep their existing values, capability detection, disabled-state presentation, draft/save/discard behavior, and user-facing copy.
- **AC-UI-VOICE-MODE-TASK-BEHAVIOR-001.6:** Desktop and mobile use the existing responsive Settings page. The shared settings content remains the single scroll owner, and every Voice Mode control remains reachable without document-level horizontal overflow.
- **AC-UI-VOICE-MODE-TASK-BEHAVIOR-001.7:** **GIVEN** a user opens Settings on desktop or mobile, **WHEN** the main menu is shown, **THEN** it contains **Task Behavior** and no standalone **Voice Mode** row.
- **AC-UI-VOICE-MODE-TASK-BEHAVIOR-001.8:** **GIVEN** a user opens **Task Behavior**, **WHEN** the page renders, **THEN** task actions, message queue, and Voice Mode appear in that order with all existing voice controls available.

## Migrated source detail

> **Archived.** Voice Mode left core and now ships as
> [`kdlbs/kandev-plugin-voice`](https://github.com/kdlbs/kandev-plugin-voice); its settings live on
> the plugin's own page. See [Voice Mode leaves core](../../plugins/requirements/voice-extraction.md). This spec is
> kept as the record of how the setting was placed while it was a core feature.

## Why

Voice input changes how users interact with tasks, but its controls currently occupy a separate
top-level Settings destination. Keeping task interaction preferences together makes Voice Mode
easier to discover in context and reduces the main Settings menu without changing saved behavior.

## What

- **Task Behavior** contains the existing Voice Mode section after the task-action and message-queue
  sections.
- The Settings main menu has no standalone **Voice Mode** row.
- Settings search continues to find the Voice Mode section and each voice control, and selecting a
  result opens **Task Behavior** at the matching section or control.
- The former `/settings/voice-mode` route is removed without a compatibility redirect.
- Voice settings keep their existing values, capability detection, disabled-state presentation,
  draft/save/discard behavior, and user-facing copy.
- Desktop and mobile use the existing responsive Settings page. The shared settings content remains
  the single scroll owner, and every Voice Mode control remains reachable without document-level
  horizontal overflow.

## Scenarios

- **GIVEN** a user opens Settings on desktop or mobile, **WHEN** the main menu is shown, **THEN** it
  contains **Task Behavior** and no standalone **Voice Mode** row.
- **GIVEN** a user opens **Task Behavior**, **WHEN** the page renders, **THEN** task actions, message
  queue, and Voice Mode appear in that order with all existing voice controls available.
- **GIVEN** a user searches Settings for Voice Mode or a voice control, **WHEN** they select the
  result, **THEN** Task Behavior opens and scrolls to the matching section or control.
- **GIVEN** the application route registry, **WHEN** Settings routes are enumerated, **THEN**
  `/settings/voice-mode` is absent and Task Behavior is the only route that renders Voice Mode.
- **GIVEN** a user changes a voice preference from Task Behavior, **WHEN** they save or discard
  through the shared settings action, **THEN** persistence behavior matches the former standalone
  page.
- **GIVEN** a Pixel 5-sized viewport, **WHEN** the user opens Task Behavior and scrolls through
  Voice Mode, **THEN** every control is visible and operable, the settings container owns vertical
  scrolling, and the document has no horizontal overflow.

## Out Of Scope

- Changing voice engines, models, shortcuts, defaults, persistence, or runtime behavior.
- Redesigning Task Behavior, the shared settings save action, or Settings navigation.
- Preserving compatibility for the former Voice Mode URL.

## Implementation Plan

See [the implementation plan](../../../plans/voice-mode-task-behavior/plan.md).
