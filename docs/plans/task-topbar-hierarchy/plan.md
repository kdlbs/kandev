---
created: 2026-09-23
status: implemented
requirements:
  - REQ-UI-TASK-TOPBAR-001
system_design:
  - ../../specs/ui/system-design/task-topbar-hierarchy.md
legacy_specs: []
---

# Task topbar hierarchy

## Overview and scope

Group workspace tools, collapse diagnostic meters, and compact assignment to
give task identity more room. Preserve domain behavior and phone composition.
The user requested full AFK delivery through a PR with seeded screenshots, so
this package proceeds to implementation in the same session. No delegation.

## Technical approach

Use existing Popover/Drawer primitives and existing tools, assignment, and metric
components. Do not add libraries, new settings, or backend state. Build and test
from the task branch, which started at freshly fetched origin/main `9874ac4cf`.
PR screenshots must demonstrate the changed branch, using isolated E2E fixtures;
the marketing skill's unchanged-main capture rule does not apply to these
before/after implementation proofs. No landing assets or personal instance.

## ASCII UI preview

UI-01: Desktop task header, closed. Structural order; spacing is illustrative.

```text
repo > Task title            [Workflow]  [Metrics] [Plugins] [Avatar] [PR] [Panels] [Tools] [...]
                                                                                |
                                                   +----------------------------+
                                                   | Task tools                 |
                                                   | Layout       [selector]    |
                                                   | Workspace [editor v] [dir] |
                                                   +----------------------------+
```

UI-02: Phone task, existing composition. Task navigation and tools use native
menus. Status opens an inset drawer when enabled; no desktop toolbar is mounted.

```text
[Menu] [Task title v] [Step v] [...]
|             Active conversation          |
|                                         |
| [Chat] [Changes] [Files] [Terminal]       |
```

Touching new desktop-chrome disclosures on a coarse-pointer wide viewport opens
an inset drawer with fixed title, scrolling body, and 44 px controls. Metrics
loading uses the existing metric renderer and no invented values.

## Tests and E2E

AC .1-.4: focused desktop Playwright coverage for long titles, opening tools,
nested layout selection, metrics preferences, and compact assignment; preserve
existing tests for moved controls. AC .5: phone navigation regression and
coarse-pointer drawer geometry/interaction. Existing component tests cover
assignment errors, auth, archived state, and header domain wiring.

## Work orders

- [x] [Task 01: Focus task chrome](task-01-focus-task-chrome.md)

## Verification results

Implemented and checked: 32 unit tests, 38 desktop/touch scenarios, and four
phone scenarios passed. Six screenshots from a disposable Northstar workspace
were inspected for desktop, compact desktop, tools, metrics, wide touch, and
phone states. The prepared workspace, fictional assignee, conversation, and
linked PR use real product APIs with mock providers. Capture teardown verified
the owned temp root was removed and the backend port closed. Exact commands
and the host code-server limitation are recorded in the work order.

## Risks

Nested editor/layout menus need their parent disclosure to stay mounted. Touch
sizes must not increase desktop density. Existing E2E selectors for moved
controls need to enter Task tools first.
