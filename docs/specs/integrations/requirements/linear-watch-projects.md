---
status: active
system: integrations
created: 2026-10-18
owners:
  - kandev
---

# Linear Watch Project Requirements

## Overview

A Linear team often has several projects, each backed by its own repository,
and keeps each project's product brief in the project description. A Linear
issue watch must be able to route by project instead of by labels that
duplicate it, and a watch prompt must be able to give the agent the project's
context next to the issue text.

As a workspace owner, I want a Linear watch to match only issues in selected
projects and to include the project brief in the task prompt, so that each
project's issues start in the right repository with the right context.

## Terminology

- **Project filter:** The `projectIds` list of a watch's Linear search filter.
- **Project summary:** Linear's short project description.
- **Project description:** The project's markdown content shown on the Linear
  project page.
- **Project placeholders:** `{{issue.project.name}}`, `{{issue.project.url}}`,
  `{{issue.project.summary}}`, and `{{issue.project.description}}`.

## Requirements

### REQ-INTEGRATIONS-LINEAR-PROJECTS-001: Project filter for Linear watches

**Intent:** Let a watch match issues by Linear project, combined with its other
filters.

#### Acceptance criteria

- **AC-INTEGRATIONS-LINEAR-PROJECTS-001.1:** When a watch filter lists one or
  more project IDs, the watch shall match only issues that belong to any of
  those projects and that also satisfy every other filter field.
- **AC-INTEGRATIONS-LINEAR-PROJECTS-001.2:** A project filter alone shall
  satisfy the rule that a watch filter is not empty. Blank project IDs shall be
  dropped before that rule is checked. A filter without project IDs shall send
  the same Linear search conditions as before.
- **AC-INTEGRATIONS-LINEAR-PROJECTS-001.3:** When a team is selected in the
  watch dialog, the dialog shall list every non-archived project accessible to
  that team and let the user toggle projects. Saving shall store the selected
  project IDs. Reopening and saving an existing watch shall keep its project
  IDs. Changing the team shall clear the selected projects, as it clears
  states and labels, and switching the workspace shall clear them. A selected
  project that the team no longer lists shall stay visible as a removable
  choice, including when a saved project-only watch has no team selected.
  Without a team, the dialog shall not request project inventory. Project
  choices shall have a hit area of at least 44 px on coarse
  pointers. Desktop and phone layouts shall behave the same.

### REQ-INTEGRATIONS-LINEAR-PROJECTS-002: Project context in watch prompts

**Intent:** Let a watch prompt carry the issue's project name, link, summary,
and bounded project description.

#### Acceptance criteria

- **AC-INTEGRATIONS-LINEAR-PROJECTS-002.1:** When a watch creates a task for an
  issue in a project, `{{issue.project.name}}`, `{{issue.project.url}}`, and
  `{{issue.project.summary}}` shall render that project's name, URL, and
  summary. For an issue without a project, every project placeholder shall
  render as an empty string.
- **AC-INTEGRATIONS-LINEAR-PROJECTS-002.2:** When a watch prompt contains
  `{{issue.project.description}}`, the placeholder shall render the project
  description captured when the poll found the issue. A watch whose prompt
  does not contain that placeholder shall not request project descriptions. If
  the description request fails, that poll shall create no tasks and the next
  poll shall retry. A successful response that omits a project shall render an
  empty description for that project.
- **AC-INTEGRATIONS-LINEAR-PROJECTS-002.3:** A project description longer than
  4,000 characters shall render as its first 4,000 characters followed by a
  notice that the description was truncated. A description of at most 4,000
  characters shall render unchanged.
- **AC-INTEGRATIONS-LINEAR-PROJECTS-002.4:** A prompt without project
  placeholders shall render exactly as before. Placeholder text inside a
  substituted value shall not be expanded again. The prompt editor shall offer
  the four project placeholders with the existing issue placeholders.

Placeholders must appear literally in the watch prompt. Saved-prompt
references expand later during prompt admission; provider placeholders inside
that appended content are not interpolated.

## Out of scope

- Project filters in the `/linear` issue browser and in task launch.
- Project milestones, initiatives, project labels, or project status filters.
- Refreshing a running task's prompt after the project description changes.
- Changes to the default watch prompt, persistence schema, or dispatch order.

## Implementation plans

- [Linear watch project filter](../../../plans/linear-watch-project-filter/plan.md)
