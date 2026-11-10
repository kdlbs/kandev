---
status: active
system: tasks
created: 2026-10-27
owners:
  - Kandev team
---

# Task Creation Attachments Requirements

## Overview

Office task creation offers the attachment experience of the Kanban creation
composer. Users can attach working files, paste images, inspect previews, and
give those files to the agent when Office starts the task.

The task system owns submission descriptors, claims, and durable task context.
Office owns its navigation, task properties, and scheduling policy.
The existing [prompt attachment contract](prompt-attachments.md) retains
ownership of limits, access checks, materialization, and storage maintenance.

## Requirements

### REQ-TASKS-CREATION-ATTACHMENTS-001: Creation attachment parity

**Intent:** A file selected during task creation remains visible, recoverable,
and available to the task's agents without a separate manual upload.

#### Acceptance criteria

- **AC-TASKS-CREATION-ATTACHMENTS-001.1:** Office creation shall accept multiple
  files through Attach files, readable pasted images, and drag-and-drop.
  The same file limits and supported image formats as Kanban shall apply.
  Ordinary text paste shall retain its existing behavior.
- **AC-TASKS-CREATION-ATTACHMENTS-001.2:** Images shall show thumbnails and
  open a larger preview. Other files shall show their names and size/type details
  through the existing attachment preview. Preview errors shall preserve siblings
  and description text. Arbitrary document-content rendering is not required.
- **AC-TASKS-CREATION-ATTACHMENTS-001.3:** Each file shall show its upload state.
  Failed files shall offer retry and removal. Create Task and its handler shall
  block while any file is incomplete, failed, or expired. Removing that file
  shall restore submission eligibility without losing ready siblings.
- **AC-TASKS-CREATION-ATTACHMENTS-001.4:** Closing and reopening the same draft
  shall preserve its attachments. Reload shall restore ready descriptors and
  previews while staging remains valid. Browser storage shall contain no file
  bytes. Expired or unavailable restored files shall require removal or reattachment.
- **AC-TASKS-CREATION-ATTACHMENTS-001.5:** Workspace changes, parent changes,
  discard, and successful creation shall isolate successive drafts. Late upload
  results shall not enter another draft. Discarded or removed staged files shall
  receive best-effort cleanup. Successful creation shall retain accepted files.
- **AC-TASKS-CREATION-ATTACHMENTS-001.6:** Successful creation shall persist the
  ordered creation attachment set and claim all its files before the task becomes
  available to an agent. Invalid ownership, expiry, or a missing file shall
  reject creation without partial claims or an agent start. An idempotent retry
  shall not attach a different file set to an existing task.
- **AC-TASKS-CREATION-ATTACHMENTS-001.7:** Task-bound Office runs shall receive
  the explicit creation attachment set with canonical names, order, and delivery
  modes. Delayed assignment, workspace pause, scheduling delay, and backend restart
  shall preserve the set. Unrelated task, chat, and session attachments shall
  never enter this set.
- **AC-TASKS-CREATION-ATTACHMENTS-001.8:** Missing or unauthorized accepted
  attachment bytes shall produce visible delivery failure. The agent shall not
  receive a silently reduced file set. Creation files shall follow task retention,
  survive individual run/session cleanup, and disappear through task deletion.
- **AC-TASKS-CREATION-ATTACHMENTS-001.9:** Desktop keyboard and phone touch
  users shall select, inspect, retry, remove, and submit attachments. Phone
  controls shall remain within the viewport and provide reachable touch targets.
  Image preview dismissal shall restore focus without closing the draft.

## Compatibility and scope

Existing Office title and project requirements remain. Attachments do not make
description text mandatory. Unassigned tasks can retain attachments for later
assignment. Existing files, staged upload expiry, and native-prompt/path delivery
retain the [shared prompt attachment semantics](prompt-attachments.md).

This capability adds no blocker picker, Office comment attachments, new file
formats, generic PDF/archive renderer, provider settings, or new overflow options.

## System design

- [Task creation attachments](../system-design/task-creation-attachments.md).
- [Office creation controls](../../office/requirements/overview.md#req-office-task-creation-001-supported-creation-controls).

## Implementation plans

- [Office creation attachment parity](../../../plans/office-new-task-supported-controls/plan.md).
