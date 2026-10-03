---
created: 2026-09-20
status: complete
requirements:
  - REQ-DW-ORPHAN-001
  - REQ-DW-ORPHAN-002
system_design:
  - ../../specs/disambiguate-waiting/system-design/orphaned-background-workloads.md
legacy_specs:
  - ../../specs/disambiguate-waiting/spec.md
---

# Implementation Plan: Orphaned Background Workloads

## Overview

The background-workload liveness probe walks the agent's descendant processes
only. A workload whose parent shell exits is reparented to init, leaves that
tree while still running, and the probe reports `settled` — so the task card
drops its parked marker and renders as if the agent were waiting for the
operator.

This plan attributes such a workload back to its session using the
`KANDEV_SESSION_ID` environment variable that agentctl already injects and every
descendant already inherits. Inheritance survives reparenting, so no capture at
launch is needed. Where a platform cannot read another process's environment the
probe keeps today's descendant-only answer, and that limit is written down.

## Confirmed root cause

- `probe.transitiveDescendants` indexes the process table by `PPID` only, so a
  reparented process is not reachable from the agent root.
- `probe.processInfo` carries PID, PPID, StartTime and Zombie. It has no field
  that survives reparenting.
- `probe.probeWithReader` therefore returns `ResultSettled` while in-turn work is
  still running, and `orchestrator.recomputeParkedLocked` clears the projection
  because it requires `ProbeResultLive`.

Measured: a wrapper shell in the failing pattern lives about 4 ms, so no design
that must observe the shell is viable. Full evidence is in the paired system
design.

## Work orders

| Work order | Result | Wave |
|---|---|---|
| [task-01-attribute-orphaned-workloads.md](task-01-attribute-orphaned-workloads.md) | The probe reports `live` for a reparented in-turn workload where the platform allows it | 1 |
| [task-02-disclose-attribution-limits.md](task-02-disclose-attribution-limits.md) | The legacy failure-modes table records the reparenting row | 2 |

## Verification

Per work order. There is no E2E coverage by decision; the reasoning is in the
system design's E2E decision section.

## Risks

- Reading `/proc/<pid>/environ` per candidate adds work to a probe that runs
  under a latency budget. Bounded by running the pass only when the descendant
  walk found nothing, and only for in-turn non-descendant candidates; the
  candidate count was measured (tens, against a 250 ms budget) and the spec
  names the unbounded scan as an accepted tradeoff rather than leaving Build to
  invent a cap.
- Darwin gets no improvement, because the platform does not expose another
  process's environment. This is a disclosed limit, not a defect in the change.
