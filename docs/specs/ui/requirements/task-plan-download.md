---
status: active
system: ui
created: 2026-10-28
owners:
  - kandev
---

# Task Plan Download Requirements

## Overview

Readers can download the current main task Plan draft for offline review. UI
owns this local presentation interaction; Tasks retains Plan content, history,
persistence, and lifecycle ownership.

## Requirements

### REQ-UI-TASK-PLAN-DOWNLOAD-001: Download the current Plan draft

**Intent:** Let a reader obtain the text currently shown in the main Plan as a
Markdown file without changing the Plan.

#### Acceptance criteria

- **AC-UI-TASK-PLAN-DOWNLOAD-001.1:** When the reader activates Download in the
  main Plan header, the system shall download `plan.md` containing the current
  draft Markdown, including unsaved edits, Unicode, and line breaks, without
  adding a title, frontmatter, or other content.
- **AC-UI-TASK-PLAN-DOWNLOAD-001.2:** When the current draft is empty or only
  whitespace, Download shall be disabled. Otherwise the action shall not
  require an active session, agent, or workspace.
- **AC-UI-TASK-PLAN-DOWNLOAD-001.3:** Download shall not save or implement the
  Plan, mutate it, write to the workspace, or request a backend operation.
  Existing independent autosave and lifecycle behavior shall continue.
- **AC-UI-TASK-PLAN-DOWNLOAD-001.4:** If initiating the download fails
  synchronously, the system shall show localized failure feedback and retain
  the current draft.
- **AC-UI-TASK-PLAN-DOWNLOAD-001.5:** Desktop and phone shall provide one
  localized, accessible, keyboard-reachable Download control in the main
  Plan header. It shall be touch-reachable on phone and coarse pointers, fit
  within the header without document horizontal overflow, and use a 28px
  ordinary desktop fine-pointer height and at least 44px on phone or coarse
  pointer, including a narrow fine-pointer phone.

## Out of scope

- Revision or preview downloads, clipboard actions, alternate export formats,
  title-based filenames, and new download overlays.
- Plan persistence, autosave, save acknowledgements, task switching, revision
  preview, and implementing a Plan.

## Implementation Plans

- [Download task Plan draft](../../../plans/task-plan-download/plan.md)
