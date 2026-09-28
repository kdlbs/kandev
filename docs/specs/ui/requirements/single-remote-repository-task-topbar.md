---
status: draft
system: ui
created: 2026-09-24
owners:
  - kandev
---

# Single remote repository task topbar requirements

## Overview

The task topbar identifies a task's repository, but its current `owner/repo`
label takes space and does not open the code host. UI owns this presentation and
navigation affordance. The workspace system remains the source of repository
identity, provider, and remote URL.

## Requirements

### REQ-UI-REMOTE-REPO-TOPBAR-001: Remote repository breadcrumb

**Intent:** A user working in a task with one remote repository can recognize
the repository and open it directly from the task topbar.

#### Acceptance criteria

- **AC-UI-REMOTE-REPO-TOPBAR-001.1:** When a task is linked to exactly one
  remote repository, its topbar shall present the external provider icon,
  repository name without its organization or owner prefix, a separator, and
  the task name, in that order. The task name shall retain its current rename
  or task-picker action.
- **AC-UI-REMOTE-REPO-TOPBAR-001.2:** The repository icon and name shall form
  one keyboard-accessible link to the repository's browser page. Activating it
  shall open the external page in a new tab without replacing the task page.
  The link's accessible name shall identify the repository and provider.
- **AC-UI-REMOTE-REPO-TOPBAR-001.3:** A task with no repository, a local
  repository, or more than one repository shall retain its current topbar
  repository presentation and navigation behavior. Repository labels in task
  cards, filters, and other surfaces shall retain their existing identities.
- **AC-UI-REMOTE-REPO-TOPBAR-001.4:** When a remote repository has no usable
  browser URL, the topbar shall still show its provider icon and short name,
  but shall not present an inert or unsafe link.
- **AC-UI-REMOTE-REPO-TOPBAR-001.5:** On a phone, the same repository
  label shall be visible beside the task picker, and its link shall be
  touch-reachable when a browser URL is available.
  Long names shall truncate without hiding the task picker or causing
  horizontal page overflow. The full repository identity shall remain
  available to assistive technology.

## Out of scope

- Changing stored repository names, task names, provider identities, or clone
  URLs.
- Changing the multi-repository selector or any repository label outside the
  task topbar.
