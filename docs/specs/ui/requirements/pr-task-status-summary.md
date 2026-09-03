---
status: active
system: ui
created: 2026-08-06
updated: 2026-08-26
owners:
  - kandev
---
# PR Task Status Summary Requirements

## Overview

Task pull-request indicators show a compact structured summary of each linked
pull request. The indicator must also work when a task row initially has only
the bounded task-status projection and not the full pull-request record.

## Requirements

### REQ-UI-PR-TASK-STATUS-SUMMARY-001: PR Task Status Summary

**Intent:** Task pull-request indicators currently flatten the PR title, review state, CI state, and mergeability into one pipe-delimited sentence. Long titles and several adjacent status values make the hover disclosure slow to scan during a brief pointer interaction.

#### Acceptance criteria

- **AC-UI-PR-TASK-STATUS-SUMMARY-001.1:** Hovering a task's PR indicator with a fine pointer, or focusing it with the keyboard, shows a compact structured summary instead of a pipe-delimited sentence.
- **AC-UI-PR-TASK-STATUS-SUMMARY-001.2:** Each linked PR has a distinct entry with its PR number and title in a visual header. Long titles wrap within the summary rather than widening it beyond the viewport.
- **AC-UI-PR-TASK-STATUS-SUMMARY-001.3:** Review, CI, and merge or terminal state appear as separate labelled rows when their source data is available. Each row combines readable text with a semantic icon and color; no meaning depends on color or icon alone.
- **AC-UI-PR-TASK-STATUS-SUMMARY-001.4:** Known GitHub states use concise user-facing copy such as **Approved**, **Passed**, **In progress**, **Changes requested**, **Conflicts**, and **Ready to merge**. An unrecognized non-empty provider value remains visible instead of being dropped.
- **AC-UI-PR-TASK-STATUS-SUMMARY-001.5:** Ready-to-merge copy uses the existing strict `isPRReadyToMerge` rule. Draft, terminal, review, check, mergeability, aggregate icon color, and multi-PR attention precedence do not change.
- **AC-UI-PR-TASK-STATUS-SUMMARY-001.6:** A task with several linked PRs shows one consistently structured entry per PR, separated clearly, while retaining the existing aggregate icon color, PR count, and ready-to-merge attributes.
- **AC-UI-PR-TASK-STATUS-SUMMARY-001.7:** The disclosure uses localized copy, readable line height, restrained semantic colors, and a viewport-contained width. Opening it does not navigate, mutate PR state, refresh the provider, or change task-row activation.
- **AC-UI-PR-TASK-STATUS-SUMMARY-001.8:** The shared task PR indicator uses the same summary in the desktop sidebar, Kanban cards, and rich task-list rows.
- **AC-UI-PR-TASK-STATUS-SUMMARY-001.9:** All status entries in one summary share label, icon, and status-text columns so Review, CI, merge, and terminal values start at the same horizontal position.
- **AC-UI-PR-TASK-STATUS-SUMMARY-001.10:** Secondary detail, including merge-queue position and estimated merge time, starts under the status text instead of under the label or icon.
- **AC-UI-PR-TASK-STATUS-SUMMARY-001.11:** Shared columns adapt to translated labels and wrapped values without a fixed label width that clips content.
- **AC-UI-PR-TASK-STATUS-SUMMARY-001.12:** GitHub, GitLab, and registered change-request providers that use the shared summary component receive the same row alignment without changing provider-specific state derivation or copy.
- **AC-UI-PR-TASK-STATUS-SUMMARY-001.13:** When hover or keyboard focus opens a GitHub PR indicator with only compact task-summary data, the disclosure shall appear. The system shall load the stored PR records for that task.
- **AC-UI-PR-TASK-STATUS-SUMMARY-001.14:** While the task-scoped load is pending, the disclosure shall show a localized loading state. On success, it shall replace that state with the structured summary without another user action.
- **AC-UI-PR-TASK-STATUS-SUMMARY-001.15:** Repeated or concurrent disclosure requests for the same task shall share one in-flight load. Loaded store data shall prevent later disclosure requests from starting another load.
- **AC-UI-PR-TASK-STATUS-SUMMARY-001.16:** When the task-scoped load fails or returns no PR records, the disclosure shall show a localized unavailable state. A later disclosure shall retry the load.
- **AC-UI-PR-TASK-STATUS-SUMMARY-001.17:** Each GitHub PR summary shall show the non-empty PR author login below the PR title. A missing author login shall not create an empty label.
- **AC-UI-PR-TASK-STATUS-SUMMARY-001.18:** On a coarse pointer, the task row shall remain the primary touch target. After task navigation, the existing PR-status drawer shall show the same author login.
- **AC-UI-PR-TASK-STATUS-SUMMARY-001.19:** TaskPR API and WebSocket payloads shall carry the owning workspace ID. The backend shall route typed PR events by that ID, and the frontend shall ignore missing or mismatched updates before changing the active workspace cache.

## Migrated source detail

## Why

Task pull-request indicators currently flatten the PR title, review state, CI state, and
mergeability into one pipe-delimited sentence. Long titles and several adjacent status values
make the hover disclosure slow to scan during a brief pointer interaction.

The structured disclosure currently sizes each row label independently. Status values therefore
do not align, and secondary merge-queue text begins under the icon instead of the status text.

## What

- Hovering a task's PR indicator with a fine pointer, or focusing it with the keyboard, shows a
  compact structured summary instead of a pipe-delimited sentence.
