---
status: draft
system: tasks
created: 2026-09-09
owners:
  - kandev
---

# Workspace Orphan Process Reaping Requirements

## Overview

Terminal task cleanup stops the executions Kandev recorded, then removes the
task's local workspace directories. Its inventory (`sessions`, `worktrees`,
`stop_targets`, `task_environment`) names launched executions only, so it can
express "stop the execution I launched" and not "stop whatever this task left
running". It then reports `succeeded`, honestly: every removal it knew about
succeeded.

On 2026-09-08 an archived task left twelve shells at 1114% CPU on a 14-core
host, cwd still naming the deleted worktree eighteen hours later. They shared a
process group belonging to no recorded execution, and `executors_running` held
zero rows for the task. Host load reached 70 and three unrelated SSH tasks failed
their `ACP initialize` handshake. One archived task degraded the board, and
nothing reported it.

This closes that gap on the one fact true of a leftover process and false of a
legitimate one: after cleanup removes a workspace directory, nothing should still
be inside it. `REQ-TASKS-RUNTIME-CLEANUP-001` owns stopping recorded runtimes;
this owns the residue.

## Terminology

- **Reap root:** a local filesystem path this cleanup job removed and then
  confirmed absent. Task, Git worktree, and quick-chat session directories are
  all reap roots when this job removed them.
- **Candidate:** a process on this host whose resolved working directory is a
  reap root or a descendant of one.
- **Recorded execution:** an `executors_running` row. Its `local_pid` is the
  host-local agentctl process Kandev launched for a session; SSH rows carry a
  remote `pid` and are never host-local.
- **Protected process:** a process this capability may never signal, whatever its
  cwd, as defined in `REQ-TASKS-ORPHAN-REAP-003`.
- **Reap phase:** the phase of the durable task-resource cleanup job doing the
  work here. It runs only on a terminal outcome: the archive, delete, or cascade
  variants that produce a `task_resource_cleanup_jobs` row.

## Requirements

### REQ-TASKS-ORPHAN-REAP-001: Terminate processes left holding a removed task workspace

**Intent:** Make "the task is archived" mean "the task's processes are gone",
not "the processes Kandev recorded are gone".

#### Acceptance criteria

- **AC-TASKS-ORPHAN-REAP-001.1:** When a terminal cleanup job has removed a
  local task workspace path and confirmed it absent, the system shall record that
  path, resolved before removal while it still existed, as a reap root in the
  job's durable resource snapshot, and run the reap phase against it. A root
  persists for the life of the job, so a later attempt acts on the roots recorded
  by the attempt that removed them; no attempt after the first removes anything.
- **AC-TASKS-ORPHAN-REAP-001.2:** When a candidate holds a reap root and passes
  every check in `REQ-TASKS-ORPHAN-REAP-003`, the system shall terminate it under
  `REQ-TASKS-ORPHAN-REAP-004`.
- **AC-TASKS-ORPHAN-REAP-001.3:** When a workspace path was not removed by this
  job, including one preserved by an ownership or reference check and one whose
  removal failed, the system shall not record it as a root.
- **AC-TASKS-ORPHAN-REAP-001.4:** When a reap root exists again at reap time, the
  system shall skip it for that attempt.
- **AC-TASKS-ORPHAN-REAP-001.5:** When a task reached a terminal outcome before
  this capability shipped, the system shall not reap its leftover processes:
  this phase acts only on removals it performs.

### REQ-TASKS-ORPHAN-REAP-002: Identify candidates by working directory, on a path that is gone

**Intent:** Fix detection to the one signal that survives removal, and reject
the mechanisms that do not, so a mechanism that cannot see a deleted directory
is never chosen for a phase that runs after deletion.

#### Acceptance criteria

- **AC-TASKS-ORPHAN-REAP-002.1:** When the reap phase enumerates candidates, the
  system shall read each process's working directory, parent process identifier,
  and command name from one host process snapshot, and shall not require the reap
  root to exist on disk. On macOS that snapshot is `lsof -a -d cwd -F pcn` with
  `ps -Ao pid=,ppid=`; on Linux it is `/proc/<pid>/cwd`, whose target carries a
  ` (deleted)` suffix the system shall strip before comparison, with
  `/proc/<pid>/stat`. A process whose entry cannot be parsed is not a candidate.
