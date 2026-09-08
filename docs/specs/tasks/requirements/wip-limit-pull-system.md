---
status: active
system: tasks
created: 2026-07-27
updated: 2026-08-12
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
