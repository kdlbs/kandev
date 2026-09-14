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

## Related contracts

- [Required-store health](postgres-domain-store-parity.md), REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.
- [Workspace read recovery](../../workspaces/requirements/workspace-read-recovery.md).

## Out of scope

- New statistics, historical rollup storage, approximate counts, or cross-user caches.
- New database pools, health-state semantics, public configuration, or feature flags.
- Guaranteed latency under unrelated write saturation, failed disks, or remote database outages.
