---
status: draft
system: disambiguate-waiting
created: 2026-09-19
owners:
  - kandev
---

# Orphaned background workloads

System boundary, terminology and related systems are in
[../README.md](../README.md). Evidence, platform measurements and the E2E
decision are in
[../system-design/orphaned-background-workloads.md](../system-design/orphaned-background-workloads.md).

## Why

The liveness probe finds background work by walking the agent process's
descendants. A workload whose parent shell exits is reparented to init and
leaves that tree while still running, so the probe reports `settled`, the parked
projection drops the marker, and the card falls back to the ordinary
"waiting for input" rendering while the job is still going. That is the exact
wrong answer this system exists to prevent, and legacy `spec.md` calls a false
`settled` the expensive direction.

This is not the rare double-fork case. A single `cmd &` with no `wait` inside an
attested background shell is enough: the wrapper shell exits within a few
milliseconds and the job is orphaned.

## Requirements

### REQ-DW-ORPHAN-001: Attribute reparented workloads to their session

A workload that leaves the descendant tree because its parent exited is still
this session's work, and the probe shall keep reporting it while it runs.

Session identity is carried by the `KANDEV_SESSION_ID` environment variable,
which agentctl already injects at agent launch and which every descendant
inherits. Inheritance is unaffected by reparenting.

- **AC-DW-ORPHAN-001.1:** On a platform that can read another process's
  environment, when a process carries this session's identity, is not a zombie,
  and started at or after the truncated turn start, the probe shall report
  `live`, whether or not that process is still a descendant of the agent
  process. Where that capability is absent this criterion imposes nothing and
  AC-DW-ORPHAN-002.1 governs the result; the two shall never be read as
  demanding different answers for one process.
- **AC-DW-ORPHAN-001.2:** When every process carrying this session's identity
  started strictly before the truncated turn start, that set alone shall not
  produce `live`. A workload leaked by an earlier turn shall not hold the
  affordance open.
- **AC-DW-ORPHAN-001.3:** When a process started at or after the truncated turn
  start but does not carry this session's identity and is not a descendant of
  the agent process, it shall not contribute to the result.
- **AC-DW-ORPHAN-001.4:** A zombie shall be excluded whether it is reached as a
  descendant or by session identity, matching the existing descendant rule.
- **AC-DW-ORPHAN-001.5:** The result shall not depend on the order in which the
  process table is enumerated. For one snapshot, and for a process set whose
  liveness does not change while the probe runs, any enumeration order shall
  produce the same result. The qualifier is not a loophole: a candidate whose
  pid was recycled before the probe began shall be rejected under every
  enumeration order, because its start-time datum differs from the snapshot's
  whenever it is read.
- **AC-DW-ORPHAN-001.6:** The probe shall remain a read-only sample. Repeating
  it against an unchanged process table shall return the same result and shall
  change no session, projection or stored state.
- **AC-DW-ORPHAN-001.7:** Concurrent probes for the same session shall not
  observe each other. Session-identity matching shall introduce no state shared
  between probe calls.
- **AC-DW-ORPHAN-001.8:** The truncated turn start of legacy AC-80 shall be
  reused unchanged, and a process whose start time equals it shall count as
  started in-turn.
- **AC-DW-ORPHAN-001.9:** The agent process and any ancestor of it shall never
  contribute `live`, on either path. They carry the session identity themselves,
  and the existing rule already excludes the agent from its own descendant set.
- **AC-DW-ORPHAN-001.10:** A process shall be treated as carrying this session's
  identity only when the value of its `KANDEV_SESSION_ID` is exactly equal to
  the session id on the probe request. No prefix, case-insensitive or partial
  match shall be accepted. The environment shall be parsed as discrete
  `NAME=VALUE` entries and never scanned as raw text, so that a variable named
  `MY_KANDEV_SESSION_ID` or `KANDEV_SESSION_ID_OLD` shall not match. When
  `KANDEV_SESSION_ID` appears more than once in one environment, the first
  occurrence shall decide and the rest shall be ignored.
- **AC-DW-ORPHAN-001.11:** Re-validating a matched candidate shall compare the
  platform's own start-time datum — the value the operating system reports for a
  process's creation and keeps reporting unchanged for that process's whole life
  — and shall never compare two values each derived afresh from the current wall
  clock. The snapshot shall retain, for every process it records, whatever value
  that comparison needs. For one unchanged, still-running process, re-validation
  shall therefore succeed however much time has passed since the snapshot was
  taken; an implementation whose re-validation can reject a process that never
  exited does not satisfy this criterion. This is a different comparison from the
  turn-start predicate, which keeps using the derived start time and its
  truncation unchanged (AC-DW-ORPHAN-001.8).