- **AC-TASKS-ORPHAN-REAP-002.2:** When the reap phase enumerates candidates, the
  system shall not use a directory-walking search such as `lsof +D <path>`:
  against a removed path it returns no process list, having nothing to walk, and
  it matches open files as well as cwd.
- **AC-TASKS-ORPHAN-REAP-002.3:** When the system compares any path against a
  reap root, it shall compare fully resolved paths.
- **AC-TASKS-ORPHAN-REAP-002.4:** When the system compares any path against a
  reap root, it shall match only on a whole path component boundary, so a reap
  root of `<base>/task-a` never matches `<base>/task-abc`.
- **AC-TASKS-ORPHAN-REAP-002.5:** When the system enumerates candidates, it
  shall match on working directory alone, never on process name, command line, or
  a recorded execution's process group.
- **AC-TASKS-ORPHAN-REAP-002.6:** When the host process snapshot cannot be read,
  is unavailable on the platform, or does not complete within its timeout, the
  system shall reap nothing for that attempt and record a skip carrying the
  reason.
- **AC-TASKS-ORPHAN-REAP-002.7:** When a working directory is inside more than
  one reap root, a worktree directory inside a task directory, the system shall
  attribute it to the longest matching root and reap it once.

### REQ-TASKS-ORPHAN-REAP-003: Never signal a process this task does not own

**Intent:** Guarantee the phase can only kill a process whose abandonment is
positively established. Signalling is irreversible and its failure mode is
destroying another user's running work, so an inconclusive check is treated
exactly like a positive finding of ownership elsewhere.

#### Acceptance criteria

- **AC-TASKS-ORPHAN-REAP-003.1:** When a candidate's working directory is not
  inside any reap root of this job, the system shall not signal it.
- **AC-TASKS-ORPHAN-REAP-003.2:** When the cleaned task does not exclusively own
  a reap root, the system shall reap nothing for that root. Exclusive ownership
  means no session of any other task names a path equal to, inside, or containing
  that root in `task_sessions.workspace_path` while in one of the live states
  `CREATED`, `STARTING`, `RUNNING`, `IDLE`, or `WAITING_FOR_INPUT`. A session
  whose `workspace_path` is empty names no path. A subtask whose
  `workspace_mode` is `inherit_parent` or `shared_group` occupies a workspace it
  does not own.
- **AC-TASKS-ORPHAN-REAP-003.3:** When a candidate is the `local_pid` of a
  recorded execution belonging to another task, or a descendant of one, the
  system shall not signal it and shall record a skip naming the owning task.
  Descendancy is walked over the parent identifiers in that same snapshot. A
  process reparented away from its launcher has no ancestry left to walk, so this
  check cannot see it and `AC-TASKS-ORPHAN-REAP-003.4` covers that case.
- **AC-TASKS-ORPHAN-REAP-003.4:** When another task's live recorded execution
  names a worktree path equal to or inside a reap root, the system shall reap
  nothing for that root.
- **AC-TASKS-ORPHAN-REAP-003.5:** When a candidate is a protected process, the
  system shall not signal it. The protected set is: any process identifier of 1
  or below, as `kill` reads zero and negative identifiers as process groups; the
  Kandev backend process itself; any ancestor of the Kandev backend process,
  walked over the same parent identifiers; and the process running the reap
  phase.
- **AC-TASKS-ORPHAN-REAP-003.6:** When a check cannot reach a conclusion, the
  system shall fail closed at the narrowest unit that check governs: one bearing
  on a single candidate, such as an unresolvable working directory, skips that
  candidate alone; one bearing on a root, such as a repository error on
  ownership, reaps nothing for that root; and an unreadable snapshot is the whole
  phase under `AC-TASKS-ORPHAN-REAP-002.6`. One unresolvable process shall never
  suppress a root.
