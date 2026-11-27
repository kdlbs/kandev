---
status: active
system: ui
created: 2026-10-28
owners:
  - kandev
---

# Workflow Export Download Requirements

## Overview

People can save the text already displayed in the workflow export dialog as a
local file. UI owns this presentation action; Tasks retains workflow definitions,
export contents, selection, and import behavior.

## Requirements

### REQ-UI-WORKFLOW-EXPORT-DOWNLOAD-001: Download displayed workflow export

**Intent:** Let a person save the current workflow export without copying it to
another editor or changing the workflow.

#### Acceptance criteria

- **AC-UI-WORKFLOW-EXPORT-DOWNLOAD-001.1:** For both a single workflow export
  and Export All, Download shall save the exact text currently displayed in the
  existing export dialog as `workflows.yaml`, preserving Unicode and line
  breaks without added or reformatted content.
- **AC-UI-WORKFLOW-EXPORT-DOWNLOAD-001.2:** Download shall be disabled when
  the displayed text is empty or only whitespace. Initiating it shall not
  fetch another export or change workflows, selections, saves, imports,
  profiles, sessions, or workspace and Git state.
- **AC-UI-WORKFLOW-EXPORT-DOWNLOAD-001.3:** A synchronous download initiation
  failure shall show localized feedback, keep the dialog and text available,
  and permit retry. The failure is local and shall not cause error telemetry.
- **AC-UI-WORKFLOW-EXPORT-DOWNLOAD-001.4:** Desktop and phone shall provide a
  visible, localized, accessible, keyboard-reachable Download action in the
  existing dialog. Phone and coarse-pointer targets shall be at least 44 px,
  including a narrow fine-pointer phone. Actions shall remain contained and
  reachable without document horizontal overflow.
- **AC-UI-WORKFLOW-EXPORT-DOWNLOAD-001.5:** The preview, Close, Copy, and
  clipboard feedback and fallback behavior shall remain available and retain
  their existing semantics.

## Out of scope

Changing the export format or schema, dynamic filenames, new dialogs,
workflow persistence or import, and shared download or clipboard contracts.

## Implementation Plans

- [Workflow export download](../../../plans/workflow-export-download/plan.md)
