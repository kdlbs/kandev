---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-RUNTIME-RECLAIM-003
---

# Agent Runtime Footprint System Design

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-EXECUTORS-RUNTIME-RECLAIM-003 | [Owned process observation](#owned-process-observation), [Current identity](#current-identity), [Workspace and UI projection](#workspace-and-ui-projection), [Measurement limits](#measurement-limits) |

## Owned process observation

The process manager observes the process tree rooted at its owned agent PID.
Windows uses a Toolhelp parent snapshot and process memory counters. Linux reads
parent links and resident pages from procfs. Other Unix platforms use one bounded
ps observation. Process names, command lines, credentials and resume tokens are
not projected. No teardown, reservation, capacity or retention decision consumes
these observations.

The existing authenticated agentctl control server exposes one GET footprint
read. Its instance manager copies instance pointers before measuring outside its
map lock. A busy process owner reports an unreadable marker instead of waiting
for lifecycle activity or blocking teardown. The read takes the startup lock
before the state lock, both nonblocking, so command/PID publication during
startup cannot race the observation. Existing ownership remains held while
walking the process tree; no start or stop is initiated by the read.

## Current identity

The standalone runtime borrows its existing authenticated control binding. The
lifecycle reader accepts only an unambiguous host-local reader and uses a bounded
request context. Per-session attribution requires the current task, session and
standalone instance identity under the existing execution-store lock. A reading
without that exact match contributes only to installation totals, never to a
guessed workspace. Remote executors do not acquire host-local attribution.

## Workspace and UI projection

The task service uses the existing workspace-read authorization before observing.
After a complete bounded runtime snapshot, it resolves each distinct measured
task through the canonical repository GetTask, including Quick Chat and hidden
automation runs. It filters by the authorized workspace instead of using the
ordinary task-list visibility filters. Unresolved ownership is an error rather
than a false measured zero; an incomplete runtime snapshot stays unknown.
The HTTP route emits only the workspace's rows and byte totals, plus the existing
installation runtime-count context.

The workspace settings card makes one bounded read when its workspace mounts.
The hook discards replies for an unmounted or replaced workspace. It registers no
settings draft or save callback and creates no timer, maintenance observer,
expvar registry or history. Desktop and narrow screens show the same loading,
unknown, failed, empty and partial states. All seven real locale catalogs and generated pseudo copy describe
unmeasured use as unknown and partial byte values as lower bounds.

```text
Desktop: workspace settings -> Agent memory in use
         Live sessions | Processes | Committed | Resident
         session identity             process count / resident bytes
Phone:   same card -> two-column totals, wrapping session rows
```

## Measurement limits

An unreadable owned process remains explicitly counted as unreadable. Byte
totals cover only readable processes. A failed or unavailable reader yields an
incomplete observation; it is never shown as measured zero. Unsupported commit
charge remains zero in the wire contract and is labelled not reported in the UI,
never estimated from RSS. The readings are snapshots and can change immediately
after observation. They do not authorize reclamation or prove process liveness
for scheduler admission.

## Verification boundary

Reuse source-owned footprint cases through temporary overlays outside normal
test collection. Cover process accounting/ancestry and real child measurement,
instance identity, current lifecycle binding, failed/partial reads, workspace
authorization and filtering, HTTP/client transport, stale UI replies and the
responsive card. Existing nearby read/auth tests are reused. No standing suite,
CI step, operating probe or automatic observer is introduced.
