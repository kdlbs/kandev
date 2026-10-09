---
id: canvases-task-canvas-workspace-preview
title: Task canvas workspace data preview
status: draft
system: canvases
owners:
  - canvases
created: 2026-09-23
last_updated: 2026-09-23
---

# Task canvas workspace data preview Requirements

## Overview

A user creates a canvas from a task and reviews the live application there
before deciding whether to add it to workspace navigation. Its task placement
must not reduce the data available to an owner-authorized first release.
Canvases owns the creation and promotion lifecycle; the Plugins system owns the
runtime data contract and grant enforcement.

## Terminology

- **Placement scope:** Where Kandev presents a canvas and which task owns its
  unpromoted lifecycle.
- **Data scope:** The largest Kandev data boundary admitted by a release's
  declared and granted capabilities. Data scope never overrides the viewing
  user's current authorization.

## Requirements

### REQ-CANVASES-WORKSPACE-PREVIEW-001: Representative task canvas preview

**Intent:** A locally requested canvas works with the current workspace's live
data before the user chooses to promote it.

**User story:** As a user creating a canvas from a task, I want to see all tasks
in my current workspace, so that I can judge the canvas before promotion.

#### Acceptance criteria

- **AC-CANVASES-WORKSPACE-PREVIEW-001.1:** After the first valid release of an
  owner-authorized task canvas activates, its declared task read capability
  shall list and read all currently authorized tasks in the canvas's workspace,
  including tasks added after publication. Refresh and pagination shall reach
  the full result set. A task in another workspace shall remain unavailable.
- **AC-CANVASES-WORKSPACE-PREVIEW-001.2:** Before promotion, every supported
  capability declared by that first release shall operate across the same
  workspace where the capability has a workspace data meaning. Undeclared or
  unsupported capabilities shall remain unavailable, and every operation shall
  obey current user authorization.
- **AC-CANVASES-WORKSPACE-PREVIEW-001.3:** The canvas shall remain discoverable
  only in its originating task until a user promotes it. Task removal shall
  retain its existing cleanup behavior. Promotion shall still require user
  confirmation, but it shall not be required to preview workspace data.
- **AC-CANVASES-WORKSPACE-PREVIEW-001.4:** The canvas context and host review
  shall distinguish task placement from workspace data access. Promotion review
  shall identify whether it changes data access or only placement.
- **AC-CANVASES-WORKSPACE-PREVIEW-001.5:** An existing task canvas with
  task-only data access shall keep that access after upgrade until a current
  workspace owner explicitly enables workspace data access. The review shall
  name the active release and its declared permissions. Confirmation shall
  widen only those declared grants without promoting or republishing the
  canvas; cancellation shall change nothing.
- **AC-CANVASES-WORKSPACE-PREVIEW-001.6:** A stale review, changed owner,
  revoked grant, missing task or workspace, or failed permission check shall
  deny the access change without widening data exposure. A new release that
  adds a capability after first publication shall still await permission
  review.
- **AC-CANVASES-WORKSPACE-PREVIEW-001.7:** Desktop and phone users shall be
  able to inspect the data scope and complete the existing-canvas access review
  from the task canvas host. The phone flow shall remain usable with one
  vertical scroll region, safe-area clearance, and touch controls.
- **AC-CANVASES-WORKSPACE-PREVIEW-001.8:** A canvas built under this contract
  shall receive only workspace data authorized for its own workspace. Its
  runtime token, manifest, or agent input shall not select a different
  workspace or expand its capabilities.

## Out of scope

- Data from other workspaces or installation-wide grants.
- Blanket grants for capabilities a release does not declare.
- New canvas plugin capability types or arbitrary backend execution.
- Automatic widening of existing published releases.

## Implementation plans

- [Task canvas workspace data preview](../../../plans/task-canvas-workspace-preview/plan.md)
