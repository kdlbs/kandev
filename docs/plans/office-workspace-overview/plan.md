---
created: 2026-09-29
status: complete
requirements:
  - REQ-OFFICE-WORKSPACE-OVERVIEW-001
system_design:
  - ../../specs/office/system-design/workspace-overview.md
---

# Implementation Plan: Office Workspace Overview

## Overview

Office users can review several Office workspaces on one page. The page reads the caller's workspace list, shows task and agent counts, and merges recent activity. It adds no write path and does not change workspace access.

The backend guard rejects agent tokens. The frontend stores the current response in memory and refreshes it every 30 seconds. Workspace and activity links keep the workspace context.

## UI preview

`UI-01` shows the loaded overview at `/office/overview`. The Office shell provides navigation and the page title. The content scrolls below the shell.

Desktop:

```text
Office navigation | Overview
                  | Workspace A  Tasks 12  In progress 2  Done 8  Agents 3  Approvals 1
                  | Workspace B  Tasks  4  In progress 1  Done 2  Agents 1  Approvals 0
                  | Recent activity
                  | Task KAN-21 updated in Workspace A        [Run]
                  | Agent completed KAN-18 in Workspace B      [Run]
```

Phone:

```text
Office menu
Overview
Workspace A
Tasks 12 · In progress 2 · Done 8
Agents 3 · Approvals 1
Workspace B
Tasks 4 · In progress 1 · Done 2
Agents 1 · Approvals 0
Recent activity
Task KAN-21 updated in Workspace A       [Run]
Agent completed KAN-18 in Workspace B    [Run]
```

The phone layout stacks the same workspace cards and activity rows. Each row remains a separate touch target. The counts and navigation are required; spacing is illustrative.

## Work order

- [Task 01: Add the read-only workspace overview](task-01-cross-workspace-overview.md)

## Verification strategy

- Run backend tests for the aggregate service, SQLite queries, route scope, and Codex probe stderr handling.
- Run frontend unit tests for the aggregate store, hook, and activity links.
- Run the desktop and phone Office navigation browser tests.
- Run the specification linter and document catalog validator.

## Implementation result

The work order is complete. The backend and frontend now provide the read-only overview. The review fix also waits for process cleanup before it classifies managed npm probe errors.
