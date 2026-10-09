---
status: active
system: ui
created: 2026-09-21
owners:
  - kandev
---

# Task navigation requirements

## Overview

First-party task links consistently open the task workbench. UI owns this
reusable navigation interaction across features. Tasks retain identity,
relationships, authorization, and lifecycle ownership.

## Requirements

### REQ-UI-TASK-NAVIGATION-001: Consistent task navigation

**Intent:** Users follow task links without restarting the loaded application.

#### Acceptance criteria

- **AC-UI-TASK-NAVIGATION-001.1:** An ordinary click, keyboard activation, or touch on a first-party task-workbench link shall open the selected task without loading a new browser document.
- **AC-UI-TASK-NAVIGATION-001.2:** Newly generated workbench URLs shall use `/t/:taskId`, preserve requested session, layout, and other supported query context, and encode each value once. Existing `/tasks/:id` deep links shall remain usable.
- **AC-UI-TASK-NAVIGATION-001.3:** Task links shall preserve browser copy-link, modified-click, middle-click, and explicit new-tab behavior.
- **AC-UI-TASK-NAVIGATION-001.4:** Task navigation shall respect existing unsaved-change guards. Cancelled navigation shall keep the current task and disclosure intact. Completed same-tab dependency navigation shall dismiss its disclosure.
- **AC-UI-TASK-NAVIGATION-001.5:** Both dependency directions shall support the same destination behavior in the desktop popover and touch drawer. Touch rows shall have at least 44px hit targets; fine-pointer rows shall retain compact sizing.
- **AC-UI-TASK-NAVIGATION-001.6:** Browser Back shall return to the preceding task after an ordinary task-link navigation without a document reload.

## Out of scope

Task selection policy, task authorization, missing-task recovery, Office routes,
external links, plugin-authored URLs, and navigation to task listings retain
existing behavior. This contract does not change backend task APIs.

## Implementation plans

- [Task link navigation](../../../plans/task-link-navigation/plan.md)
