---
id: "01-attribute-orphaned-workloads"
title: "Attribute orphaned background workloads"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-DW-ORPHAN-001
  - REQ-DW-ORPHAN-002
acceptance_criteria:
  - AC-DW-ORPHAN-001.1
  - AC-DW-ORPHAN-001.2
  - AC-DW-ORPHAN-001.3
  - AC-DW-ORPHAN-001.4
  - AC-DW-ORPHAN-001.5
  - AC-DW-ORPHAN-001.6
  - AC-DW-ORPHAN-001.7
  - AC-DW-ORPHAN-001.8
  - AC-DW-ORPHAN-001.9
  - AC-DW-ORPHAN-001.10
  - AC-DW-ORPHAN-001.11
  - AC-DW-ORPHAN-002.1
  - AC-DW-ORPHAN-002.2
  - AC-DW-ORPHAN-002.2a
  - AC-DW-ORPHAN-002.3
  - AC-DW-ORPHAN-002.4
  - AC-DW-ORPHAN-002.4a
  - AC-DW-ORPHAN-002.5
system_design:
  - ../../specs/disambiguate-waiting/system-design/orphaned-background-workloads.md
---

# Task 01: Attribute Orphaned Background Workloads

## Summary

Let the probe recognise a workload that left the descendant tree by
reparenting, by matching the `KANDEV_SESSION_ID` every descendant inherits
against the session id the probe request already carries. Keep today's answer on
any platform that cannot read another process's environment.

## In scope

- Add failing tests first, one per criterion:
  - a reparented in-turn workload carrying the identity reads `live`;
  - an identity-carrying process that started before the turn does not;
  - an in-turn process without the identity does not;
  - a zombie is excluded on both paths;
  - an unreadable candidate is skipped rather than escalating to `unknown`;
  - a failed process-table read still returns `unknown`;
  - an empty session id skips the pass and reads no environment at all;
  - a descendant hit short-circuits before any environment is read;
  - the agent process and its ancestors never contribute;
  - a session id that merely shares a prefix with ours does not match;
  - a variable named `MY_KANDEV_SESSION_ID` does not match, and a duplicated
    `KANDEV_SESSION_ID` resolves to its first occurrence;
  - a candidate whose start-time datum no longer matches the snapshot at re-read
    is skipped rather than matched;
  - an unchanged, still-running candidate re-validates successfully however much
    time has passed since the snapshot — the comparison uses the platform's
    invariant datum, never a value re-derived from the current wall clock
    (AC-DW-ORPHAN-001.11). On Linux this test MUST fail against an implementation
    that re-derives the start time through a second `/proc/uptime` read;
  - a re-validation read that fails outright skips the candidate and continues,
    and never turns the probe into `unknown` (AC-DW-ORPHAN-002.2a).
- Extend the `processTableReader` seam with an optional environment-read
  capability. Implement it on Linux over `/proc/<pid>/environ`. Do not implement
  it on Darwin; `SysctlRaw("kern.procargs2", …)` returns a 29-byte stub for
  another process and cannot supply the value.
- Thread the session id into `ProbeBackgroundWorkloads` and add the orphan pass
  after the descendant walk, in the order the system design specifies, including
  the upward PPID ancestor walk with its three stop conditions and the
  match-only start-time re-validation. The snapshot must retain, per process, the
  platform's invariant start-time datum (Linux: the raw `starttime` ticks from
  `/proc/<pid>/stat`; Darwin: the `p_starttime` timeval), because that is what
  re-validation compares — not the derived `StartTime` the turn-start predicate
  uses.
- Do NOT read the agent process's own environment (AC-DW-ORPHAN-002.4a). The
  spec forbids it; an implementation that adds the check is wrong even though it
  would pass the other criteria.
- Pass `req.SessionID` at the call site in
  `internal/agentctl/server/api/agent.go`.
- Add the three real-process-tree cases to `probe_realtree_test.go`. The
  re-validation-after-a-delay case cannot be a fake-reader unit test:
  `fakeProcessTableReader` replays stored `processInfo` values, so it returns an
  identical start time on a re-read and would pass against the very bug the case
  exists to catch.

## Out of scope

- The parked projection formula, the sampler, notifications, the WebSocket
  contract, any runtime flag, and Windows.
- Adding a new environment variable. `KANDEV_SESSION_ID` already exists.
- The legacy failure-modes row, which is task 02.

## Acceptance conditions

1. A reparented, non-zombie workload that started at or after the truncated turn
   start and carries the session's `KANDEV_SESSION_ID` makes the probe report
   `live` on Linux.
2. On a platform with no environment-read capability, every existing probe
   result is byte-for-byte what it is today.
3. No environment is read when the descendant walk alone already returned
   `live`.

## Verification

```sh
cd apps/backend
go test ./internal/agentctl/server/process/probe/... -run 'TestProbe' -v
go test ./internal/agentctl/server/api/... -run 'Probe'
make lint
```

The real-tree cases run for real on Linux. On Darwin they skip by design; the
orphan-attribution cases must be written so they skip there rather than fail.

## Likely files

- `apps/backend/internal/agentctl/server/process/probe/probe.go`
- `apps/backend/internal/agentctl/server/process/probe/processtable_linux.go`
- `apps/backend/internal/agentctl/server/process/probe/processtable_darwin.go`
- `apps/backend/internal/agentctl/server/process/probe/processtable_other.go`
- `apps/backend/internal/agentctl/server/api/agent.go`
- `apps/backend/internal/agentctl/server/process/probe/probe_test.go`
- `apps/backend/internal/agentctl/server/process/probe/probe_realtree_test.go`

## Risks

- `/proc/<pid>/environ` races with process exit. Treat every read error as
  "skip this candidate", never as a probe failure. The same holds for the
  re-validation read (AC-DW-ORPHAN-002.2a).
- **The re-validation comparison basis is the trap on this task.**
  `linuxBootTime()` anchors to `time.Now()` against `/proc/uptime`, so calling
  the reader again for one pid yields a start time that differs from the
  snapshot's for a process that never exited. Comparing derived values rejects
  every genuine match and reports `settled` for the live orphan — the defect this
  task exists to fix. `probe_realtree_test.go` already pins a fixed snapshot for
  this reason; read that comment before writing the re-read.
- The probe runs under `KANDEV_PARKED_PROBE_BUDGET`, but the candidate count is
  now measured and the spec names the unbounded scan as an accepted tradeoff
  (requirements *Out of scope* item 8). Do not invent a cap, an in-probe
  deadline or a candidate ordering; that would contradict the contract.