### REQ-DW-ORPHAN-002: Disclose what the probe cannot attribute

Attribution depends on reading another process's environment, which not every
platform permits. Where it is unavailable the probe shall degrade to the
descendant-only answer it gives today rather than guess, and the limit shall be
recorded rather than left for an operator to discover.

- **AC-DW-ORPHAN-002.1:** On a platform that cannot read another process's
  environment, the probe shall return the result produced by the descendant walk
  alone. Behaviour on those platforms shall be unchanged by this capability.
- **AC-DW-ORPHAN-002.2:** When a candidate process's environment cannot be read
  because the process exited, or because it belongs to another user, that
  candidate shall be skipped and the probe shall continue. A skipped candidate
  shall not turn the result into `unknown`.
- **AC-DW-ORPHAN-002.2a:** When the re-validation read itself fails — because the
  candidate exited before it could be read, or for any other reason — that
  candidate shall be skipped and the scan shall continue, exactly as a candidate
  whose start-time datum no longer matches is skipped. Such a failure shall never
  produce `unknown`: the `unknown` of AC-DW-ORPHAN-002.3 is reserved for a
  failure of the process-table read, and shall not be reached from this step.
- **AC-DW-ORPHAN-002.3:** When the process-table read itself fails, the probe
  shall return `unknown`, unchanged from today. A failed table read shall never
  be reported as `settled`.
- **AC-DW-ORPHAN-002.4:** When the probe request carries an empty session
  identity, the probe shall skip the identity pass entirely, read no process
  environment, and return the result of the descendant walk alone. Together with
  the missing-capability case of AC-DW-ORPHAN-002.1 these are the only two
  preconditions that skip the pass, and this is the only one carried on the
  request; no other input shall suppress it.
- **AC-DW-ORPHAN-002.4a:** The probe shall never read the agent process's own
  environment. Matching compares a candidate against the session id on the
  request, so an agent launched without `KANDEV_SESSION_ID` has no descendant
  carrying one: the identity pass runs and matches nothing, and the
  descendant-walk result stands. That outcome is a consequence of ordinary
  non-matching, not a separate fallback; no implementation shall add a check for
  it and no test shall assert one.
- **AC-DW-ORPHAN-002.5:** When the descendant walk alone already establishes
  `live`, the probe shall not need to read any process environment.
- **AC-DW-ORPHAN-002.6:** The legacy failure-modes table in
  [../spec.md](../spec.md) shall gain a row for a reparented workload, stating
  which platforms attribute it and what the remaining platforms report.

## Out of scope

Named exclusions. Each is a contract, not an oversight.

1. **Windows.** It has no process-table reader at all and already answers
   `unknown` everywhere. It inherits this contract when that reader lands; no
   Windows behaviour changes here.
2. **Changing the parked projection formula.** The three-term formula, the
   revision epoch, the task-level counter and the sampler's stop conditions are
   untouched. This capability changes only which processes the probe counts.
3. **Notification behaviour.** Legacy AC-76 keeps notification independent of
   the parked projection, and nothing here alters delivery, timing or content.
4. **The probe's WebSocket request and response contract.** Both keep their
   current shape; the session identity the match needs is already on the
   request.
5. **A new environment variable.** `KANDEV_SESSION_ID` already exists, is
   already injected on every executor path, and is already registered in the
   configuration catalog. Adding a second identity token would create a contract
   where one already exists.
6. **Attributing work by process group.** Measured and rejected; the evidence is
   in the paired system design.
7. **Persistence.** Nothing here survives a restart of either process, matching
   the rest of this system.
8. **A cap on the number of candidates whose environment is read.** Deliberately
   absent. The pass is already narrowed to in-turn, non-descendant, non-zombie
   processes and runs only when the descendant walk found nothing; measurement
   in the paired system design puts the real candidate count in the tens, which
   is around a millisecond against the existing 250 ms probe budget. Scanning
   that set unbounded inside the budget is the accepted tradeoff. A cap, a
   deadline inside the probe, or an ordering heuristic over candidates are all
   excluded; adding one later needs evidence that the measured count was wrong,
   not a preference.
