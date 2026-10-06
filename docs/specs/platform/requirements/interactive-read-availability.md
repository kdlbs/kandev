---
status: draft
system: platform
created: 2026-09-12
owners:
  - kandev
---

# Interactive Read Availability Requirements

## Overview

Platform owns shared read capacity and operational availability across callers.
Statistics consumes task history but must not exhaust normal navigation reads.
Task and workspace systems retain authority over their records and permissions.

## Requirements

### REQ-PLATFORM-INTERACTIVE-READS-001: Efficient statistics

**Intent:** Users can load statistics from substantial histories without unnecessary delay or changed totals.

#### Acceptance criteria

- **AC-PLATFORM-INTERACTIVE-READS-001.1:** Equivalent input records shall produce equivalent statistics, date buckets, ordering, and pagination before and after optimization on SQLite and PostgreSQL.
- **AC-PLATFORM-INTERACTIVE-READS-001.2:** Statistics shall preserve workspace authorization, range boundaries, and existing exclusions for ephemeral and automation tasks.
- **AC-PLATFORM-INTERACTIVE-READS-001.3:** On the documented reference workload, all seven statistics sections shall complete within five seconds at the 95th percentile of ten warm runs. The workload and machine shall be recorded with results.

### REQ-PLATFORM-INTERACTIVE-READS-002: Shared read capacity

**Intent:** Statistics load does not prevent unrelated reads or falsely indicate unavailable persistence.

#### Acceptance criteria

- **AC-PLATFORM-INTERACTIVE-READS-002.1:** When statistics requests exceed their execution capacity, excess work shall wait outside database execution or receive a bounded retryable failure.
- **AC-PLATFORM-INTERACTIVE-READS-002.2:** With healthy storage and concurrent statistics load alone, ordinary workspace reads and persistence probes shall continue to succeed.
- **AC-PLATFORM-INTERACTIVE-READS-002.3:** Cancelled or expired statistics requests shall release their capacity and shall not start deferred database work.
- **AC-PLATFORM-INTERACTIVE-READS-002.4:** Actual required-store failures shall retain the existing readiness, diagnostics, stateful-request rejection, and recovery behavior.

### REQ-PLATFORM-INTERACTIVE-READS-003: Statistics recovery

**Intent:** A temporary failure does not leave statistics permanently stuck or erase successfully loaded sections.

#### Acceptance criteria

- **AC-PLATFORM-INTERACTIVE-READS-003.1:** When one section fails, successful sections shall remain usable. The failed section shall show a localized error with retry access.
- **AC-PLATFORM-INTERACTIVE-READS-003.2:** After a transient failure, bounded retry or foreground recovery shall reload failed sections without a page reload. Repeated failures shall not cause continuous requests.
- **AC-PLATFORM-INTERACTIVE-READS-003.3:** Changing workspace or range shall cancel prior requests and retries. Data from a different selection shall never appear in the new selection.
- **AC-PLATFORM-INTERACTIVE-READS-003.4:** Desktop and phone users shall be able to retry by keyboard or touch. Loading, failure, and retry status shall be accessible.
- **AC-PLATFORM-INTERACTIVE-READS-003.5:** Copy Stats shall remain unavailable while any required section lacks current-selection data.

### REQ-PLATFORM-INTERACTIVE-READS-004: Bounded clarification reads

**Intent:** Background inbox refreshes preserve capacity for navigation and persistence checks.

#### Acceptance criteria

- **AC-PLATFORM-INTERACTIVE-READS-004.1:** Concurrent clarification listing and count requests shall execute within a shared capacity limit. Excess requests shall wait outside database execution and respect cancellation.
- **AC-PLATFORM-INTERACTIVE-READS-004.2:** With healthy storage and clarification reads alone, task reads and persistence probes shall continue to succeed.
- **AC-PLATFORM-INTERACTIVE-READS-004.3:** Equivalent clarification records shall retain visibility, ordering, pagination, current-turn ownership, and hidden-count results on SQLite and PostgreSQL.
- **AC-PLATFORM-INTERACTIVE-READS-004.4:** Within one active browser workspace, all inbox refresh triggers shall share one request. Events during that request shall schedule at most one subsequent refresh.
- **AC-PLATFORM-INTERACTIVE-READS-004.5:** Temporary inbox failures shall impose a retry delay on every trigger. Repeated events shall not bypass that delay.
- **AC-PLATFORM-INTERACTIVE-READS-004.6:** Changing workspace or authentication shall cancel obsolete inbox requests and timers. Obsolete results shall not update the new selection.

### REQ-PLATFORM-INTERACTIVE-READS-005: Task navigation recovery

**Intent:** Users can recover a task read after a temporary backend failure without reloading the application.

#### Acceptance criteria

- **AC-PLATFORM-INTERACTIVE-READS-005.1:** Temporary task-read failures shall show localized availability copy and Retry. They shall not imply deletion or lost access.
- **AC-PLATFORM-INTERACTIVE-READS-005.2:** Each task-navigation read attempt shall finish within ten seconds. A timeout shall be classified as temporary and use the same retry budget. A recovery cycle shall perform at most two automatic retries; exhausted recovery shall retain manual Retry and the task-overview link.
- **AC-PLATFORM-INTERACTIVE-READS-005.3:** Concurrent recovery triggers shall share one read. Navigation, authentication changes, and unmount shall invalidate obsolete recovery work.
- **AC-PLATFORM-INTERACTIVE-READS-005.4:** Successful recovery shall open the selected task and retain its valid selected session. It shall not select a task from an earlier request.
- **AC-PLATFORM-INTERACTIVE-READS-005.5:** Missing or inaccessible tasks shall retain the existing generic unavailable state. Authorization errors shall not receive automatic retries.
- **AC-PLATFORM-INTERACTIVE-READS-005.6:** Desktop and phone users shall have equivalent keyboard and touch recovery actions. Phone targets shall measure at least 44px without horizontal overflow.
- **AC-PLATFORM-INTERACTIVE-READS-005.7:** When a refresh fails after task details load, those details shall remain visible. The page shall identify the failed refresh without presenting stale data as newly verified.

## Related contracts

- [Required-store health](postgres-domain-store-parity.md), REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.
- [Workspace read recovery](../../workspaces/requirements/workspace-read-recovery.md).
- [Missing task routes](../../tasks/requirements/missing-task-route-recovery.md) remain authoritative for permanent lookup failures.

## Out of scope

- New statistics, historical rollup storage, approximate counts, or cross-user caches.
- New database pools, health-state semantics, public configuration, or feature flags.
- Guaranteed latency under unrelated write saturation, failed disks, or remote database outages.
- Attribution of every incident to inbox queries without query-level evidence.
