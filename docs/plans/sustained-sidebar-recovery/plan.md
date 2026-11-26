---
created: 2026-11-25
status: complete
requirements:
  - REQ-PLATFORM-INTERACTIVE-READS-007
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007
system_design:
  - ../../specs/platform/system-design/interactive-read-availability.md
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
legacy_specs: []
---

# Implementation plan: Sustained sidebar recovery

## Overview

Stop showing "Tasks could not be refreshed." during short backend stalls when rows are already visible.
Stop short SQLite writer stalls from becoming application-wide `503` windows.
Two independent work orders: backend health confirmation and frontend sidebar recovery.

## Evidence and root cause

Evidence comes from `~/.kandev/logs/backend-logs-2026-11-25-00000{7,8}.log` on the running instance
(build `85c41ef5373d`) and from live host measurements on 2026-11-25.

**Sidebar banner.** Between 11:06:17 and 11:09:13 UTC, six sidebar queries ended as client
aborts after about 10 seconds (`499`, 9,980 to 10,976 ms), and two received `503` within 0 ms.
That consumed the thirty-second recovery budget. The exhausted record then rejected every
background refresh without a request: the log contains no sidebar query from 11:09:13 to 11:12:29,
although the backend was healthy again at 11:07:39 and live task updates continued.
Only a later foreground episode at 11:12:29 restarted recovery. `admit()` in
`apps/web/lib/sidebar/sidebar-task-page-cache.ts` implements that latch, as AC-007.4 required.
Rows remained visible from the store, so the banner contradicted live data.

**Global 503 windows.** Persistence became unhealthy at 11:07:26, 11:12:26, and 18:46:53,
each time for about thirteen seconds. Each failure was a single `deadline_exceeded` probe with
one writer connection in use. Durability-barrier commits nearby spent 0.6 to 2.1 seconds in
`COMMIT` with under 5 ms of write time. One missed two-second probe marks every store unhealthy
until the next fifteen-second tick, so a two-second writer stall becomes a thirteen-second outage
for every stateful request, including reads that use separate reader connections.

**Underlying pressure (not repaired here).** The host is a Proxmox VM on a consumer NVMe
(`CT1000P310SSD8`) whose thin pool is 82% full. `/proc/pressure/io` reported a full-stall average
of 26 to 27% over five minutes while agent builds, lints, and Go tests shared the disk. Swap held 8 GiB.
`kandev.db` is 17 GB (4.2 M pages, 26 free) with about 8 GiB of page cache available, so many
reads reach the stalled disk. Successful sidebar queries have p50 191 ms and p99 1.6 s; a warm
replay took 0.24 s and a cold one 3.6 s. A full per-table size breakdown was not completed
because scanning the live database added I/O pressure; it was stopped.

## Scope

### In scope

- Confirmation probe for contention-only SQLite health failures (Task 01).
- Sustained retained-row sidebar schedule and a sixty-second background cooldown after exhaustion (Task 02).
- Requirement, design, ADR, and `apps/web/AGENTS.md` sidebar guidance alignment.

### Out of scope

- Disk capacity, VM storage layout, or agent build scheduling on the host.
- Writer-holder attribution, durability-barrier batching, checkpoint tuning, and database size reduction.
- Task-detail and task-navigation recovery timing (REQ-PLATFORM-INTERACTIVE-READS-005).
- New settings, feature flags, pools, or health states.

## Work orders

| Order | Work order | Depends on | Result |
| --- | --- | --- | --- |
| 01 | [Confirm contention health failures](task-01-confirm-contention-health-failures.md) | none | done |
| 02 | [Sustain sidebar recovery](task-02-sustain-sidebar-recovery.md) | none | done |

The orders are independent. Run them sequentially; Task 01 first shortens the outages Task 02 must absorb.

## ASCII UI preview

UI-01: Desktop sidebar, rows retained, backend stalled for 90 seconds.

```text
Before (at about 30 s)            After (0 to 150 s)
TASKS [All tasks] [Filter]        TASKS [All tasks] [Filter]
Tasks could not be refreshed.     Task A                  24m
                       [Retry]    Task B                  12s
Task A                  24m       Task C                   2m
Task B                  12s
```

UI-02: After 150 seconds of failed recovery, the existing warning and Retry appear unchanged.
A live task update at least sixty seconds later starts one quiet cycle; success removes the warning.

The phone task picker and app navigation use the same states in their existing containers.
No new control, copy, or spacing is introduced. Existing localized strings remain authoritative.
This maps to AC-PLATFORM-INTERACTIVE-READS-007.1, .2, .4, and .7.

## Verification strategy

Each work order lists exact commands. Backend uses deterministic probe stubs, not timing on a real disk.
Frontend uses fake timers for schedules, cooldown, and admission; existing E2E keeps quiet-recovery coverage.

## Risks

- A real outage that presents only as deadline expiry is detected up to about five seconds later.
- A longer quiet window can show rows up to 150 seconds stale. Live WebSocket updates continue to patch them.
- Raising the attempt count increases load during a real outage; delays grow to thirty seconds to bound it.
