---
status: active
system: ui
created: 2026-08-25
owners:
  - kandev
---

# Changes File Row Containment Requirements

## Overview

The Changes panel can be narrower than the browser viewport, and repository file
paths can be arbitrarily long. File identity must adapt to the panel's available
inline width without covering the change statistics and status cue that explain
the row. The UI system owns this presentation contract; repository paths, pull
request data, Git status, and diff routing remain owned by their existing
systems.

## Requirements

### REQ-UI-CHANGES-FILE-ROW-CONTAINMENT-001: Preserve file identity and trailing metadata

**Intent:** Keep every Changes file row understandable and actionable when its
path is longer than the available panel width.

#### Acceptance criteria

- **AC-UI-CHANGES-FILE-ROW-CONTAINMENT-001.1:** When a working-tree or pull-request file row has a path wider than the available Changes panel, the directory and filename shall truncate within the row without overlapping its addition count, deletion count, or status marker.
- **AC-UI-CHANGES-FILE-ROW-CONTAINMENT-001.2:** The trailing addition count, deletion count, and status marker shall remain fully contained and readable at every supported Changes panel width.
- **AC-UI-CHANGES-FILE-ROW-CONTAINMENT-001.3:** Truncation shall preserve the row's existing full-path title and its click or tap outcome, including the pull-request and repository identity used to open the diff.
- **AC-UI-CHANGES-FILE-ROW-CONTAINMENT-001.4:** Desktop and phone Changes surfaces shall provide the same containment behavior without adding horizontal overflow to the panel or document.

## Out of scope

- Changing file-path, diff-statistic, or file-status data.
- Changing row ordering, grouping, density, actions, or diff-source precedence.
- Changing the Changes panel's resize limits, scroll ownership, or mobile entry point.
