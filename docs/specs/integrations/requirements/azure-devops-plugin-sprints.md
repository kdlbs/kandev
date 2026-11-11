---
status: draft
system: integrations
created: 2026-11-10
owners:
  - kandev
---

# Azure DevOps Plugin Teams and Sprints Requirements

## Overview

The Azure DevOps connector plugin ([requirements](azure-devops-plugin.md))
browses Boards, work items, and pull requests. This capability adds the team
and sprint views that Azure Boards users work in daily.

## Terminology

- **Sprint:** an iteration of a team, with start and finish dates.
- **Backlog item:** a work item of the team's requirement backlog level (for
  example, a user story) that can have child tasks.
- **Taskboard:** the sprint view that lays out each backlog item's child tasks
  in columns by state.

## Requirements

### REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-SPRINTS-001: Teams and sprints

**Intent:** Users work through their own teams and sprints, as in Azure
Boards: they find their team without scrolling through every team of the
project, narrow work items to a team and sprint, and follow a sprint on a
taskboard. Requested by the owner on 2026-11-10 after testing on `contoso`,
where the project has hundreds of teams.

#### Acceptance criteria

- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-SPRINTS-001.1:** Every team selector shall list
  the teams the connected identity belongs to first, each marked **My teams**,
  before the other teams of the project, and shall keep the type-to-filter
  behavior. The system shall remember the team last chosen in each project for
  each user and workspace, and the Board and Sprints team selectors shall
  preselect it, else the user's first team. The optional work-item team filter
  keeps its own saved value.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-SPRINTS-001.2:** Work-item mode shall offer
  optional **Team** and **Sprint** filters. A team limits results to the team's
  area paths, with or without sub-areas as the team defines them; a sprint
  limits results to one of the team's iterations. The filters shall combine
  with the search box and the WIQL query, and saved views shall keep them.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-SPRINTS-001.3:** The browse page shall offer a
  **Sprints** mode with team, sprint, and person selectors. The sprint selector
  shall filter by typed text, tag each sprint as past, current, or future, open
  on the current sprint (the latest past sprint when there is none), and show
  its dates and the working days left, counted with the team's working days and
  days off.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-SPRINTS-001.4:** Sprints mode shall show the
  sprint as a taskboard: one row per backlog item in the sprint, in backlog
  order, with its child tasks in the team's taskboard columns, and one row for
  tasks without a parent in the sprint. Each row shall collapse and expand, and
  one control shall collapse or expand all rows.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-SPRINTS-001.5:** The person selector shall
  offer **All**, **Unassigned**, and each person assigned to a work item of the
  sprint, and shall show only the matching tasks, in the rows that contain them
  or whose backlog item matches.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-SPRINTS-001.6:** A taskboard card shall open the
  work-item detail and offer the same task actions as a work-item result. The
  taskboard is read-only: moving cards between columns is out of scope.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-SPRINTS-001.7:** On a phone, the selectors shall
  stack full width and each row shall show its tasks grouped by column, without
  horizontal scrolling of the page.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-SPRINTS-001.8:** When the team has no sprints,
  or the sprint has no work items, Sprints mode shall say so and keep the
  selectors usable.

## Out of scope

- Moving taskboard cards, sprint planning (assigning items to sprints),
  capacity, and burndown.
- Kandev host changes.

## System design

- [Azure DevOps Plugin Teams and Sprints System Design](../system-design/azure-devops-plugin-sprints.md)
