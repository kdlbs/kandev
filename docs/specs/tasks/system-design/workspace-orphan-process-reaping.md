---
status: draft
system: tasks
requirements:
  - REQ-TASKS-ORPHAN-REAP-001
  - REQ-TASKS-ORPHAN-REAP-002
  - REQ-TASKS-ORPHAN-REAP-003
  - REQ-TASKS-ORPHAN-REAP-004
  - REQ-TASKS-ORPHAN-REAP-005
  - REQ-TASKS-ORPHAN-REAP-006
  - REQ-TASKS-ORPHAN-REAP-007
created: 2026-09-10
updated: 2026-09-10
owners:
  - build
---

# Workspace Orphan Process Reaping System Design

## Purpose and boundaries

This design adds a reap phase to the durable `task_resource_cleanup_jobs`
worker: after that worker removes a local workspace path and confirms it
absent, any host process whose resolved cwd is inside the removed path is
terminated. It closes the gap between "stop the execution this task
launched" (the worker's existing inventory) and "stop whatever this task left
running" (a process the worker's inventory never named).

It does not own workspace removal itself, remote/SSH directory reclamation
(`docs/specs/executors/system-design/remote-task-directory-reclamation.md`),
or name-based process sweeping (excluded by
`REQ-TASKS-RUNTIME-CLEANUP-001`'s design).

The requirements document
([`workspace-orphan-process-reaping.md`](../requirements/workspace-orphan-process-reaping.md))
went through four Spec Review rounds. Round 4 closed with nine open findings
(F19-F27) that the user explicitly accepted rather than reopening the spec —
see that document's "Implementation seam" and "USER DECISION — round 4"
sections. This design records the safe reading chosen for each one, with a
pointer back to its finding id, so a later reader can tell an accepted gap
from an unnoticed one.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| REQ-TASKS-ORPHAN-REAP-001 (reap roots) | [Root candidates and confirmation](#root-candidates-and-confirmation) |
| REQ-TASKS-ORPHAN-REAP-002 (detection) | [Host snapshot and attribution](#host-snapshot-and-attribution) |
| REQ-TASKS-ORPHAN-REAP-003 (ownership) | [Ownership](#ownership) |
| REQ-TASKS-ORPHAN-REAP-004 (signalling) | [Signal escalation](#signal-escalation) |
| REQ-TASKS-ORPHAN-REAP-005 (outcomes) | [Persistence and outcome records](#persistence-and-outcome-records), [Observability](#observability) |
| REQ-TASKS-ORPHAN-REAP-006 (job integration) | [Control flow](#control-flow) |
| REQ-TASKS-ORPHAN-REAP-007 (bounds) | [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

All new code lives in `internal/task/service`, alongside the worker it
extends (`resource_cleanup_jobs.go`), mirroring the shape of
`resource_cleanup_ssh.go`'s `reclaimSSHTaskDirs`:

- `resource_cleanup_orphan_reap.go` — root-candidate gathering and
  confirmation, phase orchestration (`runOrphanReapPhase`), skip/candidate
  recording, best-effort cross-attempt persistence.
- `resource_cleanup_orphan_reap_match.go` — cwd-to-root attribution
  (`attributeOrphanReapCandidates`, longest-match, AC-002.7).
- `resource_cleanup_orphan_reap_ownership.go` — the fail-closed ownership
  filter (`applyOrphanReapOwnership`), REQ-003.
- `resource_cleanup_orphan_reap_signal.go` — SIGTERM → SIGKILL escalation
  (`signalOrphanReapCandidates`), REQ-004.
- `resource_cleanup_orphan_reap_metrics.go` — the `orphan_reap_total`
  expvar counter map.
- `resource_cleanup_orphan_reap_host_{darwin,linux,other}.go` — platform host
  snapshotters and single-pid verifiers behind the `orphanReapHostSnapshotter`
  / `orphanReapVerifier` interfaces.
- `resource_cleanup_orphan_reap_signal_{unix,windows}.go` — the raw per-PID
  `syscall.Kill` (never a process group, AC-004.1) and its error
  classification (AC-004.4/004.5).

`Service` carries three lazily-defaulted, injectable fields
(`orphanReapHostSnapshotter`, `orphanReapVerifier`, `orphanReapSignaler`) so
tests can drive ownership, batching, cancellation, and outcome classification
against fabricated PIDs without ever issuing a real signal to a process this
process does not own — this feature signals processes Kandev never launched,
so a test sending a real signal to an arbitrary PID would risk affecting an
unrelated host process.

## Root candidates and confirmation

`gatherOrphanReapRootCandidates` runs **before** `performTaskCleanup`, using
the exact paths the job might remove: each worktree's path, its parent "task
directory" (`filepath.Dir(worktreePath)` — confirmed as the per-task
container directory by `worktree/manager_cleanup.go`'s
`tryRemoveEmptyTaskDir`), and each cleaned-up session's quick-chat directory.
Each candidate is only kept if it currently exists (`os.Lstat`); a worktree
path that never existed does not spuriously add its parent as a root — the
parent is only added when it exists independently of whether the worktree
itself was ever there. After `performTaskCleanup` runs,
`confirmOrphanReapRootsRemoved` re-`Lstat`s each candidate and keeps only the
ones now absent. This runs on **every** attempt (see [F21](#f21-ac-0011-a-root-is-recorded-on-whichever-attempt-actually-removes-it)),
and `mergeOrphanReapRoots` appends newly-confirmed roots into
`snapshot.OrphanReapRoots`, deduplicated, so a root persists for the life of
the job once recorded (AC-001.1).

## Host snapshot and attribution

`runOrphanReapPhase` takes one whole-host process snapshot per attempt
(`orphanReapSnapshotTimeout` = 5s, AC-002.6/007.2), via the platform
`orphanReapHostSnapshotter`:

- **darwin**: `lsof -a -d cwd -F pcn` for the whole-host cwd table, `ps -Ao
  pid=,ppid=` for ancestry (the input-inventory receipts in the requirements
  doc measured this at 0.170s / 970 resolvable cwds of 1331 processes).
- **linux**: `/proc/<pid>/cwd` (stripping a trailing `" (deleted)"`) and
  `/proc/<pid>/stat`'s ppid field.
- **other** (including windows): `errOrphanReapUnsupportedPlatform`, handled
  by the phase as a skip, never a job failure (AC-007.4).

`attributeOrphanReapCandidates` matches each process's cwd to the **longest**
(deepest) root it falls under (`orphanReapPathWithinRoot`, a
component-boundary-safe prefix check — `/tasks/task-a` does not match
`/tasks/task-abc`), so a worktree root nested inside its task directory root
attributes independently (AC-002.7). A process with an empty/unparsed cwd
is dropped by the platform reader and never reaches attribution.

## Ownership

`applyOrphanReapOwnership` (REQ-003) queries `ListExecutorsRunning` and the
new `ListLiveWorkspaceSessions` **once** per phase, then applies checks at the
narrowest unit each governs (AC-003.6):

- A repository error, or an other-task stored session path that fails to
  resolve, is phase-wide inconclusive across every currently active root —
  `skipEveryOrphanReapRoot` records a **detection-failure** skip for each one
  (see [Observability](#observability)).
- A root equal to, inside, or **containing** another task's live session
  workspace is blocked (`orphanReapFindOverlap`, bidirectional — AC-003.2).
- A root equal to or **inside** another task's live recorded execution's
  worktree is blocked (`orphanReapFindContainment`, one-directional —
  AC-003.4). This asymmetry (containing vs. inside-only) is the literal text
  of the two ACs, not an implementation choice; see
  [F22](#f22-ac-0034-vs-ac-0032-asymmetric-containment-implemented-as-specified).
- A candidate whose PID or ancestor PID is another task's `local_pid` is
  skipped (AC-003.3), and every candidate is checked against a protected set
  of PID ≤ 1 plus the backend's own PID and its ancestry (AC-003.5).

### F19 (AC-003.2): effective session workspace path

The AC names `task_sessions.workspace_path`, but the repository's effective
path COALESCEs a `task_environments` override:
`COALESCE(NULLIF(te.workspace_path, ''), ts.workspace_path)`
(`repository/sqlite/session.go`'s `taskSessionSelectCols` /
`taskSessionFromClause`). Reading the raw column is a fail-open ownership
hole for a promoted multi-repo task. **Safe reading applied: query the
effective path.** Implemented as a new `SessionRepository.ListLiveWorkspaceSessions`
method reusing the existing COALESCE query, filtered to the five live states
AC-003.2 names (`CREATED`, `STARTING`, `RUNNING`, `IDLE`,
`WAITING_FOR_INPUT`) — deliberately a new method rather than widening
`ListActiveTaskSessions`, because that method's several unrelated callers
must not start seeing `IDLE` sessions.

### F20 (AC-003.4): "live" recorded execution

`executors_running.status` has seven values
(`starting`/`running`/`ready`/`failed`/`stopped`/`completed`/`prepared`);
nothing in the AC or Terminology defines "live". **Safe reading applied:**
`orphanReapExecutorIsLive` treats a row as live unless its status is
`failed`, `stopped`, or `completed` — over-protective by construction,
matching REQ-003's stated fail-closed posture.

### F22 (AC-003.4 vs AC-003.2): asymmetric containment, implemented as specified

AC-003.2 protects a path "equal to, inside, or containing" a root;
AC-003.4 protects only "equal to or inside". Spec Review left this asymmetry
in the frozen contract rather than resolving it. This design implements both
ACs literally (`orphanReapFindOverlap` vs. `orphanReapFindContainment`) and
does not attempt to make the two checks symmetric — that would be an AC
change, not an implementation detail.

### F26 (AC-002.3 + AC-003.6): unresolvable stored path fails every active root closed

AC-002.3 requires resolve-then-compare for "any path"; ownership compares
other tasks' stored session paths, which need not exist on disk, so
`filepath.EvalSymlinks` can fail on a live install with no error on Kandev's
part. Per AC-003.6 this is fail-closed, and `otherTaskSessionPaths` applies it
literally: **any** unresolvable other-task path makes every currently active
root inconclusive for this attempt, not just the root it happens to resemble.
F26's viability concern — one stale row can make the feature permanently
inert on a long-lived install — was raised and not mitigated in the spec (the
round-3 re-read declined a clause on the safety axis; round 4 raised the
viability axis and the user's round-4 decision left the spec unchanged). This
design implements the literal fail-closed behavior and relies on the Warn-level
detection-failure logging in [Observability](#observability) to surface a
persistently-inert phase to an operator, rather than silently and permanently
signalling nothing.

### F21 (AC-001.1): a root is recorded on whichever attempt actually removes it

`executeTaskResourceCleanupJob` runs `performTaskCleanup` on every one of up
to `taskResourceCleanupMaxAttempts = 8` attempts, so "no attempt after the
first removes anything" is not true of the job this phase joins. **Safe
reading applied: `confirmOrphanReapRootsRemoved` runs every attempt**, and
`mergeOrphanReapRoots` appends whatever it newly confirms into the durable
`OrphanReapRoots` list — so a root removed on attempt 3 (for example, after a
dirty-worktree retry) is still recorded and reaped, not silently dropped
because it missed attempt 1.

## Signal escalation

`signalOrphanReapCandidates` (REQ-004) sends SIGTERM to every candidate
before the shared grace period starts (AC-004.3), re-verifying each
candidate's cwd immediately before every signal
(`orphanReapReverifyInsideRoot`, AC-003.7 — closing the PID-reuse window as
tightly as a non-`pidfd` design can). After `orphanReapGraceDelay` (2s,
matching `agentctl/server/process/runner.go`'s escalation window — only the
duration is borrowed, never its process-group kill), survivors are
re-verified and sent SIGKILL; after `orphanReapSettleDelay` (1s), any
candidate still alive is recorded `survived` and produces a retryable
`errOrphanReapCandidateSurvived`. Mid-phase cancellation
(AC-006.3) stops further escalation and records every candidate that had
already received a signal as `skipped`, naming the signal already sent; it
never issues a signal after `ctx.Done()`.

### F23 (AC-004.1/004.2 + AC-005.1): terminated vs. killed outcome mapping

No AC states which signal produces which of the four outcome values.
**Decision applied:** a candidate found dead (`Alive` reports `false`, or the
signal itself failed with the process already gone — AC-004.4) at the
SIGKILL-eligibility check is recorded `terminated`, because SIGTERM alone
ended it. A candidate that received SIGKILL and is found dead afterward is
recorded `killed`. This mapping is call-site-order-dependent, not signal-name
dependent: `sendOrphanReapSigkills` records `terminated` for anyone dead
*before* SIGKILL is sent; `resolveOrphanReapSurvivors` records `killed` for
anyone dead *after* it was sent. `AC-005.5`'s `orphan_reap_total` counters
(`terminated`, `killed`, `survived`, `skipped_candidate`) follow the same
per-record classification.

### F24 (AC-007.5): the 256-candidate bound counts every attempted candidate

The AC's two clauses conflict (the bound "counts candidates attempted", but
"a skip under REQ-003 or AC-004.5 does not consume it" — AC-004.5's
permission-denied outcome is only observable *after* an attempt).
**Decision applied: the bound is enforced before ownership-approved
candidates are signalled at all** — `runOrphanReapPhase` sorts `toSignal` by
PID and truncates to `orphanReapMaxCandidates` (256) before calling
`signalOrphanReapCandidates`. Every one of the 256 is attempted (signalled,
including any that resolve to AC-004.5 permission-denied), and the remainder
past 256 is never attempted at all this cycle — recorded as
`errOrphanReapCandidateBoundReached`, a retryable error so a later attempt
picks up the deferred remainder against the same durable root. This reads
AC-007.5's first clause as authoritative and treats the second clause's
"does not consume it" as describing candidates *filtered out before
attempting* (an ownership skip), not candidates that failed mid-attempt.

### F25 (AC-006.3 + AC-006.5): cancellation before any signal is sent

AC-006.3 requires persisting "each candidate it already signalled" on
mid-phase cancellation; AC-006.5 requires a re-detected candidate to take
"this attempt's outcome". Neither AC states what happens to a candidate that
was re-detected this attempt (survived ownership filtering, is in
`toSignal`) but the phase was cancelled **before any signal reached it**.
**Decision applied, literal AC-006.3 reading:** `signalOrphanReapCandidates`
checks `ctx.Err()` once at entry and returns immediately with no record
written for any candidate if the context is already cancelled — there is no
"signal already sent" to name, so AC-006.3 has nothing to persist. A
candidate in that state keeps whatever record a **previous** attempt already
wrote for its PID (unchanged, not superseded, because `recordOrphanReapCandidate`
only supersedes on a fresh write — AC-006.5), or has no record at all if this
was its first detection. This is the accepted gap F25 names: a record that
should arguably reflect "seen again this attempt, not yet reached" does not
exist. Once at least one SIGTERM has been sent, cancellation during the grace
or settle wait is fully covered — every already-signalled candidate is
recorded `skipped`, naming the signal already sent
(`recordOrphanReapCancelledMidPhase`).

### F27 (AC-005.7 + AC-006.5): root/phase skip records have no supersession

AC-006.5 defines pid-keyed supersession for candidate records; AC-005.7's
root-level and phase-level skip records have no key. **Implemented as
specified: skips are append-only** (`recordOrphanReapRootSkip` and its
detection-failure/phase variants always append to `snapshot.OrphanReapSkips`,
never replace an entry for the same root). A later attempt that successfully
reaps a previously-skipped root leaves that root's earlier "skipped: not
exclusively owned" record in place beside the fresh candidate outcomes for
the same root. This can mislead a snapshot reader about a root's current
state, but has no unsafe runtime effect — accepted per F27's LOW severity and
the round-4 user decision not to reopen the spec for it.

## Control flow

`runOrphanReapPhase` is the last phase of `executeTaskResourceCleanupJob`
(AC-006.1), gated exactly like `reclaimSSHTaskDirs` above it: only when
`len(failedStops) == 0` (AC-006.2) and only after every earlier
`context.Cause(ctx)` check in the job has passed (AC-006.3's pre-start
clause). Order within the phase: merge durable roots → skip a root that
exists again at reap time (AC-001.4) → take the host snapshot → attribute
candidates to roots → apply ownership → cap and sort → signal. A snapshot
that fails to attribute any candidate, or ownership that clears every
candidate, ends the phase with no error and no warning-level record
(AC-005.6) — a deliberate skip still reports the cleanup job `succeeded`
(upheld three times across Spec Review rounds).

## Failure and recovery

Every failure mode fails at the narrowest unit it governs (AC-003.6):

| Failure | Scope | Job outcome |
| --- | --- | --- |
| Unsupported platform | Whole phase | Skip, job succeeds |
| Host snapshot unreadable/timed out (AC-002.6) | Whole phase | Skip, job succeeds |
| Ownership repository error / unresolvable other-task path | Every currently active root | Skip, job succeeds |
| A root exists again at reap time (AC-001.4) | That root only | Skip, job succeeds; root stays recorded |
| PID reused/moved before a signal (AC-003.7) | That candidate only | Skipped candidate |
| Permission denied sending a signal (AC-004.5) | That candidate only | Skipped candidate, non-retryable |
| A candidate survives SIGKILL | That candidate | `errOrphanReapCandidateSurvived`, retryable |
| 256-candidate bound reached (AC-007.5) | Deferred remainder | `errOrphanReapCandidateBoundReached`, retryable |
| Context cancelled mid-phase (AC-006.3) | Already-signalled candidates recorded skipped | `errOrphanReapCancelledMidPhase`, retryable |

A retryable error from this phase folds into the cleanup job's existing
8-attempt backoff ladder (`taskResourceCleanupMaxAttempts`,
`taskResourceCleanupRetryDelays`) — this phase adds no retry mechanism of its
own.

## Persistence

No schema change. Three new fields on the existing durable
`taskResourceCleanupSnapshot` JSON payload
(`task_resource_cleanup_jobs.resource_snapshot`):

- `orphan_reap_roots` (`[]string`) — every root confirmed removed across the
  job's life, append-only, deduplicated (AC-001.1).
- `orphan_reap_records` (`[]orphanReapCandidateRecord`) — one entry per pid
  ever seen, keyed by pid, superseded on re-detection
  (`recordOrphanReapCandidate`, AC-006.5).
- `orphan_reap_skips` (`[]orphanReapSkipRecord`) — root-level and phase-level
  skips, append-only ([F27](#f27-ac-0057--ac-0065-rootphase-skip-records-have-no-supersession)).

`persistOrphanReapProgressBestEffort` saves these three fields after a
**failed** cleanup attempt, via `UpdateClaimedTaskResourceCleanupSnapshot`,
before the worker's normal retry path runs. Without this, AC-006.5's
cross-attempt supersession would have nothing durable to supersede: the job
only otherwise persists its final snapshot on success. This save is
best-effort — a failure to persist here is logged and does not become part of
the attempt's own error, since the attempt is already retrying for an
unrelated reason.

## Security

Every signal is sent per-PID via a raw `syscall.Kill(pid, sig)`, never a
process group (`-pid`) — AC-004.1 forbids collateral damage to unrelated
processes sharing a group. The [Ownership](#ownership) section is this
feature's authorization boundary: it runs before any signal is sent, and its
every branch fails closed. The [protected set](#ownership) (PID ≤ 1, the
backend's own PID, and its ancestry) is re-derived from the same host
snapshot every attempt, so it cannot go stale.

## Observability

`orphan_reap_total` (`expvar.NewMap`, dev-gated at `/debug/vars` like every
other counter family) is keyed by outcome: `seen`, `terminated`, `killed`,
`survived`, `skipped_candidate`, `skipped_root`, `skipped_phase`,
`cap_reached` (AC-005.5). Every signalled outcome (terminated/killed/survived)
logs at **Warn** so an operator scanning logs sees any reap without opting in
(AC-005.3); a skipped candidate logs at **Info** (AC-005.4).

Root- and phase-level skips split by cause, per the requirements document's
"Implementation seam" note (carried into this design, not the requirements,
because it is a logging-severity decision): a **benign** skip — a completed
check that found real ownership, or AC-001.4's "root exists again" — logs at
Info via `recordOrphanReapRootSkip` / `recordOrphanReapPhaseSkip`. A
**detection-failure** skip — the check itself could not run (a repository
error, an unresolvable other-task path, or AC-002.6's unavailable host
snapshot) — logs at **Warn** via `recordOrphanReapRootSkipDetectionFailure` /
`recordOrphanReapPhaseSkipDetectionFailure`, because the host conditions
causing a detection failure are exactly what this feature exists to guard
against, and a silent permanent Info-level skip (see
[F26](#f26-ac-0023--ac-0036-unresolvable-stored-path-fails-every-active-root-closed))
would leave that guard invisible.

## Related decisions

No new ADR: every durable choice here resolves an accepted gap in an already
frozen, spec-reviewed contract (`workspace-orphan-process-reaping.md`), not a
fresh architectural boundary.
