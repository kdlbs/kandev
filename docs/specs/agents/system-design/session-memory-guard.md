---
status: current
system: agents
requirements:
  - REQ-AGENTS-SESSION-CEILING-003
created: 2026-10-20
owners:
  - kandev
---

# Session Memory Guard Design

## Boundary and mapping

| Criteria | Implementation boundary |
| -------- | ----------------------- |
| .1, .2   | `sessionCeilingController.admit`, `handOffOrAdmit`, and `decideLocked` in `orchestrator/session_ceiling.go`; `memoryPressuredLocked` and `coordinatorTaskMemoryExemption` in `session_ceiling_memory.go`; the card note in `ceiling_surface.go` |
| .3       | The existing ceiling retry sweep, which replays deferred launches through the same admission |
| .4       | `memoryPressuredLocked` fail-open path and `logMemoryUnavailableLocked` |
| .5       | `features.sessionMemoryGuard`, `ServiceConfig.SessionMemoryGuardEnabled`, and `hostmemory.ReadConfig` |

## Host reading

`internal/system/hostmemory` is a leaf package that only observes the host.
Windows reads `GlobalMemoryStatusEx` for physical memory and psapi
`GetPerformanceInfo` for system-wide commit charge. Linux reads
`/proc/meminfo` (`MemAvailable`, `Committed_AS`, `CommitLimit`) and
`/proc/sys/vm/overcommit_memory`. The commit pair is used only in strict mode
(`2`); under the default heuristic (`0`) and always (`1`) modes the kernel does
not enforce `CommitLimit` and `Committed_AS` routinely exceeds it, so only
available memory is judged there. `readingFromProc` carries no build tag so the
parsing is unit-tested on every platform. Other
platforms return an incomplete reading. `Gate.Pressured` is pure: available
memory below the floor, or commit percent above the ceiling when commit is
known, is pressure; an incomplete reading is unknown.

## Admission

Only automatic launches are judged. A manual launch returns "not pressured"
before the host is read, so it keeps its existing admission: the count-ceiling
override, and admission when the population cannot be read
(`decideUnknownPopulationLocked`). No manual caller (queue send-now,
clarification answers, initial-submission recovery, unattended wake, user
prompt resume) can therefore receive a memory refusal or lose its input.

For an automatic launch the controller reads the host inside its existing
critical section, only when the gate is enabled. A complete reading is reused
for one second (`memorySampleTTL`) so a burst of launches reads the host once;
failed and incomplete readings are not cached. The coordinator exemption reads
the task, so it is consulted only after the reading is pressured. A pressured host bypasses the
unlimited-ceiling fast path so that the population read can still recognize an
already-admitted session. `decideLocked` returns already-admitted sessions
first, then refuses a pressured automatic launch with `memory_pressure` before
the free-slot branch.
The refusal reserves no capacity and carries no population reading.
`handOffOrAdmit` keeps its counted-session hand-off before the memory check.

A refused launch takes the existing deferral path, so the card note, queue
order, and sweep replay are shared with count refusals. The note names the
host condition.

## Thresholds

The default floor is 8 GiB of available memory (8192 MiB). On a host with 16 GiB
or less, normal desktop use often leaves less than that available, so with the
flag on every automatic launch may be held. Such hosts should lower
`KANDEV_SESSION_MEMORY_MIN_AVAILABLE_MB` (for example to 2048) before enabling
the flag.

## Exemption

`NewService` passes `coordinatorTaskMemoryExemption`, which exempts tasks whose
origin is the Office coordinator. A task that cannot be read is not exempt.

## Verification

Orchestrator unit tests drive the controller with a fake reader: automatic
refusal, manual launches neither held nor reading the host (with a known and an
unknown population), the one-second sample reuse, the unlimited ceiling, the commit ceiling, counted sessions and
hand-offs, the exemption, fail-open and its single warning, the disabled gate,
and re-admission after pressure clears. Hostmemory tests cover `Pressured`,
`ReadConfig`, `readingFromProc` under each overcommit mode, and the live Windows
reader.