- Each linked PR has a distinct entry with its PR number and title in a visual header. Long titles
  wrap within the summary rather than widening it beyond the viewport.
- Review, CI, and merge or terminal state appear as separate labelled rows when their source data
  is available. Each row combines readable text with a semantic icon and color; no meaning depends
  on color or icon alone.
- All status entries in one summary share three columns: label, fixed icon, and status text. Review,
  CI, merge, and terminal values start at the same horizontal position even when labels differ in
  width.
- Secondary detail, such as merge-queue position and estimated merge time, starts under the status
  text. It does not start under the icon or label.
- The shared columns adapt to translated labels and wrapped values. The layout does not use a fixed
  label width that clips a longer locale.
- Known GitHub states use concise user-facing copy such as **Approved**, **Passed**, **In
  progress**, **Changes requested**, **Conflicts**, and **Ready to merge**. An unrecognized
  non-empty provider value remains visible instead of being dropped.
- Ready-to-merge copy uses the existing strict `isPRReadyToMerge` rule. Draft, terminal,
  review, check, mergeability, aggregate icon color, and multi-PR attention precedence do not
  change.
- A task with several linked PRs shows one consistently structured entry per PR, separated clearly,
  while retaining the existing aggregate icon color, PR count, and ready-to-merge attributes.
- The disclosure uses localized copy, readable line height, restrained semantic colors, and a
  viewport-contained width. It remains informational and does not mutate provider state.
- If the browser has only compact task-summary data, hover or keyboard focus loads the stored PR
  records for that task. The disclosure shows loading and unavailable states without closing.
- Repeated requests for the same task share one in-flight load. Loaded task-store data prevents
  another load.
- Each GitHub PR entry shows its non-empty author login below the title.
- The shared task PR indicator uses the same summary in the desktop sidebar, Kanban cards, and rich
  task-list rows.
- GitHub, GitLab, and registered change-request providers that use the shared summary component get
  the same row alignment. Provider-specific state derivation and copy remain unchanged.
- On coarse-pointer phone and tablet layouts, task rows keep their existing primary tap behavior
  and passive PR indicator. Detailed touch interaction remains available through the existing PR
  status drawer after opening the task, so this visual refinement adds no hover-only required
  action or competing compact touch target.

## Scenarios

- **GIVEN** an open PR with an approval, successful CI, and clean mergeability, **WHEN** the user
  hovers or focuses its task PR indicator, **THEN** the summary separates the PR number and title
  from labelled **Review: Approved**, **CI: Passed**, and **Merge: Ready to merge** rows, and all
  three status values have the same horizontal start.
- **GIVEN** a PR with changes requested, failing CI, a merge conflict, a blocked state, or a
  behind-base state, **WHEN** its summary opens, **THEN** each available condition appears in its
  own readable row with matching semantic text and icon without changing the indicator's existing
  attention color.
- **GIVEN** a long PR title, **WHEN** its summary opens near a viewport edge, **THEN** the title
  wraps inside the collision-aware summary and the disclosure causes no document-level horizontal
  overflow.
- **GIVEN** a pointer-hovered PR indicator also has keyboard focus, **WHEN** the pointer leaves,
  **THEN** the summary remains visible until focus leaves or the user dismisses it with Escape.
- **GIVEN** several PRs linked to one task, **WHEN** the task PR summary opens, **THEN** every PR is
  identifiable by number and title and its available status rows are visually grouped beneath it.
- **GIVEN** a non-empty provider status Kandev does not recognize, **WHEN** the summary opens,
  **THEN** that value remains visible as fallback text; absent status fields do not produce empty
  rows.
- **GIVEN** merge-queue detail appears below **Awaiting checks**, **WHEN** the summary opens,
  **THEN** that detail starts under **Awaiting checks** and wraps within the status column.
- **GIVEN** translated row labels have different lengths, **WHEN** the summary renders, **THEN** the
  longest available label defines the shared label column and no label or status is clipped.
- **GIVEN** a task row has only compact PR status, **WHEN** a fine pointer hovers the PR indicator
  or keyboard focus reaches it, **THEN** a loading disclosure opens and then shows the full stored
  PR summary without another user action.
- **GIVEN** two mounted surfaces request the same task PR details, **WHEN** their requests overlap,
  **THEN** they share one in-flight load and preserve newer store data that arrives first.
- **GIVEN** a stored PR has an author login, **WHEN** the task summary or mobile PR-status drawer
  opens, **THEN** the author login appears below the PR title.
- **GIVEN** a phone task-switcher drawer, **WHEN** a linked-PR task row renders and the user taps
  the row, **THEN** the existing task navigation and PR indicator remain usable without horizontal
  overflow or a new hover-dependent interaction.

## Out of scope

- Changing provider polling, persistence, or task-to-change-request associations.
- Refreshing GitHub when the task-row disclosure opens.
- Loading all workspace PR records to satisfy one task-row disclosure.
- Adding check-run lists, reviewer lists, comments, merge controls, or other full PR-detail content
  to the task indicator summary.
- Changing GitHub, GitLab, or registered-provider status derivation, icon precedence, or copy.
- Adding a new phone/tablet task-row drawer or turning the compact PR indicator into a separate
  touch action.

## Implementation plan

[PR task status summary plan](../../../plans/pr-task-status-summary/plan.md)

[Sidebar task row presentation refinement](../../../plans/sidebar-task-row-presentation/plan.md)

[PR task status hover hydration](../../../plans/pr-task-status-hover-hydration/plan.md)
