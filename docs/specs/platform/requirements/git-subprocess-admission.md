---
status: active
system: platform
created: 2026-08-02
owners:
  - kandev
---
# Git Subprocess Admission Requirements

## Overview

Kandev limits concurrent Git subprocesses, but every caller currently competes in one undifferentiated pool. A burst of interactive multi-repository log, diff, or status work can occupy the pool long enough for workspace monitoring and instance setup to wait behind it. Workspace polling also starts its execution timeout before admission, so queue delay can be reported as a Git failure and eventually stop a healthy tracker.

## Requirements

### REQ-PLATFORM-GIT-SUBPROCESS-ADMISSION-001: Git Subprocess Admission

**Intent:** Kandev limits concurrent Git subprocesses, but every caller currently competes in one undifferentiated pool. A burst of interactive multi-repository log, diff, or status work can occupy the pool long enough for workspace monitoring and instance setup to wait behind it. Workspace polling also starts its execution timeout before admission, so queue delay can be reported as a Git failure and eventually stop a healthy tracker.

#### Acceptance criteria

- **AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-001.1:** All Git subprocesses in a Kandev process MUST pass through one process-wide admission controller.
- **AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-001.2:** Active Git subprocesses MUST never exceed `KANDEV_GIT_MAX_CONCURRENT`.
- **AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-001.3:** The existing default and environment override remain unchanged by this work.
- **AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-001.4:** A caller that is the only queued work MAY use all available capacity. Separate per-class pools that can exceed the global cap are not permitted.
- **AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-001.5:** Each class has a FIFO wait queue.
- **AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-001.6:** Released slots are assigned in deterministic round-robin order across non-empty classes: `interactive`, then `lifecycle`, then `background`, then repeat.
- **AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-001.7:** Empty classes are skipped, so available capacity is never reserved for work that does not exist.
- **AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-001.8:** When all three classes remain queued and admitted commands complete, each class receives a slot within at most three successful slot releases.

## Migrated source detail

