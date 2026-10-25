---
status: active
system: ui
created: 2026-08-07
owners:
  - Kandev
---
# GitHub Saved-Query Default Views Requirements

## Overview

Saved GitHub queries can capture a team's useful pull-request or issue view, including its repository filter, but users must select that view again every time they enter the dashboard or switch result kind. A saved query should be able to become the default view for its result kind.

## Requirements

### REQ-UI-GITHUB-SAVED-QUERY-DEFAULTS-001: GitHub Saved-Query Default Views

**Intent:** Saved GitHub queries can capture a team's useful pull-request or issue view, including its repository filter, but users must select that view again every time they enter the dashboard or switch result kind. A saved query should be able to become the default view for its result kind.

#### Acceptance criteria

- **AC-UI-GITHUB-SAVED-QUERY-DEFAULTS-001.1:** GitHub saved queries may be marked as the default view.
- **AC-UI-GITHUB-SAVED-QUERY-DEFAULTS-001.2:** Pull requests and issues each own an independent default saved query.
- **AC-UI-GITHUB-SAVED-QUERY-DEFAULTS-001.3:** `/github` continues to open on pull requests. It applies the pull-request default when one exists, otherwise the first configured pull-request query.
- **AC-UI-GITHUB-SAVED-QUERY-DEFAULTS-001.4:** Switching between pull requests and issues applies the destination kind's default saved query, otherwise its first configured query.
- **AC-UI-GITHUB-SAVED-QUERY-DEFAULTS-001.5:** Applying a saved default restores both its custom GitHub query and repository filter.
- **AC-UI-GITHUB-SAVED-QUERY-DEFAULTS-001.6:** Setting or clearing a default affects future dashboard entry and kind switches. It does not replace the view currently on screen.
- **AC-UI-GITHUB-SAVED-QUERY-DEFAULTS-001.7:** A default marker and set/clear action appear beside each saved query in the desktop menu and mobile filter surface.
- **AC-UI-GITHUB-SAVED-QUERY-DEFAULTS-001.8:** New saved queries are not defaults until explicitly marked.

## Migrated source detail

## Why

Saved GitHub queries can capture a team's useful pull-request or issue view,
including its repository filter, but users must select that view again every
time they enter the dashboard or switch result kind. A saved query should be
able to become the default view for its result kind.

## What

- GitHub saved queries may be marked as the default view.
- Pull requests and issues each own an independent default saved query.
- `/github` continues to open on pull requests. It applies the pull-request
  default when one exists, otherwise the first configured pull-request query.
- Switching between pull requests and issues applies the destination kind's
  default saved query, otherwise its first configured query.
- Applying a saved default restores both its custom GitHub query and repository
  filter.
- Setting or clearing a default affects future dashboard entry and kind
  switches. It does not replace the view currently on screen.
- A default marker and set/clear action appear beside each saved query in the
  desktop menu and mobile filter surface.
- New saved queries are not defaults until explicitly marked.

## Persistence

The default marker is stored with the existing saved-query object. Workspace
GitHub settings own it when a workspace is active; portable user settings own
it when no workspace is active. Copying GitHub workspace settings therefore
copies default saved views together with saved queries.

Legacy saved-query objects without a marker remain valid and are treated as
non-default. At most one saved query per kind is effective as default. If
malformed persisted data marks several defaults for one kind, the first valid
entry remains default and later entries are normalized to non-default.

## Failure modes

- A failed default update leaves the prior persisted and rendered markers in
  place, surfaces an error, and permits retry.
- Deleting the active saved query keeps the existing first-configured-query
  fallback.
- Deleting an inactive default saved query removes that future default without
  changing the current view.
- If a kind has no configured queries and no saved default, it opens with an
  empty query and no repository filter.

## Mobile contract

The existing GitHub mobile filter sheet remains the entry point. Each saved
query row exposes a visible, accessible 44px default action alongside its
primary select action. Default and delete actions do not select the row. The
sheet keeps one vertical scroll owner and must not create document-level
horizontal overflow.

## Scenarios

- **GIVEN** a saved pull-request query is the pull-request default, **WHEN** the
  user enters `/github`, **THEN** its label, custom query, and repository filter
  are active.
- **GIVEN** independent pull-request and issue defaults, **WHEN** the user
  switches kinds, **THEN** the destination kind's saved default becomes active.
- **GIVEN** no issue default, **WHEN** the user switches to issues, **THEN** the
  first configured issue query is active with all repositories selected.
- **GIVEN** a non-active saved query, **WHEN** the user marks it default,
  **THEN** its marker updates after persistence succeeds while the current view
  stays unchanged.
- **GIVEN** a default update fails, **WHEN** the request settles, **THEN** the
  old default marker remains and an error is shown.
- **GIVEN** a phone viewport, **WHEN** the user taps the saved query's default
  action, **THEN** the marker updates without selecting the row, clipping the
  action, or causing horizontal overflow.

## Out of scope

- Choosing built-in configured queries as defaults.
- Remembering the last open kind or changing the initial kind from pull requests.
- GitLab saved-query defaults.
- A separate settings page or save-dialog-only default control.
- Renaming the existing GitHub **Default queries** configuration.

## Implementation plan

[Implementation plan](../../../plans/github-saved-query-defaults/plan.md)
