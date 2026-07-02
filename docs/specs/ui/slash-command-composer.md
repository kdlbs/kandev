---
status: shipped
created: 2026-07-02
owner: cfl
---

# Slash Command Composer Selection

## Why

ACP agents can advertise slash commands during session setup. Kandev exposes those commands in the chat composer, but choosing a command from the autocomplete currently sends the command immediately. Users need command selection to prepare a draft instead, so they can add context, arguments, or surrounding text before deciding to send.

## What

- When a chat composer has advertised agent commands and the user types `/`, the composer shows matching slash commands from the active session.
- Selecting a slash command with Enter, Tab, or pointer/touch replaces only the active slash trigger range with a visually distinct inline command chip in the composer. It MUST NOT submit, queue, or otherwise send a chat message.
- The inserted command chip remains part of the editable draft. The user can add text before or after it, delete it, or leave the composer without starting an agent turn.
- The command chip serializes to the same plain slash command text when submitted, copied from the draft value, or saved as a draft.
- After selection, focus remains in the composer and the caret lands after the inserted command with room to continue typing arguments.
- Explicit send actions still submit the full current draft: the configured submit shortcut, the Send button, and existing queue behavior continue to work after the user edits the inserted command.
- Escape dismisses the slash command menu without sending a message or changing the draft.
- The behavior is the same for task chat and quick chat because both use the shared TipTap composer. Mobile touch selection has the same non-sending selection semantics as desktop keyboard selection.
- Passthrough terminal/composer surfaces that intentionally treat `/` as literal terminal input do not show this menu and are unchanged.

## Scenarios

- **GIVEN** a task chat session with advertised commands, **WHEN** the user types `/s` and presses Enter to choose `/slow`, **THEN** the composer contains `/slow` as a command chip, focus remains in the composer, and no user message or agent turn is created.
- **GIVEN** a selected `/slow` draft in task chat, **WHEN** the user types `1s` after it and clicks Send, **THEN** the chat sends exactly `/slow 1s` and the agent handles it as a normal slash command prompt.
- **GIVEN** the draft already contains `please run ` before the slash trigger, **WHEN** the user selects `/slow`, **THEN** the draft becomes `please run /slow` without sending.
- **GIVEN** the slash menu is open in a composer configured to submit plain Enter, **WHEN** the user presses Enter on a highlighted menu item, **THEN** Enter selects the command and does not submit the draft.
- **GIVEN** the slash menu is open in quick chat, **WHEN** the user selects a command, **THEN** quick chat keeps the command in the editable draft and does not send until the user explicitly sends.
- **GIVEN** the slash menu is open on mobile, **WHEN** the user taps a command row, **THEN** the composer keeps the command in the draft and the user can continue editing before tapping Send.
- **GIVEN** the slash menu is open, **WHEN** the user presses Escape, **THEN** the menu closes and no message is sent.

## Out of scope

- Changing the ACP `available_commands_update` contract or backend command storage.
- Adding command argument forms, command detail panes, or richer rendering of `input_hint`.
- Changing the global command panel, plan editor slash menu, passthrough mode, terminal input, or shell behavior.
- Adding automatic execution confirmation prompts for slash commands.
