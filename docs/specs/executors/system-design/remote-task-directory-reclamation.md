---
status: current
system: executors
requirements:
  - REQ-SSH-TASKDIR-RECLAMATION-001
  - REQ-SSH-TASKDIR-RECLAMATION-002
  - REQ-SSH-TASKDIR-RECLAMATION-003
  - REQ-SSH-TASKDIR-RECLAMATION-004
  - REQ-SSH-TASKDIR-RECLAMATION-005
  - REQ-SSH-TASKDIR-RECLAMATION-006
  - REQ-SSH-TASKDIR-RECLAMATION-007
---

# Remote Task-Directory Reclamation System Design

## Purpose and boundaries

Reclamation is a task-scoped step inside the existing durable task-resource
cleanup job, not a new subsystem and not a new background sweeper. The job
already owns the terminal-outcome trigger, the ordering against runtime stops,
the retry-with-backoff schedule, the cancellation path when an archive is undone,
and the durable `task_resource_cleanup_jobs` row that surfaces failure. This
design adds one phase to that job and one connection-level helper to the SSH
executor package.

The boundary that matters most is *where the removal does not go*.
`SSHExecutor.StopInstance` is the obvious place and is the wrong one, for three
independent reasons:

1. **Scope mismatch.** `StopInstance` is per session; a task directory is per
   task. A three-session task would evaluate removal three times, and the first
   session's removal would delete the directory out from under the other two
   while they are still stopping.
2. **Error laundering.** `stopTaskRuntimeTargets` deliberately swallows stop
   errors for targets already marked terminal
   (`if target.terminal { ...; continue }`) and for `runtimeStopAlreadyComplete`.
   A removal failure raised from `StopInstance` would be discarded on exactly the
   paths this feature runs on, producing the false-success shape that
   REQ-SSH-TASKDIR-RECLAMATION-005 exists to prevent.
3. **Missing knowledge.** Ownership (REQ-SSH-TASKDIR-RECLAMATION-003) is a
   question about tasks, environments, and sibling rows. The executor has an
   instance; the task service has the graph.

