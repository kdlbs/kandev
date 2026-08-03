---
status: shipped
created: 2026-08-03
owner: kandev
---

# Quick Terminal

## Why

Users who need a short-lived host shell must navigate to **Settings → Agents** before they can
run a command. A terminal beside the existing New Task and Quick Chat entry points makes this
utility available without leaving the current workflow.

## What

- With an active workspace and the desktop sidebar expanded, the New Task row shows a
  `Quick terminal` icon immediately to the left of the existing `Quick Chat` icon.
- Activating `Quick terminal` opens a shell on the Kandev host using the same permissions,
  command environment, session lifecycle, and failure behavior as the terminal on
  **Settings → Agents**.
- On tablet and phone Home and Tasks surfaces, an equivalent `Quick terminal` icon appears
  immediately to the left of `Quick Chat`.
- On tablet and desktop, Quick Terminal opens as a floating dialog that is wider and taller than
  the terminal on **Settings → Agents**, while remaining contained within the viewport.
- On phone, Quick Terminal opens as a focused full-height surface with one terminal scroll owner,
  dynamic-viewport sizing, safe-area clearance, and a visible close action.
- The terminal accepts keyboard input, displays shell output and connection/session errors, and
  stops its host-shell session when the surface closes.
- Closing Quick Terminal returns focus to the launcher that opened it. A later launch starts or
  reconnects through the existing host-shell lifecycle; Quick Terminal does not create a durable
  task, terminal tab, or conversation.
- The shortcut is absent when no workspace is active. It is also absent from the collapsed desktop
  sidebar, matching the existing trailing Quick Chat action.

## Failure modes

- If the host-shell session cannot start or its stream cannot connect, the open Quick Terminal
  surface shows the existing terminal error state and remains dismissible.
- Closing the surface during startup cancels the pending start and does not leave an unseen shell
  session running.

## Scenarios

- **GIVEN** an active workspace and an expanded desktop sidebar, **WHEN** the New Task row renders,
  **THEN** `Quick terminal` appears immediately to the left of `Quick Chat`.
- **GIVEN** the desktop shortcut is visible, **WHEN** the user activates `Quick terminal`, **THEN**
  a host shell opens in a floating dialog that is wider and taller than the Agents-page terminal.
- **GIVEN** the tablet Home or Tasks header has an active workspace, **WHEN** it renders, **THEN**
  `Quick terminal` appears immediately to the left of `Quick Chat` and opens the same large floating
  terminal.
- **GIVEN** the phone Home or Tasks header has an active workspace, **WHEN** the user taps
  `Quick terminal`, **THEN** a full-height terminal surface opens within the dynamic viewport and
  the user can dismiss it back to the launcher.
- **GIVEN** Quick Terminal is open and the shell start or stream fails, **WHEN** the failure is
  reported, **THEN** the surface shows the error and still allows the user to close it.
- **GIVEN** Quick Terminal has an active or starting session, **WHEN** the user closes the surface,
  **THEN** the session is stopped or cancelled and no durable task, chat, or terminal tab is added.
- **GIVEN** the desktop sidebar is collapsed or no workspace is active, **WHEN** the relevant
  navigation surface renders, **THEN** it does not show an unusable Quick Terminal shortcut.

## Out of scope

- Task-workspace terminals, repository working-directory selection, or task terminal tabs.
- Multiple simultaneous Quick Terminal windows, persistent terminal history, or reconnecting a
  terminal across a Kandev restart.
- A keyboard shortcut, command-palette action, or additional task-switcher launch point.
- Changes to the existing host-shell backend API, authorization policy, or Agents-page terminal
  dimensions.

## Implementation plan

[Quick Terminal implementation](../../plans/quick-terminal/plan.md)
