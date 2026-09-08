---
status: active
system: workspaces
created: 2026-04-29
owners:
  - Carlos Florencio
---
# Improve Kandev Requirements

## Overview

Users who hit a bug or have a feature idea today have no in-app way to report it, and even when they do, the report sits as text someone else has to act on. Make filing an improvement a one-click action that produces a real, actionable task the user's own agent picks up immediately — turning every report into a contribution.

## Requirements

### REQ-WORKSPACES-IMPROVE-KANDEV-001: Improve Kandev

**Intent:** Users who hit a bug or have a feature idea today have no in-app way to report it, and even when they do, the report sits as text someone else has to act on. Make filing an improvement a one-click action that produces a real, actionable task the user's own agent picks up immediately — turning every report into a contribution.

#### Acceptance criteria

- **AC-WORKSPACES-IMPROVE-KANDEV-001.1:** The **Improve Kandev** action in the desktop app-sidebar footer opens a task-creation dialog that is pre-configured for the kandev codebase: repository locked to `https://github.com/kdlbs/kandev`, base branch `main`, workflow selected from the hidden Improve Kandev workflows, and description seeded with a starter template. On phones the same action is a 44px-or-larger row in the existing mobile home menu's **Utilities** section.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.2:** The dialog reuses the existing task-create UI, including prompt enhancement, image paste, and file attachments.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.3:** The dialog explains the flow up front: the agent will implement the change, the user will test it, then the agent opens a PR. Brief copy positions this as the user contributing to kandev's future.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.4:** The explanation includes a "Do not show this again" preference. Once selected, later uses of **Improve Kandev** skip the explanation and open the pre-configured task-creation dialog directly. The preference is local to the current browser profile and can be cleared with other local UI state.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.5:** The task-creation dialog offers three report kinds: **Bug fix**, **Feature request**, and **Open issue**. Bug fixes and feature requests use the existing implementation workflow. Open issue uses a separate hidden, one-step workflow and visibly explains that the agent only publishes a GitHub issue; it does not implement the change or open a pull request.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.6:** An "Include recent logs" toggle (default on) attaches a context bundle to the task: recent backend logs, frontend logs, and a metadata snapshot. The bundle lives in a temporary folder and is referenced by file path in the task description so the agent can read it on demand.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.7:** Submitting the dialog creates the task in the dedicated **Improve Kandev** workspace, clones the kandev repo if needed, and starts the agent on the first step.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.8:** The dedicated workspace is created automatically on first bootstrap and reused on every later use, keeping improve tasks isolated and segregated from the user's regular work. It is named `Improve Kandev`, is a normal visible workspace (with a kanban workflow), and persists across restarts.

## System design

The migrated technical source is split into [part 1](../system-design/improve-kandev.md).
