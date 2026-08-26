# Class-Aware Git Subprocess Admission

- **Status:** Accepted
- **Date:** 2026-08-02
- **Scope:** Backend and agentctl Git subprocess scheduling
- **Related spec:** [Git Subprocess Admission](../specs/platform/requirements/git-subprocess-admission.md)
- **Related issue:** [#2150](https://github.com/kdlbs/kandev/issues/2150)

## Context

Kandev has a process-wide semaphore that caps Git subprocess concurrency. All Git
callers enter the same queue, while some API handlers also impose their own
fan-out limit. This bounds peak process creation but does not guarantee progress
for workspace polling or repository lifecycle work during sustained interactive
traffic. It also leaves callers to decide whether admission time consumes a
command timeout.

The admission policy is a cross-cutting platform contract: every backend and
agentctl Git call must use it, future call sites must select the correct behavior,
and operators need to identify which kind of work is creating contention.

## Decision

Kandev will retain one process-local hard cap and make Git admission class-aware.

1. Every production Git operation declares `interactive`, `lifecycle`, or
   `background` work when it enters the shared controller.
2. Each class uses a FIFO queue. On each released slot, the controller selects the
   next non-empty class by deterministic round robin in this order:
   `interactive`, `lifecycle`, `background`.
3. The scheduler is work-conserving. A class may use the entire capacity when no
   other class is waiting; there are no reserved slots or independent quotas.
4. The global cap remains the only authority on active Git subprocess count and
   continues to use `KANDEV_GIT_MAX_CONCURRENT`.
5. Admission uses the caller lifetime context. Per-command execution timeouts are
   created only after admission succeeds.
6. Public Git wrappers require a work class. Classless production helpers are
   removed or made inaccessible so new call sites cannot silently join the wrong
   queue.
7. Aggregate metrics are preserved and supplemented by per-class admission
   metrics and a structured snapshot exposed through authenticated diagnostics.
8. Multi-repository status, log, and diff handlers derive their local worker bound
   from the shared Git capacity. Local fan-out controls queued work; it does not
   create a second concurrency policy.

## Why deterministic round robin

Round robin gives every continuously queued class a simple bounded-progress rule
without tuning weights that lack workload evidence. It also remains
work-conserving: interactive work keeps full throughput when lifecycle and
background queues are empty. FIFO within a class makes cancellation and ordering
testable and avoids priority inversions inside a class.

## Alternatives considered

### Raise the global limit

Rejected as the scheduling policy. A larger cap may move the saturation point but
does not prevent one workload from excluding another, and it increases peak
process pressure on Windows.

### Keep only local fan-out caps

Rejected because concurrent requests and non-HTTP Git callers still meet at the
global semaphore. Independent local constants cannot enforce system-wide
progress or the total active-process bound.

### Reserve slots per class

Rejected because idle reservations reduce throughput and require choosing
platform-wide quota sizes without sufficient workload evidence.

### Strict interactive priority

Rejected because sustained user traffic could indefinitely delay workspace
monitoring and instance setup, recreating the liveness failure.

### Weighted scheduling

Deferred. It adds policy knobs and more complex guarantees. Deterministic round
robin is sufficient for the observed liveness problem; telemetry can justify a
future weight decision if needed.

## Consequences

- Git call sites become explicit about user-visible, lifecycle, or background
  intent, making incorrect classification visible in code review.
- Every continuously queued class makes bounded progress once running commands
  release slots, but the scheduler cannot preempt commands that are already
  running.
- Admission and execution latency can be measured separately, preventing queue
  delay from masquerading as repository failure.
- The controller becomes more complex than a semaphore and needs focused race,
  cancellation, fairness, and accounting tests.
- Scheduling state remains process-local. Independent backend and agentctl
  processes each enforce their own configured cap; cross-host coordination is not
  introduced.
