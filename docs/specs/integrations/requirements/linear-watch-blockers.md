---
status: active
system: integrations
created: 2026-10-19
owners:
  - kandev
---

# Linear Watch Blocker Requirements

## Overview

Teams plan work in Linear with "blocked by" relations. A Linear issue watch
starts tasks in its dispatch order (for example oldest first), and that order
often contradicts the dependency graph: an issue created early can depend on
one created later. Without a dependency check the watch starts issues before
the work they depend on is finished.

As a workspace owner, I want a Linear watch to start an issue only after every
issue that blocks it is completed, so that tasks follow the dependency order I
planned in Linear.

## Terminology

- **Blocker:** An issue linked to another issue by a Linear relation of type
  `blocks`; the other issue is "blocked by" it.
- **Completed blocker:** A blocker whose workflow state type is `completed`.
- **Held issue:** An issue that matches a watch filter but that the watch does
  not start yet because a blocker is not completed.
- **Blocker setting:** The `requireBlockersDone` flag of a watch's Linear
  search filter.

## Requirements

### REQ-INTEGRATIONS-LINEAR-BLOCKERS-001: Hold issues until their blockers are completed

**Intent:** Let a watch defer an issue until Linear reports every blocker as
completed, without changing the watch's matching or ordering otherwise.

#### Acceptance criteria

- **AC-INTEGRATIONS-LINEAR-BLOCKERS-001.1:** When the blocker setting is on, a
  poll shall not start a task for an issue that has at least one blocker whose
  state type is not `completed`.
- **AC-INTEGRATIONS-LINEAR-BLOCKERS-001.2:** From the first poll after all of
  its blockers are completed, a held issue shall be dispatched like any other
  matched issue, in the watch's dispatch order and under its in-flight cap. A
  held issue shall not take an in-flight slot and shall not
  be recorded as seen. A poll shall still scan the later result pages of the
  watch, so issues that can start are not hidden behind held ones. A held
  issue, and a released issue whose dispatch the in-flight cap defers, shall
  be re-checked on later polls even after it has left the watch's paged
  search window, for up to 250 such issues per watch, until a task is started
  for it or it no longer matches the watch filter.
- **AC-INTEGRATIONS-LINEAR-BLOCKERS-001.3:** A canceled blocker shall keep the
  issue held. Blockers in another team or project shall count like any other
  blocker. When Linear reports more blocker relations than the watch fetched,
  the issue shall be held.
- **AC-INTEGRATIONS-LINEAR-BLOCKERS-001.4:** The watch shall request issue
  relations from Linear only when the blocker setting is on, with a bounded
  number of relations per issue and a bounded number of re-check requests
  per poll. The blocker setting shall not be sent to
  Linear as a search condition.
- **AC-INTEGRATIONS-LINEAR-BLOCKERS-001.5:** When the blocker setting is off,
  including for every existing watch, polls shall send the same Linear
  requests and start the same issues as before. The blocker setting alone
  shall not satisfy the rule that a watch filter is not empty.

### REQ-INTEGRATIONS-LINEAR-BLOCKERS-002: Blocker setting in the watch dialog

**Intent:** Let a user turn the blocker setting on or off for a watch.

#### Acceptance criteria

- **AC-INTEGRATIONS-LINEAR-BLOCKERS-002.1:** The watch dialog shall offer an
  "Only start issues whose blockers are done" switch with the filter fields.
  Saving shall store its value in the watch filter. Reopening and saving an
  existing watch shall keep the stored value. Changing the team or workspace
  shall not change it. The watch table shall mark a watch with the setting on.
  On coarse pointers the switch shall have a hit area of at least 44 px.
  Desktop and phone layouts shall behave the same.

## Out of scope

- Other relation types (`related`, `duplicate`, `similar`) and sub-issue
  hierarchy.
- Treating a canceled blocker as resolved, or a per-watch choice for it.
- Blocker checks in the `/linear` issue browser, in task launch, or for tasks
  that already started.
- Showing held issues or their blockers in the UI.

## Implementation plans

- [Linear watch blockers](../../../plans/linear-watch-blockers/plan.md)
