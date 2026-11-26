---
created: 2026-11-24
status: implemented
requirements:
  - REQ-UI-FIRST-RUN-DIALOG-001
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-009
system_design:
  - ../../specs/ui/system-design/first-run-dialog-availability.md
  - ../../specs/tasks/system-design/task-relationship-graph.md
---

# Keep onboarding off focused graph visits

The desktop sidebar opens Home with a nonempty graphFocus parameter. PageClient checks the browser completion marker and viewport width without checking graph focus. An unfinished tour can therefore cover the selected node. The existing sidebar browser test seeds the completion marker.

Isolated Chromium reproduced this on web revision `7aa5e3c` with a prebuilt backend reporting `cd9311560`. Start on a task without a completion marker. Right-click a sidebar task and select Show in graph. The browser reaches the focused graph URL and displays AI Agents over the mounted board.

The [first-run requirements](../../specs/ui/requirements/first-run-dialog-availability.md) qualify desktop tour availability for this destination. The [task graph requirements](../../specs/tasks/requirements/task-relationship-graph.md) already specify the destination in AC-TASKS-TASK-RELATIONSHIP-GRAPH-009.4.

One [work order](task-01-graph-focus.md) covers the visibility condition and regression checks. Restrict the exception to a nonempty graph focus. A general explicit-destination check would also suppress onboarding on unrelated Home visits.

## ASCII UI preview

UI-01. After Show in graph, the chosen node is focused and interactive.

```text
Desktop/tablet: [Sidebar] | [Board Graph: selected task]
Phone:          [Full-screen task graph sheet: selected task]
```

The surface and focus are required. Spacing is illustrative.
