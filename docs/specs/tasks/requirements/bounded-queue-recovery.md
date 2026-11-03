---
status: active
system: tasks
created: 2026-10-11
owners:
  - kandev
---

# Bounded queue recovery

## Overview
Task agents need to inspect instructions on exact terminal predecessors or direct children without consuming them. Task/session identity and relationships make this task-system behavior.

## Requirements

### REQ-TASKS-QUEUE-RECOVERY-001: Scoped immutable queue observation

**Intent:** Recover instructions through bounded evidence under exact task authority.

#### Acceptance criteria
- **AC-TASKS-QUEUE-RECOVERY-001.1:** Current-session census shall accept no caller-selected target and return body-free FIFO identities, hashes, sizes, capacity and auto-run, excluding reserved rows from pending count.
- **AC-TASKS-QUEUE-RECOVERY-001.2:** Terminal reads shall require an exact own-task session different from caller in CANCELLED, FAILED or COMPLETED, without fallback.
- **AC-TASKS-QUEUE-RECOVERY-001.3:** Child inventory/body reads shall require exact direct parent, same workspace and exact target session. Other relations shall fail without revealing queue contents.
- **AC-TASKS-QUEUE-RECOVERY-001.4:** Manifests/fragments shall bind identities, FIFO positions, incarnation, full-body SHA-256, byte offset/length, completion and attachment/settings/reservation evidence; empty/identical bodies shall retain distinct IDs.
- **AC-TASKS-QUEUE-RECOVERY-001.5:** Changed contents, order, settings, reservation observation, incarnation or authority shall reject continuation without partial data. Pinned first-page mismatch shall reject.
- **AC-TASKS-QUEUE-RECOVERY-001.6:** Every outcome shall preserve queue/order, policies, generations, claims, leases, moves and task/session state, without inference, consumption, acknowledgement or transfer.
- **AC-TASKS-QUEUE-RECOVERY-001.7:** Bounds shall be 256 sessions, 1,024 entries/session, 16 MiB source/session and 32 MiB/inventory; body budget 1,024..262,144 (default 65,536), 256 fragments, 2 MiB response and 4 KiB cursor. Oversize inventories shall reject rather than imply completeness.
- **AC-TASKS-QUEUE-RECOVERY-001.8:** Authenticated Kanban/Office task surfaces shall expose readers under existing allowlists. Raw WebSocket, forged identity and other MCP surfaces shall fail. Body-free output/logs shall exclude raw attachments, metadata, paths and credentials.
- **AC-TASKS-QUEUE-RECOVERY-001.9:** Memory, SQLite and PostgreSQL shall agree. Empty FIFO shall not certify dispatch absence, replacement acknowledgement or retirement.

## Exclusions
Queue mutation/replay/settlement, archive verification, replacement intake, physical retirement, plugin Host expansion, UI and deployment. Source merge alone does not authorize overlay removal.