- **AC-TASKS-ORPHAN-REAP-003.7:** When the system is about to send any signal, it
  shall first re-read that one process's working directory (`lsof -a -p <pid> -d
  cwd -F n`, or `/proc/<pid>/cwd`) under the same resolution and suffix rules and
  confirm it is still inside a reap root, skipping the candidate where that read
  fails or resolves outside. A process identifier can be reused between
  enumeration and signalling, and a reused identifier belongs to an unrelated
  process. This read is not a snapshot and falls outside
  `AC-TASKS-ORPHAN-REAP-007.2`'s bound.

### REQ-TASKS-ORPHAN-REAP-004: Escalate from SIGTERM to SIGKILL, per process

**Intent:** Give a leftover process the same chance to exit cleanly a recorded
execution gets, and the same guarantee that refusing will not save it. It may
flush its output before it dies; it may not ignore the request.

#### Acceptance criteria

- **AC-TASKS-ORPHAN-REAP-004.1:** When the system reaps a candidate, it shall
  send `SIGTERM` to that process identifier alone, never the process group.
- **AC-TASKS-ORPHAN-REAP-004.2:** When a candidate has not exited within a
  2 second grace period after `SIGTERM`, the system shall send `SIGKILL` to that
  process identifier. Exit is observed with signal `0`: a candidate is not a child
  of this process and cannot be waited on. The 2 seconds match
  `internal/agentctl/server/process/runner.go:485`; only the duration is
  borrowed, as that code waits on an owned child and kills a process group, both
  of which this contract forbids.
- **AC-TASKS-ORPHAN-REAP-004.3:** When the system sends `SIGTERM` to a set of
  candidates, it shall send to all of them before starting the grace period,
  never serializing a 2 second wait per candidate.
- **AC-TASKS-ORPHAN-REAP-004.4:** When a signal fails because the process no
  longer exists, the system shall record the outcome as terminated and shall not
  report an error.
- **AC-TASKS-ORPHAN-REAP-004.5:** When a signal fails because the system lacks
  permission, it shall record a skip carrying the reason and shall not treat it
  as retryable.
- **AC-TASKS-ORPHAN-REAP-004.6:** When a candidate is still alive 1 second after
  `SIGKILL`, the settle window an uninterruptible sleep needs, and the
  re-verification in `AC-TASKS-ORPHAN-REAP-003.7` still places it inside a reap
  root, the system shall record it as survived and return a retryable error.
- **AC-TASKS-ORPHAN-REAP-004.7:** When the reap phase's retryable errors exhaust
  the job's retry ladder and it reaches its failed state, the system shall leave
  the workspace removals that already succeeded in place.

### REQ-TASKS-ORPHAN-REAP-005: Report every reap, and every deliberate skip

**Intent:** Remove the silence: a job reporting `succeeded` while leaving eleven
cores running is why the incident lasted eighteen hours.

#### Acceptance criteria

- **AC-TASKS-ORPHAN-REAP-005.1:** When the reap phase completes, the system
  shall persist one record per candidate in the job's durable resource snapshot,
  carrying the process identifier, resolved working directory, matched reap
  root, command name, and an outcome of terminated, killed, skipped, or
  survived.
- **AC-TASKS-ORPHAN-REAP-005.2:** When an outcome is skipped or survived, the
  system shall record a reason with it.
- **AC-TASKS-ORPHAN-REAP-005.3:** When the reap phase terminates or kills at
  least one candidate, the system shall emit a structured warning-level log
  naming the task, the count, and each process identifier with its working
  directory.
- **AC-TASKS-ORPHAN-REAP-005.4:** When the reap phase skips a root or a
  candidate, it shall emit a structured informational log carrying the reason.
- **AC-TASKS-ORPHAN-REAP-005.5:** When the reap phase runs, the system shall
  increment install-wide counters on `/debug/vars` under an `orphan_reap_*`
  prefix: candidates seen, terminated, killed, skipped by reason, and survived.
- **AC-TASKS-ORPHAN-REAP-005.6:** When the reap phase finds no candidates, it
  shall record an empty result, emit no warning, and leave the job's terminal
  state unchanged.
- **AC-TASKS-ORPHAN-REAP-005.7:** When the reap phase skips a whole root or the
  whole phase, leaving no candidate record to carry the reason, the system shall
  persist a root-level or phase-level skip record carrying it. A recorded skip
  shall leave the cleanup job's terminal state unchanged: only
  `AC-TASKS-ORPHAN-REAP-004.6`, `AC-TASKS-ORPHAN-REAP-006.3`, and
  `AC-TASKS-ORPHAN-REAP-007.5` fail a job on this phase's account.

### REQ-TASKS-ORPHAN-REAP-006: Ordering, idempotency, and concurrency

**Intent:** State sequencing and repeat semantics explicitly, so a second
attempt after a partial failure is safe and predictable: this phase runs inside
a job that retries, can be cancelled mid-flight, and runs concurrently with
other tasks' cleanup jobs.

#### Acceptance criteria

- **AC-TASKS-ORPHAN-REAP-006.1:** When a cleanup job executes, the reap phase
  shall run once per attempt, after every workspace removal in that attempt and
  after remote task-directory reclamation, as its last phase.
- **AC-TASKS-ORPHAN-REAP-006.2:** When any runtime stop for the job failed, the
  system shall skip the reap phase for that attempt, as the existing rule gating
  remote reclamation on a clean stop does.
- **AC-TASKS-ORPHAN-REAP-006.3:** When the job's context is already cancelled at
  the start of the reap phase, the system shall skip the phase and send no
  signal, so a backend shutdown never begins a termination it cannot supervise.
  When it is cancelled after the phase has begun, the system shall send no
  further signal, persist each candidate it already signalled as skipped with a
  reason naming the signal already sent, and return a retryable error.
- **AC-TASKS-ORPHAN-REAP-006.4:** When the system processes candidates, it shall
  order them by ascending process identifier.
- **AC-TASKS-ORPHAN-REAP-006.5:** When a cleanup job is retried, the reap phase
  shall re-enumerate from a fresh snapshot rather than reuse the previous
  attempt's list. Reap records are keyed by process identifier: a re-detected
  candidate takes this attempt's outcome, superseding any earlier record for that
  identifier; one not re-detected keeps the record it has. A later empty attempt
  must not erase those records.
- **AC-TASKS-ORPHAN-REAP-006.6:** When a retry finds every previous candidate
  gone, the reap phase shall complete with an empty result and shall not fail
  the job.
- **AC-TASKS-ORPHAN-REAP-006.7:** When two cleanup jobs for different tasks run
  concurrently, each shall act only on its own reap roots; where both
  reference one shared workspace, `AC-TASKS-ORPHAN-REAP-003.2` stops the
  non-owning job.
- **AC-TASKS-ORPHAN-REAP-006.8:** When the same candidate is signalled more than
  once, whether by a retry or a concurrent job, the system shall treat the
  duplicate as success, not an error.
- **AC-TASKS-ORPHAN-REAP-006.9:** When a process starts inside a reap root after
  the attempt's snapshot was taken, the system shall not reap it in that attempt
  and shall not fail the job on its account.

### REQ-TASKS-ORPHAN-REAP-007: Bounded cost, and an explicit platform contract

**Intent:** Keep a phase that runs on every terminal cleanup from becoming a
cost, and state what happens where it cannot work, so an operator archiving
fifty tasks sees cleanup stay as fast as it is today.

#### Acceptance criteria

- **AC-TASKS-ORPHAN-REAP-007.1:** When a cleanup job has no reap roots, the
  system shall skip the reap phase entirely without reading the host snapshot.
- **AC-TASKS-ORPHAN-REAP-007.2:** When the reap phase reads the host process
  snapshot, it shall take at most one per attempt regardless of reap root count,
  bounded by a combined timeout of at most 5 seconds spanning every read that
  snapshot makes, the duration the `lsof` invocation in
  `internal/agentctl/server/api/port_listener.go` already uses. The per-candidate
  re-verification of `AC-TASKS-ORPHAN-REAP-003.7` falls outside this bound.
- **AC-TASKS-ORPHAN-REAP-007.3:** When the reap phase runs on a workspace with no
  candidates, its added latency shall be the snapshot alone.
- **AC-TASKS-ORPHAN-REAP-007.4:** When the reap phase runs on Windows, the
  system shall skip it, record a skip naming the platform, and leave the job's
  terminal state unchanged.
- **AC-TASKS-ORPHAN-REAP-007.5:** When more than 256 candidates would be
  signalled for one job, the system shall signal the first 256 in the
  deterministic order of `AC-TASKS-ORPHAN-REAP-006.4`, record that the bound was
  reached, and return a retryable error so a later attempt takes the remainder.
  The bound counts candidates the system attempts to signal: one skipped under
  `REQ-TASKS-ORPHAN-REAP-003` or `AC-TASKS-ORPHAN-REAP-004.5` does not consume it,
  so a persistently unsignalable process cannot starve those behind it.

## Prior art

**Our own prior reasoning (wiki).** Searched: nothing. Receipt: vault
`/Users/henry/Documents/henry/wiki` via the `@henry` pin,
`QMD_WIKI_COLLECTION=wiki`; every read under it returns `EPERM` from this session
(macOS protection on `~/Documents`), confirmed with `ls`, `cat` and the sandbox
disabled, and `qmd` is not on `PATH`. A blocked leg, not an empty result.

**What other products shipped (saas-kb).** Searched: `search_fsm_docs`,
`category: "ai_sdlc"`, four queries on orphan cleanup at teardown, background
command lifetime, sandbox teardown, and host resource exhaustion. Claude Code
documents the behavior that produced the incident: a background command from the
main conversation "keeps running after a final response", so no fix belongs on
that side. Every competitor covered answers this class by isolation, not reaping:
OpenHands and the Claude Agent SDK guide containerize, so teardown is a container
kill, and Kiro watches host memory pressure. None documents reaping by working
directory; relevance was at or below 0.016 on all four.

**What we do differently.** Kandev's `local_pc` and worktree executors run on
the host by design, so container teardown is unavailable and host-wide pressure
limits would penalize healthy tasks for one bad one. We take the narrower rule
the incident supports: a removed workspace positively states that nothing inside
it is wanted, making cwd containment sufficient evidence of abandonment without
name matching, and keeping this inside the boundary
`REQ-TASKS-RUNTIME-CLEANUP-001` set when it excluded a name-based sweeper.

## Out of scope

- **Remote SSH runners.** Reaping an orphaned `agentctl` or its descendants on
  a remote host is card `f860757f`. Detection here reads a local snapshot and
  cannot see a remote one, and remote reclamation has its own opt-in, safety
  probes, and specification in
  `docs/specs/executors/requirements/remote-task-directory-reclamation.md`.
- **The 2026-09-08 load-generator script.** An ad-hoc tool call, not committed
  code (`grep loadpids` over the tree returns nothing), so there is no
  repository-side script to add a `trap` to.
- **A background sweeper for pre-existing orphans.** Processes left by a task
  cleaned up before this shipped are not swept: calling a process an orphan with
  no removal to reason from is a different, larger safety problem.
- **A name-based or command-line sweeper.** Excluded by the design for
  `REQ-TASKS-RUNTIME-CLEANUP-001` and not reopened here.
- **Preventing the leak at spawn time.** A supervised process group inherited by
  every task-spawned process, or a workspace lease a process must renew, would
  stop the leak earlier, but both change how agents launch work rather than how
  cleanup ends it.
- **Windows support.** `AC-TASKS-ORPHAN-REAP-007.4` makes the phase a recorded
  no-op there.
- **A user-visible surface.** Cleanup jobs have no API or UI surface today and
  this adds none: reporting is the snapshot, logs, and `/debug/vars`.
- **Reaping on non-terminal stops.** An ordinary stop, a rollback, and a backend
  shutdown preserve the workspace, so there is no removal and no root.
