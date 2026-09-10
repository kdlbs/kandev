---
status: active
system: ui
created: 2026-07-25
owners:
  - Kandev
---
# Task Listing Display Preferences Requirements

## Overview

People can remember their last task-listing mode on each device or explicitly
choose Threads as their Home destination. Choosing another mode for the current
interaction must not replace that explicit default. List users can also choose
richer rows without losing the compact default.

UI owns this reusable presentation and navigation preference contract. It does
not change task lifecycle or workspace ownership. The Threads destination uses
the existing [conversation deck](threads-conversation-deck.md) and
[saved views](threads-saved-views.md).

## Terminology

- **Remembered listing:** The most recently used Kanban, Pipeline, List, or
  Threads mode in this browser on this device.
- **Startup choice:** The user's saved choice of Task overview, Last visited
  task, or Threads in Settings > Preferences > Appearance > Startup Page.
- **Bare Home:** Opening Kandev's root without an explicit task, session,
  workflow, or overview destination. Naming only a workspace still permits the
  startup choice to apply.
- **Home action:** A Home link, brand link, Home command, or settings exit that
  returns to the active workspace's home. An explicit Task overview or Back
  destination remains separate.

## Requirements

### REQ-UI-TASK-LISTING-DISPLAY-PREFERENCES-001: Task Listing Display Preferences

**Intent:** Preserve independent device presentation choices and portable list
detail settings.

#### Acceptance criteria

- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.1:** Selecting **All Workflows** remains selected in Kanban and List even when the workspace has exactly one visible workflow.
- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.2:** The selected workflow filter continues to use the existing portable user setting and survives navigation and reloads.
- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.3:** The remembered listing shall
  support Kanban, Pipeline, List, and Threads, survive browser restarts, and
  remain device-local rather than syncing through the user's portable settings.
- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.4:** When no fixed Home default or
  eligible startup task applies, opening the Home task surface shall restore
  the remembered listing.
- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.5:** An invalid remembered listing
  shall fall back to Kanban. A missing or unavailable preference shall also
  fall back to Kanban unless the existing legacy Pipeline preference supplies
  the compatibility fallback.
- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.6:** Pipeline remains unavailable on phone-sized layouts. A phone temporarily renders Kanban when Pipeline is the saved device preference without overwriting that saved preference.
- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.7:** List view adds a **Show task details** display option. It is disabled by default and appears in both desktop and mobile display-options surfaces while List is active.
- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.8:** Enabling **Show task details** enriches each list row with the same useful task context surfaced by Kanban cards when that data exists: repository slug chips, a truncated description, pull-request status, session count, parent-task context, and review-attention state.

- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.9:** With task details disabled,
  compact rows shall retain their title, task-state icon, updated time, and
  actions. Missing rich metadata shall omit only that metadata, not the row.
- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.10:** The richer-row choice shall
  survive reloads and apply on the user's other signed-in devices. After a
  failed save, an authoritative reload shall restore the persisted value.
- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.11:** Clicking or tapping a rich
  row shall open its task. Archive, delete, and interactive metadata actions
  shall remain independent, without horizontal document overflow on phones.
- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.12:** When browser storage rejects
  a view change, the chosen view shall remain usable for the current document;
  reopening the app shall use the available saved preference or fallback.

### REQ-UI-TASK-LISTING-DISPLAY-PREFERENCES-002: Startup compatibility

**Intent:** Keep existing startup choices and workspace-local recent-task
behavior while extending the choice with Threads.

#### Acceptance criteria

- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-002.1:** Task overview shall remain
  the default startup choice. Existing users shall retain their saved Task
  overview or Last visited task choice.
- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-002.2:** With Last visited task
  selected, bare Home shall open the newest local recent task belonging to the
  resolved active workspace. A newer entry from another workspace shall not
  become the target.
- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-002.3:** If no matching local recent
  task is available, including when local storage cannot be read, Last visited
  task shall fall back to the remembered listing and its existing fallback.
- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-002.4:** Last visited task shall not
  resume a task on a Home action, a task's Back action, or an explicit task,
  session, workflow, or overview destination.
- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-002.5:** Missing or unsupported saved
  startup choices shall normalize to Task overview. An unsupported submitted
  choice shall fail without replacing the previous saved choice, and an
  unrelated settings save shall leave the startup choice unchanged.

### REQ-UI-TASK-LISTING-DISPLAY-PREFERENCES-003: Explicit Threads Home default

**Intent:** Let a user consistently return to Threads even after using another
task-listing mode.

#### Acceptance criteria

- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.1:** Startup Page shall offer
  Threads alongside Task overview and Last visited task on desktop and phone.
  Visible localized descriptions shall explain remembered overview behavior,
  startup-only task resume, and Threads applying to both startup and Home.
- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.2:** Selecting Threads shall use
  the existing shared Settings Save changes and discard flow. Only a successful
  save shall change the durable default; failures shall retain a retryable
  draft and the prior saved default. Discard shall restore the saved choice.
- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.3:** The saved Threads choice
  shall survive browser and Kandev restarts and apply on the user's other
  signed-in devices without requiring a local remembered Threads value.
- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.4:** With Threads selected,
  bare Home and every Home action shall open Threads in the resolved workspace,
  including after the user has visited Kanban, Pipeline, or List. Saving the
  choice shall not itself change the remembered listing; subsequent view
  navigation shall not change the saved startup choice.
- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.5:** An explicit view selection,
  task, session, workflow, overview, or Threads focus link shall retain its
  destination and scope during navigation, reload, and browser history use.
  An explicit overview continues to mean the remembered task listing; the
  fixed Threads choice shall not override it. Saving a default while another
  destination is open shall not redirect that page.
- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.6:** Home shall retain the active
  or explicitly requested workspace. Selecting another non-Office workspace
  shall apply Threads to that selected workspace. Office workspace Home shall
  continue to open Office. No-workspace recovery shall remain available.
- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.7:** An empty Threads result
  shall stay in Threads and display its existing empty state. Unavailable or
  malformed browser listing storage shall not prevent a saved Threads Home
  choice from working.
- **AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.8:** Phone users shall be able to
  select and save Threads with touch, use Home to reach the existing native
  mobile deck, and reload it. The settings option shall have an accessible
  label and a touch target at least 44 CSS pixels high, with visible save
  controls, safe-area clearance, and no horizontal document overflow.

## Out of scope

- Syncing the remembered listing or exact recent task between devices.
- Separate fixed defaults for Kanban, Pipeline, or List; those remain reachable
  through Task overview's remembered mode.
- New per-workspace startup settings or changing workspace selection ownership.
- Choosing a particular saved Threads filter, task, or session as Home.
- Adding a phone-native Pipeline visualization.
- User-configurable selection or ordering of individual rich-row metadata
  fields.
- Changing existing sort, group, archive, pagination, or preview-panel
  behavior.
- Changing a task's explicit Back/Task overview destination to generic Home.
- Swipe-indicator timing or mobile topbar normalization from the parent task.
- Parent worktree changes, parent demo data, or launching other task sessions.
- Removing the legacy `kanban_view_mode` API field in this change.
