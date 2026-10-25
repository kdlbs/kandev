---
status: current
system: tasks
requirements:
  - REQ-TASKS-EXACT-PROFILE-LAUNCH-001
---

# Exact Profile Assignment at Launch System Design

## Purpose and boundaries

The task system owns task-session selection and exact assignment binding. The
agents system owns profile configuration and runtime model policy; this design
uses its profile resolution without changing those contracts.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-EXACT-PROFILE-LAUNCH-001` | [Redirected launch](#redirected-launch) |

## Assignment admission and change token

The agent settings store maintains a monotonic profile revision token in the
same database as profiles. Inserts, updates, and deletes advance it, including
writes that bypass the settings API. The task assignment captures this token
only after validating the profile's workspace and enabled state. Its repository
write rechecks the profile and token in the assignment transaction, so a
concurrent edit cannot silently qualify a stale snapshot. Existing assignments
without a token are not inferred from timestamps; they fail validation until
reassigned.

Every launch path resolves the active assignment again. Session bindings and
deferred moves carry the assignment generation and revision token. Prompt and
resume admission compare both with the selected session and reject mismatches
or a conflicting model before dispatch. A durable receipt records the applied
or failed-closed outcome for a governed launch.

## Redirected launch

`orchestrator.Service.startCreatedSession` resolves the active exact assignment
before workflow processing. If workflow processing completes the original
session and redirects to the task's newly active session, the task system must
set that session's profile to the resolved assignment before binding the exact
generation and revision. Persist the profile and its snapshot through the
existing state-guarded session update. A rejected update stops the launch. The
launch request must use the same assigned profile identity that was persisted.

This rule applies only when an exact assignment exists. Other workflow
redirects retain their current profile resolution behavior.