`StopInstance` therefore keeps its current contract unchanged: it never removes
the task directory. That is what makes AC-SSH-TASKDIR-RECLAMATION-004.1 and
AC-SSH-TASKDIR-RECLAMATION-004.2 structural rather than conditional — an ordinary
stop and a backend shutdown never create a cleanup job, so there is no code path
from them to a removal.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-SSH-TASKDIR-RECLAMATION-001` | [Control flow](#control-flow) |
| `REQ-SSH-TASKDIR-RECLAMATION-002` | [Safety probe](#safety-probe) |
| `REQ-SSH-TASKDIR-RECLAMATION-003` | [Ownership resolution](#ownership-resolution) |
| `REQ-SSH-TASKDIR-RECLAMATION-004` | [Purpose and boundaries](#purpose-and-boundaries) |
| `REQ-SSH-TASKDIR-RECLAMATION-005` | [Failure and recovery](#failure-and-recovery) |
| `REQ-SSH-TASKDIR-RECLAMATION-006` | [Data and contracts](#data-and-contracts) |
| `REQ-SSH-TASKDIR-RECLAMATION-007` | [Control flow](#control-flow) |

## Components and responsibilities

- **`lifecycle` package (`apps/backend/internal/agent/runtime/lifecycle`).** Owns
  a new `SSHTaskDirReclaimer` that, given a connection descriptor and a target
  directory, opens its own SSH connection, runs the read-only safety probes, and
  performs the guarded removal. It reuses `ResolveSSHTarget`, the existing
  fingerprint pinning, `runSSHCommand`, and `shellQuote`. It does not touch
  `SSHExecutor` session state, because by the time it runs every session has been
  stopped and its connection closed.
- **`task/service` package.** Owns the new cleanup-job phase: capturing the
  reclamation targets into the job snapshot at prepare time, resolving ownership,
  invoking the reclaimer, and folding the outcome into the job's error set.
- **`orchestrator/executor` package.** Owns projecting the new per-profile
  setting into launch metadata as an authoritative key.
- **Web settings.** Owns the per-profile toggle and its blast-radius copy.

## Data and contracts

### Per-profile setting

A new executor-profile config key `ssh_reclaim_task_dir` (string `"true"` to
enable; anything else, including absent and empty, disables). It is added to
`profileConfigAuthoritativeKeys` in
`apps/backend/internal/orchestrator/executor/executor_state.go`, not to
`profileConfigPassthroughKeys`. That distinction is the mechanism behind
AC-SSH-TASKDIR-RECLAMATION-006.3: authoritative keys are written unconditionally
from the profile, so a task that supplies `ssh_reclaim_task_dir` in its own
metadata is overwritten rather than honoured. This matches the existing treatment
of `ssh_workdir_root` and `ssh_shell`, which are authoritative for the same
reason — they are redirect vectors.

A matching `MetadataKeySSHReclaimTaskDir` is added to `persistentMetadataKeys` so
the value survives into `ExecutorRunning.Metadata` and is readable by the cleanup
job after a backend restart, alongside the connection tuple that
`ResumeRemoteInstance` already relies on.

Default disabled satisfies AC-SSH-TASKDIR-RECLAMATION-006.1 with no migration:
existing profile rows have no such key, and an absent key is disabled.

### Reclamation target

Captured into `taskResourceCleanupSnapshot` as a new field so the job is
self-contained and survives a restart:

| Field | Meaning |
| --- | --- |
| `host`, `port`, `user` | Connection identity |
| `host_fingerprint` | Pinned fingerprint; a mismatch fails the attempt |
| `identity_source`, `identity_file`, `proxy_jump` | Auth and routing |
| `workdir_root` | Profile workspace root |
| `remote_task_dir` | Absolute remote path recorded at launch |
| `shell` | Login shell used for remote commands |

Targets are gathered from `ListExecutorsRunningByTaskID` at prepare time and
de-duplicated by `(host, port, user, remote_task_dir)`, so a multi-session task
produces one target, and a task spanning two hosts produces two.

## Control flow

Inside `executeTaskResourceCleanupJob`, after the existing stop phase and the
existing `performTaskCleanup`, and only when `len(failedStops) == 0` and no
context cancellation is pending:

1. **Gate on the setting.** Skip targets whose snapshot does not carry
   `ssh_reclaim_task_dir == "true"`.
2. **Resolve ownership** (see below). A non-owned or unresolvable target is
   skipped and recorded, not failed.
3. **Validate the path shape.** The target must equal
   `<workdir_root>/tasks/<segment>` after normalization, with `<segment>`
   non-empty and containing no `/`, no `.`, and no `..`. Anything else is
   refused. This is a belt-and-braces guard for
   AC-SSH-TASKDIR-RECLAMATION-003.4; it makes an empty or corrupted task-dir name
   incapable of resolving to `<workdir_root>/tasks/` itself.

   The comparison is made against the *expanded* root. `ensureRemoteTaskDir`
   builds the recorded directory from `expandRemoteHome(workdir_root)`, while
   the profile stores the unexpanded form and stores nothing when the field is
   left blank. A literal `~/.kandev` is therefore resolved against the remote
   `$HOME` first, and a blank root falls back to the executor's own default, so
   the guard bounds the path instead of refusing every default-configured host.
   A root that needs no expansion issues no remote command, which is what keeps
   a refused path from touching the host at all.
4. **Connect** using the snapshot tuple and pinned fingerprint. An empty pinned
   fingerprint is an error: a launch cannot happen without a trusted host key,
   so its absence means the recorded connection is not one Kandev established
   and dialling would accept whatever key answered.
5. **Probe** (see below). Any at-risk or inconclusive verdict skips and records.
6. **Remove** with `rm -rf -- <shell-quoted path>`, then stat the path to confirm
   it is gone. A non-zero exit or a surviving path is an error, not a skip.

The `cleanup_script` ordering required by REQ-SSH-TASKDIR-RECLAMATION-007 is
satisfied by this position rather than by new sequencing code: the script runs
inside `SSHExecutor.StopInstance` during step 0's stop phase, which the job
completes and checks before reaching step 1. AC-SSH-TASKDIR-RECLAMATION-007.2
follows because the probe in step 5 reads the tree the script left behind.

### Ownership resolution

A target is owned by the task when both hold:

- The task's `TaskEnvironment.TaskID` is the job's task, and
  `hasActiveOtherTaskSessionsForEnvironment` reports no other live task on that
  environment. This is the existing signal that distinguishes a task from an
  `inherit_parent` subtask borrowing the parent's workspace, and it is already
  consulted for the local worktree path.
- No `executor_running` row belonging to another task carries the same
  `(host, port, user, remote_task_dir)`.

The second check is the one that catches the cross-host case the environment
check cannot see, and it is a direct analogue of
`CountActiveWorktreeReferences`. Either check erroring resolves to *not owned*
per AC-SSH-TASKDIR-RECLAMATION-003.3.

### Safety probe

Run against the task-directory root and every direct child directory that
contains a `.git` entry, matching the remote layout the v1 spec defines. Each
checkout must satisfy all four, and every probe is read-only
(AC-SSH-TASKDIR-RECLAMATION-002.6):

| Probe | Command | Safe when |
| --- | --- | --- |
| Is a checkout | `git rev-parse --is-inside-work-tree` | prints `true` |
| Clean tree | `git status --porcelain --untracked-files=all --ignored` | empty output after Kandev-owned `.kandev/` runtime entries are excluded |
| Everything pushed | `git rev-list --count HEAD --branches --not --remotes` | prints `0` |
| No stashes | `git stash list` | empty output |

The third probe is the load-bearing one and the reason a clean `git status` is
not sufficient. It counts commits reachable from `HEAD` or any local branch that
are reachable from no remote-tracking ref — which is exactly "work that would be
lost". Including `HEAD` explicitly covers a detached head, which `--branches`
alone misses.

Remote-tracking refs are a local cache and are not refreshed: the probe answers
"was this pushed at some point", which is the correct bar, and fetching during
cleanup would need network and credentials for a remote that may no longer exist.

The task-directory root not being a checkout is inconclusive, not safe: it means
preparation failed or the layout is unrecognized, and the directory may hold
arbitrary user content. Same for a probe that fails, times out, or returns
unparseable output. All resolve to skip
(AC-SSH-TASKDIR-RECLAMATION-002.5).

Probes run under a bounded per-target timeout derived from the job's context.

## Failure and recovery

The distinction that carries REQ-SSH-TASKDIR-RECLAMATION-005 is **skip versus
error**, and the two are deliberately not the same outcome:

- A **skip** — reclamation disabled, directory shared, work at risk, verdict
  inconclusive — is a correct result. It appends no error, the job completes
  `succeeded`, and the reason is logged and recorded. Retrying would not change
  the verdict, and a permanently-failing job for a directory the user is keeping
  on purpose is noise (AC-SSH-TASKDIR-RECLAMATION-002.7).
- An **error** — unreachable host, auth failure, fingerprint mismatch, non-zero
  `rm`, directory still present afterwards — appends to the job's `errs` slice
  exactly as `cleanup worktrees` and `cleanup quick-chat dir` already do. The
  existing `retryTaskResourceCleanupJob` then applies the 1m/5m/15m/1h/3h/6h/12h
  backoff, writes `last_error`, and marks the row `failed` after
  `taskResourceCleanupMaxAttempts`.

Because the job re-decodes its snapshot and re-runs the phase on every attempt,
AC-SSH-TASKDIR-RECLAMATION-005.3 holds without extra work: no verdict is cached
across attempts.

Reclamation runs after `performTaskCleanup`, so a failure cannot prevent local
row, worktree, and quick-chat cleanup from completing
(AC-SSH-TASKDIR-RECLAMATION-005.4).

`cancelIfTaskUnarchived` already runs before and within the job body and cancels
the whole job when an archive is undone, giving
AC-SSH-TASKDIR-RECLAMATION-004.3.

Backend shutdown cancels the cleanup worker's context mid-job. The phase refuses
to begin on a cancelled context, so an irreversible removal is never started
against a process that is going away; the durable job row survives and the next
attempt runs the phase with a live context. That check lives inside the phase
rather than at its call site so a later reordering of the job body cannot
quietly remove it.

## Persistence

No new tables and no migration. The reclamation targets live inside the existing
`task_resource_cleanup_jobs.resource_snapshot` JSON blob, which is already the
mechanism by which the job survives a backend restart. The new snapshot field is
additive and absent-tolerant: a job row written by an older backend decodes with
an empty target list and performs no reclamation.

The per-profile setting persists in the executor profile's existing `Config` map;
no schema change.

## Security

The removal is the most destructive operation Kandev performs on a machine it
does not own, so the design keeps three independent barriers between a bug and a
user's disk: the profile-authoritative opt-in, the ownership resolution, and the
path-shape guard. Each is sufficient on its own to prevent removal.

Connections reuse the pinned `host_fingerprint` from the snapshot with the same
no-silent-re-pin rule as every other SSH path; a changed host key fails the
attempt rather than proceeding against an unknown machine. The remote path is
shell-quoted through the existing `shellQuote` helper and validated in Go before
it is quoted, so a task-dir name can neither escape its parent nor inject shell.

## Observability

Structured logs at the reclamation phase carry `task_id`, `host`, `remote_task_dir`,
and an `outcome` of `removed`, `skipped`, or `failed`, with a `reason` on the
latter two. `skipped` reasons are enumerated (`disabled`, `shared`,
`dirty_worktree`, `unpushed_commits`, `stash_present`, `not_a_checkout`,
`probe_failed`) so an operator can tell a deliberate preservation from a broken
probe.

The existing `task_resource_cleanup_jobs` row remains the durable record of a
failure, via its `state`, `attempts`, and `last_error` columns. The resource
snapshot in that row also records each target outcome and safety-skip reason,
so a successful preservation is discoverable after restart.

## Testing

Every test for the removal path uses a fake remote-command seam or a temporary
directory. No test executes a real deletion against a real workspace root. The
`SSHTaskDirReclaimer` takes its command execution through an interface so unit
tests never open an SSH connection, and the probe-verdict table is exercised
purely against recorded command outputs.

Regression coverage must include the directory being removed on terminal archive
and delete, and preserved on ordinary stop and on backend shutdown.

## Related decisions

A new ADR records the opt-in default and the placement of the removal in the
cleanup job rather than in `StopInstance`.
