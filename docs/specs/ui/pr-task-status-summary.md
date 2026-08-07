---
status: building
created: 2026-08-06
owner: kandev
---

# PR Task Status Summary

## Why

Task pull-request indicators currently flatten the PR title, review state, CI state, and
mergeability into one pipe-delimited sentence. Long titles and several adjacent status values
make the hover disclosure slow to scan during a brief pointer interaction.

## What

- Hovering a task's PR indicator with a fine pointer, or focusing it with the keyboard, shows a
  compact structured summary instead of a pipe-delimited sentence.
- Each linked PR has a distinct entry with its PR number and title in a visual header. Long titles
  wrap within the summary rather than widening it beyond the viewport.
- Review, CI, and merge or terminal state appear as separate labelled rows when their source data
  is available. Each row combines readable text with a semantic icon and color; no meaning depends
  on color or icon alone.
- Known GitHub states use concise user-facing copy such as **Approved**, **Passed**, **In
  progress**, **Changes requested**, **Conflicts**, and **Ready to merge**. An unrecognized
  non-empty provider value remains visible instead of being dropped.
- Ready-to-merge copy uses the existing strict `isPRReadyToMerge` rule. Draft, terminal,
  review, check, mergeability, aggregate icon color, and multi-PR attention precedence do not
  change.
- A task with several linked PRs shows one consistently structured entry per PR, separated clearly,
  while retaining the existing aggregate icon color, PR count, and ready-to-merge attributes.
- The disclosure uses localized copy, readable line height, restrained semantic colors, and a
  viewport-contained width. It remains informational: opening it does not navigate, mutate PR
  state, fetch new detail, or change task-row activation.
- The shared task PR indicator uses the same summary in the desktop sidebar, Kanban cards, and rich
  task-list rows.
- On coarse-pointer phone and tablet layouts, task rows keep their existing primary tap behavior
  and passive PR indicator. Detailed touch interaction remains available through the existing PR
  status drawer after opening the task, so this visual refinement adds no hover-only required
  action or competing compact touch target.

## Scenarios

- **GIVEN** an open PR with an approval, successful CI, and clean mergeability, **WHEN** the user
  hovers or focuses its task PR indicator, **THEN** the summary separates the PR number and title
  from labelled **Review — Approved**, **CI — Passed**, and **Merge — Ready to merge** rows.
- **GIVEN** a PR with changes requested, failing CI, a merge conflict, a blocked state, or a
  behind-base state, **WHEN** its summary opens, **THEN** each available condition appears in its
  own readable row with matching semantic text and icon without changing the indicator's existing
  attention color.
- **GIVEN** a long PR title, **WHEN** its summary opens near a viewport edge, **THEN** the title
  wraps inside the collision-aware summary and the disclosure causes no document-level horizontal
  overflow.
- **GIVEN** several PRs linked to one task, **WHEN** the task PR summary opens, **THEN** every PR is
  identifiable by number and title and its available status rows are visually grouped beneath it.
- **GIVEN** a non-empty provider status Kandev does not recognize, **WHEN** the summary opens,
  **THEN** that value remains visible as fallback text; absent status fields do not produce empty
  rows.
- **GIVEN** a phone task-switcher drawer, **WHEN** a linked-PR task row renders and the user taps
  the row, **THEN** the existing task navigation and PR indicator remain usable without horizontal
  overflow or a new hover-dependent interaction.

## Out of scope

- Changing GitHub status derivation, merge readiness, polling, API contracts, persistence, or
  task-to-PR associations.
- Adding check-run lists, reviewer lists, comments, merge controls, or other full PR-detail content
  to the task indicator summary.
- Changing GitLab merge-request or Azure DevOps pull-request indicators.
- Adding a new phone/tablet task-row drawer or turning the compact PR indicator into a separate
  touch action.

## Implementation plan

[PR task status summary plan](../../plans/pr-task-status-summary/plan.md)
