---
status: active
system: integrations
created: 2026-11-16
owners:
  - kandev
---

# Issue-watch auto-start requirements

## Overview

Native issue watches create tasks and request automatic starts through the shared watcher dispatch pipeline.
An omitted watch profile must use the available task defaults. A failed automatic start must remain visible.

Integrations owns this contract because the watch initiates the task launch.
Tasks owns profile resolution, workflow routing, session admission, and task status presentation.
The [alert-ingest capability](alert-ingest.md) covers a separate declarative ingest path.

## Terminology

- **Omitted profile:** A watch does not select an agent profile or executor profile.
- **Effective profile:** The profile selected under the existing task and workflow rules.
- **Deferred start:** Session admission queues the launch until capacity becomes available.

## Requirements

### REQ-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001: Defaulted and visible automatic starts

**Intent:** An eligible watch-created task must start with its effective profile or expose a launch failure.

#### Acceptance criteria

- **AC-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001.1:** When the watch, step, and workflow omit an agent profile, an available workspace default shall start the task. Opening the task shall not be necessary.
- **AC-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001.2:** Watch starts shall preserve existing profile precedence: task-specific step replacement, step profile, workflow default, saved watch/task selection, task assignee, then workspace default. Explicit session targets shall retain their existing routing authority.
- **AC-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001.3:** When the executor profile is omitted, watch starts shall retain existing task and workspace executor selection. An explicit executor profile shall retain its existing meaning.
- **AC-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001.4:** When no agent profile resolves, the start shall fail without creating a session. An unavailable selected profile shall not silently select another profile. Existing deleted-profile watch handling shall remain intact.
- **AC-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001.5:** When automatic start fails, the task shall expose its existing automatic-start failure indicator through task updates and after reload. Desktop and phone boards shall show the same failure status.
- **AC-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001.6:** When admission defers a start for capacity, the task shall retain its queued launch without an automatic-start failure indicator.
- **AC-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001.7:** When start fails after task creation, the watch shall retain the task association and deduplication. Omitted profiles alone shall not disable the watch. Nonautomatic and overflow-queued tasks shall retain their current launch eligibility.
- **AC-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001.8:** When a later start reaches `STARTING` or `RUNNING`, the existing failure indicator shall clear. A failed attempt shall not overwrite the status of any concurrently active session.

## Out of scope

- Automatic retries or repair of tasks from earlier failed starts.
- New mandatory fields, profile precedence, executor defaults, error schema, or polling behavior.
- Recording dispatch errors in a watch's `last_error` field.
- New UI composition, controls, translations, or failure-detail presentation.
- Legacy GitHub watchers, Azure DevOps, plugin task creation, and declarative alert ingest.

## References

- [System design](../system-design/issue-watch-auto-start.md)
- [Implementation plan](../../../plans/issue-watch-auto-start-defaults/plan.md)
