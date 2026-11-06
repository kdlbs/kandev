---
status: active
system: platform
created: 2026-10-20
owners:
  - kandev
---

# Silent Restore Queue Resume

## Overview

Silent restart recovery pauses Auto-run when the restored session has queued
work, so the queue cannot race the restore
(`AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.21`). Nothing turns it back on, so after
every backend restart each restored session with queued messages stays paused
until a person re-enables it, and agent-to-agent messages queued for it stop.
An opt-in flag lets a successful restore return Auto-run to what it was.

## Requirements

### REQ-PLATFORM-SILENT-RESTORE-QUEUE-RESUME-001: Restore the Auto-run a successful restore paused

**Intent:** With the flag on, a session that was flowing before the restart
keeps flowing after a successful restore; nothing else changes.

#### Acceptance criteria

- **AC-PLATFORM-SILENT-RESTORE-QUEUE-RESUME-001.1:** With
  `features.resumeQueueAfterSilentRestore` off, which is the default in every
  shipped profile, the restore keeps its current pause and nothing re-enables
  Auto-run automatically.
- **AC-PLATFORM-SILENT-RESTORE-QUEUE-RESUME-001.2:** With the flag on, Auto-run
  stays paused while the restore runs, and after the restore succeeds it is
  re-enabled only if it was on before the restore paused it. The restore itself
  still sends no prompt and drains nothing inside its locks; after a durable
  restore commit, it schedules the existing ordinary drain outside those locks.
- **AC-PLATFORM-SILENT-RESTORE-QUEUE-RESUME-001.3:** A pause the user set before
  or during the restore, and the pause of a failed or not-yet-committed restore,
  shall remain. An unreadable policy cannot authorize automatic resumption.

## Scope exclusions

This setting does not start pending tasks, retry unsent requests, restore
archived history, resolve independent recovery blocks, or change manual queue
controls. It adds no timer or new automatic-drain scheduler. Recovery pause
ownership is process-local: a later process cannot infer ownership from a saved
OFF policy and leaves it paused.

## Design and delivery

- [System design](../system-design/silent-restore-queue-resume.md)
- [Delivery package](../../../plans/silent-restore-queue-resume/plan.md)
