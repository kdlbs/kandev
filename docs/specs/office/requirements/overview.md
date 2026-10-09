---
status: draft
system: office
created: 2026-04-25
owners:
  - cfl
---
# Office: Overview Requirements

## Overview

Kandev users today manually trigger every task execution, monitor each agent individually, and shepherd work through the kanban board one task at a time. There is no way for agents to work independently across tasks, delegate work, run recurring jobs, or roll progress up across related initiatives - all table-stakes for autonomous multi-agent workflows. Office adds an autonomy layer on top of kandev's existing task system: a coordinator agent manages a fleet of workers, picks up tasks, delegates subtasks, tracks costs, and reports progress. Users decide when to let agents run autonomously and when to drill into a single task for low-level details.

This spec is the top-level entry point for Office. It covers the workspace model, projects, configuration storage and sync, and the first-run onboarding wizard. Other Office surfaces (agents, skills, scheduler, costs, routines, inbox, assistant) live in sibling specs under `docs/specs/office/`.

## Requirements

### REQ-OFFICE-OVERVIEW-001: Office: Overview

**Intent:** Kandev users today manually trigger every task execution, monitor each agent
individually, and shepherd work through the kanban board one task at a time. There is no way for
agents to work independently across tasks, delegate work, run recurring jobs, or roll progress up
across related initiatives - all table-stakes for autonomous multi-agent workflows. Office adds an
autonomy layer on top of kandev's existing task system: a coordinator agent manages a fleet of
workers, picks up tasks, delegates subtasks, tracks costs, and reports progress. Users decide when
to let agents run autonomously and when to drill into a single task for low-level details. This spec
is the top-level entry point for Office. It covers the workspace model, projects, configuration
storage and sync, and the first-run onboarding wizard. Other Office surfaces (agents, skills,
scheduler, costs, routines, inbox, assistant) live in sibling specs under `docs/specs/office/`.

#### Acceptance criteria

- **AC-OFFICE-OVERVIEW-001.1:** A new route at `/office` is accessible from a top-level navigation link on the kandev homepage.
- **AC-OFFICE-OVERVIEW-001.2:** The `/office/*` routes use a full-replacement sidebar (replaces the default sidebar).
- **AC-OFFICE-OVERVIEW-001.3:** The sidebar layout:
- **AC-OFFICE-OVERVIEW-001.4:** **Workspace switcher** at the top - dropdown to switch between workspaces.
- **AC-OFFICE-OVERVIEW-001.5:** **Top actions**: New Task, Dashboard, Inbox.
- **AC-OFFICE-OVERVIEW-001.6:** **Work**: Tasks, Routines.
- **AC-OFFICE-OVERVIEW-001.7:** **Projects**: expandable project list with `+` to create.
- **AC-OFFICE-OVERVIEW-001.8:** **Agents**: expandable agent list with `+` to create. Each entry shows a status dot and channel indicators (Telegram, Slack icons) if configured.

## System design

The migrated technical source is split into [part 1](../system-design/overview-01.md), [part 2](../system-design/overview-02.md).

## Workspace configuration export

### REQ-OFFICE-CONFIG-EXPORT-001: Preview and download Office configuration

**Intent:** Operators can export workspace configuration from Preferences and
verify the exact files before downloading them.

#### Acceptance criteria

- **AC-OFFICE-CONFIG-EXPORT-001.1:** Preferences shall open the export page through navigation and direct URL entry for the selected workspace. Loading, no-workspace, empty and failed-load states shall be explicit; a failed request shall offer retry.
- **AC-OFFICE-CONFIG-EXPORT-001.2:** Preview paths and content shall match downloaded ZIP entries. Download shall contain exactly the selected files; no selection shall disable download. Export shall not modify configuration or expose secret/runtime-only data.
- **AC-OFFICE-CONFIG-EXPORT-001.3:** Workspace changes shall clear the previous preview and selection immediately. A late response shall not display or download another workspace's files. Download failures shall be visible without losing the current selection.
- **AC-OFFICE-CONFIG-EXPORT-001.4:** Phone users shall select files, inspect one preview at a time, return to the list and download with the same semantics as desktop. Controls shall be touch-accessible and content shall stay within the viewport.

The [configuration export design](../system-design/config-export.md) owns the
preview/download contract. Import and config-sync mutation behavior are unchanged.