Related issue: [#2150](https://github.com/kdlbs/kandev/issues/2150)

## Why

Kandev limits concurrent Git subprocesses, but every caller currently competes in
one undifferentiated pool. A burst of interactive multi-repository log, diff, or
status work can occupy the pool long enough for workspace monitoring and
instance setup to wait behind it. Workspace polling also starts its execution
timeout before admission, so queue delay can be reported as a Git failure and
eventually stop a healthy tracker.

The existing fan-out caps reduce the frequency of saturation, but they do not
define which work may progress under contention or distinguish admission delay
from command failure. The platform needs one enforceable concurrency boundary
with deterministic progress for every kind of Git work.

## What

### One global hard cap

- All Git subprocesses in a Kandev process MUST pass through one process-wide
  admission controller.
- Active Git subprocesses MUST never exceed `KANDEV_GIT_MAX_CONCURRENT`.
- The existing default and environment override remain unchanged by this work.
- A caller that is the only queued work MAY use all available capacity. Separate
  per-class pools that can exceed the global cap are not permitted.

### Explicit work classes

Every production Git operation MUST declare one of these classes:

| Class | Work included |
| --- | --- |
| `interactive` | User-observed Git requests, including status, log, diff, staging, commits, and pushes |
| `lifecycle` | Repository, worktree, runtime, tracker setup, cleanup, rescans, and instance construction |
| `background` | Periodic workspace polling and detached refresh work |

Classless production helpers are not part of the supported API. Tests MAY use a
dedicated capacity seam without weakening production classification.

### Fair, work-conserving admission

- Each class has a FIFO wait queue.
- Released slots are assigned in deterministic round-robin order across non-empty
  classes: `interactive`, then `lifecycle`, then `background`, then repeat.
- Empty classes are skipped, so available capacity is never reserved for work
  that does not exist.
- When all three classes remain queued and admitted commands complete, each class
  receives a slot within at most three successful slot releases.
- Already-running commands are not preempted.
- A canceled waiter is removed promptly and MUST NOT consume a slot or start a
  subprocess.

### Admission and execution are different phases

- Admission waits under the lifetime context supplied by the caller.
- A command-specific execution timeout starts only after admission succeeds and
  before the subprocess starts.
- Admission cancellation or queue delay MUST NOT be counted as a Git command
  failure and MUST NOT advance the workspace tracker's permanent-failure counter.
- A Git failure or timeout after admission still follows the caller's existing
  error policy, releases its slot, and may advance the tracker failure counter.
- Missing or removed workspaces retain their existing stop behavior.

### Consistent multi-repository behavior

- Multi-repository status, log, and diff requests use one shared fan-out policy.
- Fan-out worker counts are derived from the configured Git capacity rather than
  a separate hardcoded concurrency constant.
- The admission controller remains authoritative for active subprocess count;
  local fan-out only bounds goroutine and queued-work creation.
- Existing HTTP and WebSocket response shapes, ordering guarantees, and partial
  result behavior remain unchanged.

### Diagnostics

- Existing aggregate Git subprocess metrics remain available for compatibility.
- Admission metrics additionally expose, per class: active work, waiters,
  admission attempts, and cumulative admission wait time.
- The agentctl control server exposes its process-wide admission snapshot through
  an authenticated debug endpoint.
- Admission state is in-memory and resets when its owning process restarts.

## API and permissions

- Existing user-facing Git APIs are unchanged.
- Agentctl adds `GET /api/v1/debug/subprocess-admission` on its control server.
  The route requires the same bearer token as instance-management routes and is
  not added to the health or handshake authentication exemptions.
- The endpoint permits no changing of capacity, queues, work classes, or running
  commands.

## Persistence guarantees

The controller, queues, counters, and structured snapshots are process-local
runtime state. They are not written to SQLite or restored after restart. The
configured capacity continues to come from the existing environment/default
resolution at process startup.

## Failure modes

- If a waiting request is canceled, it returns the context error without starting
  Git and without disturbing the round-robin cursor.
- If Git cannot start after admission, the slot is released and the start error is
  reported as an execution failure.
- If a running command exceeds its execution timeout, it is canceled, the slot is
  released, and the normal post-admission failure policy applies.
- If the agentctl debug endpoint is unavailable, the caller receives the existing
  control-client error; no admission state is persisted or inferred remotely.

## Scenarios

### Interactive load cannot disable workspace tracking

1. Interactive multi-repository requests keep their queue continuously populated.
2. A workspace poll enters the `background` queue.
3. The poll receives admission within the bounded round-robin window.
4. Its execution timeout begins after admission.
5. Queueing alone does not increase the tracker failure count or stop monitoring.

### Background work cannot starve instance setup

1. Background polling occupies and replenishes the available capacity.
2. Repository or tracker setup enters the `lifecycle` queue.
3. Lifecycle work receives a slot within the bounded round-robin window.
4. Other classes continue to use remaining slots without exceeding the global cap.

### Single-class throughput remains available

1. Only interactive work is queued.
2. Interactive work consumes every available slot.
3. No capacity remains idle solely to preserve a class reservation.

### Cancellation while queued is safe

1. A request waits while the global cap is full.
2. Its context is canceled before admission.
3. It leaves the queue promptly, starts no subprocess, and does not block the next
   eligible waiter.

### Cancellation wins a simultaneous grant race

1. A queued request is canceled at the same scheduling boundary that releases a
   slot.
2. The scheduler removes the canceled waiter before advancing the round-robin
   cursor, or returns an admission-canceled result if cancellation wins after a
   grant notification.
3. The canceled request starts no Git subprocess and does not consume a slot or
   count as a post-admission failure.

### Fresh status keeps interactive classification

1. A user requests fresh status while the tracker is also capable of polling.
2. Every Git subprocess spawned for that fresh observation is admitted as
   `interactive`, while periodic polling remains `background`.
3. Lock-suppression environment settings may differ by caller, but command
   scheduling and per-class metrics reflect the user-visible request.

### Raw Git execution cannot bypass admission

1. A production code path constructs a Git command for lifecycle, interactive,
   or background work.
2. It invokes a classified shared helper that acquires the process-wide Git
   controller before starting the command.
3. A repository guard fails when production code directly runs a raw Git command
   or uses an unclassified execution path.

## Out of scope

- Raising the default Git concurrency cap or introducing platform-specific defaults.
- Removing the process-wide cap or replacing Git subprocesses with a native library.
- Serializing all instance creation independently of Git admission.
- Changing public status, log, or diff payloads.
- Aggregating admission snapshots across Docker or remote agentctl processes.
- Guaranteeing wall-clock completion when Git itself hangs; execution timeouts
  continue to bound commands after admission.

## Success criteria

- Deterministic tests prove the hard global cap, same-class FIFO order,
  work-conserving behavior, cancellation safety, and bounded progress across all
  three classes.
- A saturated interactive workload cannot stop a healthy workspace tracker due
  only to admission delay.
- Saturated background work cannot starve interactive or lifecycle operations.
- Status, log, and diff fan-out share the same capacity-derived policy.
- Aggregate and per-class diagnostics attribute contention to admission wait versus
  command execution.
- Before closing GitHub issue #2150, current `main` and the completed change are
  measured on Windows with the same workload. Record p50, p95, and maximum
  latency; timeout and tracker-stop counts; and peak Git process count with the
  cap left at 12. Host-dependent values are evidence for closure, not fixed CI
  thresholds.

## Decisions

- [Class-aware Git subprocess admission](../../../decisions/2026-08-02-class-aware-git-subprocess-admission.md)

## Implementation plan

- [Git subprocess admission plan](../../../plans/git-subprocess-admission/plan.md)
