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
