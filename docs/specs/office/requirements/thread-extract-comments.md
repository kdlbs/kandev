---
status: draft
system: office
created: 2026-11-10
owners:
  - Kandev
---

# Office: Thread Extract Comments Requirements

## Overview

Office users need to attach clear feedback to a specific passage in an agent comment. This requirement covers that task-discussion behavior. The Office system owns it because the source comment, task scope, and routing belong to Office.

## Terminology

- **Extract:** A selected passage from an agent comment with a user-written remark.
- **Source passage:** The text selected from the agent comment.

## Requirements

### REQ-OFFICE-THREAD-EXTRACTS-001: Attach feedback to an agent comment

**Intent:** Let a user give feedback about a precise part of an agent's task comment.

**User story:** As an Office user, I want to attach a remark to a selected passage, so that the agent can understand which part needs attention.

#### Acceptance criteria

- **AC-OFFICE-THREAD-EXTRACTS-001.1:** When a task is editable, the user shall be able to select text in an agent-authored comment and add a remark to that passage. The user shall not be able to create an extract from a user-authored comment or a read-only task.
- **AC-OFFICE-THREAD-EXTRACTS-001.2:** Before submission, the composer shall show the source passage and remark as a task-scoped draft. The draft shall survive a page reload in the same browser tab and shall be removable before submission.
- **AC-OFFICE-THREAD-EXTRACTS-001.3:** When the user submits the composer, Office shall store the extract and any typed text in one user-authored task comment. After success, the composer shall remove only extracts that still match the submitted values. New extracts and edits made during the request shall remain pending.
- **AC-OFFICE-THREAD-EXTRACTS-001.4:** If comment submission fails, the pending extracts shall remain available in the composer.
- **AC-OFFICE-THREAD-EXTRACTS-001.5:** A mention copied from an agent's source passage shall remain quoted context and shall not create a new agent route. A mention written in the user's remark shall remain eligible for the existing Office mention routing.
- **AC-OFFICE-THREAD-EXTRACTS-001.6:** On desktop and touch layouts, the Add action shall save the remark as a pending extract. If a Run action is unavailable, the selection shortcut shall use Add and shall not discard the remark.

## Out of scope

- Extracts from user-authored comments.
- Draft synchronization across browser tabs, devices, or users.
- A new comment API, database record type, or plugin-facing contract.
- Changes to the Office workflow's existing comment wake policy.
