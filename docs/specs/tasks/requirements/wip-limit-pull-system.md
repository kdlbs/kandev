---
status: active
system: tasks
created: 2026-07-27
updated: 2026-10-25
owners:
  - kandev
---
# WIP Limits and Visible Overflow Queues Requirements

## Overview

Workflow WIP limits control active admission, not visibility. Work that cannot run immediately remains
durable and visible in a deterministic queue.

## Requirements

### REQ-TASKS-WIP-LIMIT-PULL-SYSTEM-001: WIP Limits and Visible Overflow Queues

**Intent:** Apply one visible overflow and admission contract to task creation, task moves,
integrations, and workflow transitions without exceeding configured WIP capacity.

#### Acceptance criteria

- **AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.1:** When a task is created for a limited step, the system shall admit it when capacity exists; otherwise it shall keep the task durable and visible in the destination queue or its configured one-hop feeder, with `wip_limit: 0` remaining unlimited.
- **AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.2:** When a queued task is not admitted, the system shall not start an agent, create a session, prepare an executor, or consume a WIP slot; when capacity opens, it shall promote queued work in destination-first deterministic order and run destination entry behavior only after admission.
- **AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.3:** When a task is moved by UI, HTTP, WebSocket, MCP, drag/drop, bulk action, or workflow transition into a full limited step, the system shall complete the move as queued in that destination without routing through its feeder, and shall defer destination entry effects until promotion.
- **AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.4:** When a task is created or moved through any supported surface, the system shall return and publish its actual placement, admission, queue destination, and queue time so the Kanban board can show admitted and queued cards separately with correct WIP counts.
- **AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.5:** When a configured feeder is full, invalid, or belongs to another workflow, the system shall return a typed conflict and persist no task or launch intent; it shall not walk a second feeder or create a hidden task.
- **AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.6:** When direct creation and event-driven promotion can both auto-start the same admitted task, the system shall use one race-safe claim so at most one session starts, and shall release the claim when launch fails so retry remains possible.
- **AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.7:** Saving a workflow step's WIP limit
  shall apply to subsequent admissions without restart. Increasing it or setting
  it to unlimited shall trigger reconciliation of eligible queued tasks without
  another task move. Lowering it shall preserve already admitted work and defer
  new admissions until capacity permits them.
- **AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.8:** A WIP-queued task's details shall
  identify the Workflow WIP limit, workflow, and destination step, and link to
  that workspace's workflow configuration. The link shall work on desktop and
  phone without starting a session. Unknown or inaccessible destinations shall
  show an unavailable state rather than link to an unrelated workflow.
- **AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.9:** Queue explanations shall label WIP
  capacity in tasks and global session capacity in sessions. If WIP promotion
  exposes a global session limit, the displayed reason shall follow the current
  authoritative queue state. Changing one limit shall not bypass the other.
- **AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.10:** Pull-from guidance shall remain
  readable after a tap on phone and coarse-pointer devices, with a dismissible
  drawer and a touch target of at least 44 pixels. Desktop focus and hover shall
  expose the same guidance without changing the selected feeder or draft.
- **AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.11:** When an engine transition queues
  a session-lifecycle task at a full WIP-limited destination, the backend shall
  log the deferred entry with the task, session, source and destination steps,
  limit, and `wip_limit_reached` reason. Entry remains deferred until admission.
- **AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.12:** An admitted task keeps its WIP
  slot until it leaves the step, is archived, or is deleted, whatever its
  session state. A workspace may opt in, per workspace and off by default, to
  stop counting an admitted `IN_PROGRESS` or `FAILED` task whose sessions are
  all terminal (or that has none) once a configurable grace (default 10
  minutes, whole minutes from 1 to one year) has passed since its last task or session
  change. The released task is not moved, re-stated, or un-admitted; it counts
  again as soon as it has a live session. After the grace, the next queued or
  feeder task is pulled without another task move or a restart.
- **AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.13:** For any workflow step the caller
  can read, Kandev shall list each admitted task with whether it counts toward
  the step's WIP limit and why (live session, no live session, release grace
  pending with its release time, or released), and the number counted shall
  equal the number admission uses.
