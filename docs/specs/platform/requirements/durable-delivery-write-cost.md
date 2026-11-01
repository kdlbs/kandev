---
status: active
system: platform
created: 2026-10-29
owners:
  - kandev
---

# Delivery write cost requirements

## Overview

Platform owns durable delivery between agentctl and the backend
([durable agent delivery](durable-agent-delivery.md)). Delivery persistence
shares the backend's SQL writer with every interactive action. This document
bounds what delivery may spend on that writer while keeping the power-loss
guarantee in [pruned delivery recovery](pruned-delivery-recovery.md).

## Terminology

- **Interactive write:** A SQL mutation a user waits on, such as task creation,
  archive, delete, or prompt submission.
- **Journal release:** A backend request that lets agentctl discard journal
  data: event acknowledgment or submission retirement.
- **Durability barrier:** A point after which every earlier SQL commit survives
  a host power loss.

## Requirements

### REQ-PLATFORM-DELIVERY-WRITE-COST-001: Bounded delivery write cost

**Intent:** Streaming agent output must not starve the interactive writes that
users wait on, such as creating, switching, archiving, or deleting a task and
sending a prompt.

**User story:** As a user running several agent sessions at once, I want task
actions to respond promptly while those sessions stream output.

#### Acceptance criteria

- **AC-PLATFORM-DELIVERY-WRITE-COST-001.1:** When eight SQLite-backed sessions each offer 100 streamed output events per second, durable delivery shall sustain at least 90% of the offered event rate, and the p95 latency of an ordinary task-creation write shall be at most 25% of the pre-change baseline measured on the same host in the same benchmark run.
- **AC-PLATFORM-DELIVERY-WRITE-COST-001.2:** A batch of compatible streamed output events shall become received, projected, and cursor-advanced together or not at all. A crash or failed commit shall leave none of that batch received, and retry shall produce the same canonical messages as an uninterrupted run.
- **AC-PLATFORM-DELIVERY-WRITE-COST-001.3:** Every request that lets agentctl release journal data, including event acknowledgment and submission retirement, shall be preceded by a successful durability barrier that covers the SQL state the request relies on. A failed barrier shall send no release request and shall retain the pending progress for retry. This preserves AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.1.
- **AC-PLATFORM-DELIVERY-WRITE-COST-001.4:** SQL writes that no journal release depends on shall survive termination of the Kandev process. Compatibility effect: after a host power loss or operating-system crash, the most recent of those writes can be lost, as they could before durable delivery. PostgreSQL deployments shall keep their existing commit durability.

## Out of scope

- Combining receipt for tool-call, shell-output, terminal, or other unbatched
  events.
- Disabling the journal for local executors.
- Multiple SQL writers or write prioritization.

## Design and delivery

- [System design](../system-design/durable-delivery-write-cost.md).
- [Fix package](../../../plans/delivery-writer-contention/plan.md).
