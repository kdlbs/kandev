---
status: active
system: executors
created: 2026-08-24
owners:
  - kandev
---

# Remote Task-Directory Reclamation Requirements

## Overview

When a task that ran on an SSH executor is archived or deleted, Kandev stops the
remote `agentctl` process and removes the per-session runtime directory, but
leaves the remote git checkout at `<workdir_root>/tasks/<task-dir-name>/` on the
host forever. That was the deliberate v1 boundary; the v1 specification records
it under "Out of scope" and `docs/public/feature-status.md` tells users that
task directories remain for manual housekeeping.

The consequence is an unbounded disk leak on a machine Kandev does not own. A
host observed on 2026-08-24 held 1.2 GB across six task directories, five of
which belonged to tasks that no longer existed in Kandev. Every corresponding
`task_resource_cleanup_jobs` row read `state=succeeded` — honestly, because the
directory removal was never attempted.

This capability reclaims those directories. The difficulty is not the removal;
it is proving the removal is safe. Deleting a directory on a machine Kandev does
not own is irreversible, and the failure mode is silent loss of a user's work.
The safety constraints below are therefore the substance of the capability, not
qualifications on it.

## Terminology

- **Task directory:** the remote directory `<workdir_root>/tasks/<task-dir-name>/`
  that Kandev creates for one task. Its root is the primary repository checkout;
  additional repositories and branches are direct child directories.
- **Reclamation:** removing a task directory and everything beneath it from the
  remote host.
- **Terminal outcome:** a task or session archive, delete, or cascade variant.
  These are the outcomes that produce a durable task-resource cleanup job.
- **Resumable stop:** any other stop, including an ordinary user stop, a
  rollback, a failed-agent stop, a stale-replacement stop, and backend shutdown.
- **At-risk content:** anything inside a task directory that a user could not
  recover from a remote after the directory is gone. Uncommitted changes,
  untracked files, stash entries, and commits not contained by any
  remote-tracking ref are all at risk.
- **Owning task:** the single task whose lifecycle governs a task directory. A
  subtask launched with `workspace_mode: inherit_parent` shares its parent's
  directory and does not own it.

## Requirements

### REQ-SSH-TASKDIR-RECLAMATION-001: Reclaim remote task directories on terminal outcomes

**Intent:** Stop the unbounded disk leak on user-owned remote hosts by removing
the task directory once its owning task has reached a terminal outcome and every
safety condition below is satisfied.

**User story:** As an operator running Kandev tasks on my own server, I want
Kandev to clean up the checkouts it created once I archive or delete the task,
so that my disk does not fill with work I have finished with.

#### Acceptance criteria

- **AC-SSH-TASKDIR-RECLAMATION-001.1:** When a task that ran on an SSH executor
  reaches a terminal outcome, reclamation is enabled for the executor profile
  that ran it, and every condition in REQ-SSH-TASKDIR-RECLAMATION-002 and
  REQ-SSH-TASKDIR-RECLAMATION-003 is satisfied, the system shall remove the
  remote task directory and everything beneath it.
- **AC-SSH-TASKDIR-RECLAMATION-001.2:** When reclamation removes a task
  directory, the system shall leave `<workdir_root>/tasks/` itself, sibling task
  directories, the shared remote `agentctl` binary, and its content-hash sidecar
  untouched.
- **AC-SSH-TASKDIR-RECLAMATION-001.3:** When a task reached a terminal outcome
  before this capability shipped and its directory is still on the host, the
  system shall not remove it; reclamation acts only on outcomes it observes, and
  no retroactive sweep of pre-existing directories is performed.
- **AC-SSH-TASKDIR-RECLAMATION-001.4:** When reclamation removes the directory of
  an archived task that is later unarchived, the system shall re-materialize the
  checkout through the normal preparation path on the next launch rather than
  failing the launch.

### REQ-SSH-TASKDIR-RECLAMATION-002: Never destroy work that cannot be recovered

**Intent:** Guarantee that reclamation can only remove a directory whose contents
are positively established to be recoverable. A clean working tree is not
evidence: a tree can be clean and still hold commits that were never pushed. An
inconclusive check must be treated exactly like a positive finding of risk.

**User story:** As a developer whose remote checkout holds commits I have not
pushed, I want Kandev to refuse to delete that checkout and tell me why, so that
housekeeping can never cost me work.

#### Acceptance criteria

- **AC-SSH-TASKDIR-RECLAMATION-002.1:** Before removing a task directory, the
  system shall inspect every git checkout it contains and shall remove the
  directory only when each checkout positively demonstrates that it holds no
  at-risk content.
- **AC-SSH-TASKDIR-RECLAMATION-002.2:** When a checkout has uncommitted
  modifications, staged changes, untracked files, or ignored files outside the
  Kandev-owned `.kandev/` runtime path, the system shall skip reclamation for
  that task directory and report the reason.
- **AC-SSH-TASKDIR-RECLAMATION-002.3:** When a checkout holds a commit reachable
  from any local branch or from `HEAD` that is not contained by any
  remote-tracking ref, the system shall skip reclamation for that task directory
  and report the reason. Absence of local modifications shall never on its own
  satisfy AC-SSH-TASKDIR-RECLAMATION-002.1.
- **AC-SSH-TASKDIR-RECLAMATION-002.4:** When a checkout holds stash entries, the
  system shall skip reclamation for that task directory and report the reason.
- **AC-SSH-TASKDIR-RECLAMATION-002.5:** When any safety probe cannot be completed
  — the command fails, times out, returns output the system cannot parse, `git`
  is unavailable on the host, or the task directory root is not a git checkout —
  the system shall treat the directory as at risk, skip reclamation, and report
  the reason. An unanswerable probe shall never be resolved in favour of removal.
- **AC-SSH-TASKDIR-RECLAMATION-002.6:** The safety probes shall be read-only with
  respect to the remote repository. The system shall not fetch, push, prune,
  garbage-collect, reset, or otherwise mutate a checkout in order to establish
  that it is safe to remove.
- **AC-SSH-TASKDIR-RECLAMATION-002.7:** When reclamation is skipped for a safety
  reason, the system shall record the skip and its reason durably enough for a
  user to discover which directories were preserved and why, and shall report the
  task-resource cleanup job as succeeded rather than failed, because a preserved
  directory is a correct outcome and not an error to retry.

### REQ-SSH-TASKDIR-RECLAMATION-003: Remove only task-owned directories

**Intent:** A task directory can be shared. A subtask launched with
`workspace_mode: inherit_parent` runs inside its parent's directory. Reclaiming a
directory another live task is still using is data loss dressed as housekeeping.

**User story:** As a user running subtasks inside a parent task's workspace, I
want deleting one subtask to leave the shared workspace intact, so that the
sibling tasks still using it keep working.

#### Acceptance criteria

- **AC-SSH-TASKDIR-RECLAMATION-003.1:** Before removing a task directory, the
  system shall establish that the task reaching the terminal outcome is the
  owning task for that directory on that host.
- **AC-SSH-TASKDIR-RECLAMATION-003.2:** When any other task that has not itself
  reached a terminal outcome maps to the same host and the same task directory,
  the system shall skip reclamation and report the directory as shared.
- **AC-SSH-TASKDIR-RECLAMATION-003.3:** When ownership cannot be established, the
  system shall skip reclamation and report the reason, on the same
  inconclusive-means-preserve rule as AC-SSH-TASKDIR-RECLAMATION-002.5.
- **AC-SSH-TASKDIR-RECLAMATION-003.4:** The system shall refuse to reclaim any
  path that is not exactly one non-empty path segment beneath the profile's
  `<workdir_root>/tasks/`, and shall refuse a segment containing a path
  separator or a relative path component.

### REQ-SSH-TASKDIR-RECLAMATION-004: Terminal outcomes only

**Intent:** Resuming into an existing remote checkout is a core part of the SSH
executor's value. Reclamation must be reachable only from outcomes that end a
task, and must be unreachable from every stop a user or the backend can perform
while intending to come back.

**User story:** As a user who stops a long-running remote task overnight, I want
my remote checkout to be exactly where I left it in the morning, so that
resuming does not re-clone or lose local state.

#### Acceptance criteria

- **AC-SSH-TASKDIR-RECLAMATION-004.1:** When a session stops for any reason other
  than a terminal outcome, including an ordinary user stop, a rollback, a
  force stop, a failed-agent stop, and a stale-replacement stop, the system shall
  leave the remote task directory in place.
- **AC-SSH-TASKDIR-RECLAMATION-004.2:** When the backend shuts down, the system
  shall leave every remote task directory in place, and shall not perform
  reclamation as part of shutdown.
- **AC-SSH-TASKDIR-RECLAMATION-004.3:** When a task archive is undone before its
  cleanup job runs, the system shall cancel the reclamation together with the
  rest of that job's cleanup and leave the directory in place.

### REQ-SSH-TASKDIR-RECLAMATION-005: A failed reclamation is a real error

**Intent:** A remote removal that did not happen must not be reported as one that
did. The existing durable cleanup job already retries and surfaces failures; a
swallowed reclamation error would reproduce a known false-success defect shape on
the remote path, and would leave operators believing disk was reclaimed when it
was not.

**User story:** As an operator, I want a remote host that was unreachable during
cleanup to show up as a retrying and eventually failing cleanup job, so that I
learn the directory is still there instead of being told it was removed.

#### Acceptance criteria

- **AC-SSH-TASKDIR-RECLAMATION-005.1:** When reclamation is attempted and fails —
  the host is unreachable, authentication fails, the pinned host fingerprint no
  longer matches, the removal command exits non-zero, or the directory still
  exists after the removal — the system shall propagate the failure to the
  owning task-resource cleanup job.
- **AC-SSH-TASKDIR-RECLAMATION-005.2:** When reclamation fails, the system shall
  not mark the cleanup job succeeded; the job shall retry under its existing
  backoff schedule and record the failure reason in its `last_error` field.
- **AC-SSH-TASKDIR-RECLAMATION-005.3:** When reclamation is retried, the system
  shall re-run every safety check from REQ-SSH-TASKDIR-RECLAMATION-002 and
  REQ-SSH-TASKDIR-RECLAMATION-003 against the host's current state rather than
  reusing an earlier verdict.
- **AC-SSH-TASKDIR-RECLAMATION-005.4:** When reclamation fails or is skipped, the
  system shall still complete the rest of the task's resource cleanup, and shall
  not leave local rows or worktrees uncleaned because a remote host was
  unavailable.

### REQ-SSH-TASKDIR-RECLAMATION-006: Opt-in per executor profile

**Intent:** Existing installations hold remote directories that were created
under a documented promise that Kandev would not delete them. Turning
reclamation on during an upgrade would retroactively break that promise against
data on a machine Kandev does not own, with no undo. The user must choose it, and
must choose it for a specific host and root.

**User story:** As an operator, I want to decide per SSH profile whether Kandev
may delete task directories under that profile's workspace root, so that
upgrading Kandev never silently changes what happens to my disk.

#### Acceptance criteria

- **AC-SSH-TASKDIR-RECLAMATION-006.1:** The system shall expose reclamation as a
  per-executor-profile setting that defaults to disabled, including for every
  profile that exists at upgrade time.
- **AC-SSH-TASKDIR-RECLAMATION-006.2:** When reclamation is disabled for the
  profile that ran the task, the system shall leave the remote task directory in
  place and shall behave exactly as it did before this capability existed.
- **AC-SSH-TASKDIR-RECLAMATION-006.3:** The executor profile shall be the sole
  source of truth for the setting; a value supplied through task metadata shall
  never enable reclamation.
- **AC-SSH-TASKDIR-RECLAMATION-006.4:** The setting shall be presented to the
  user with its blast radius stated: which host and which workspace root it
  applies to, and that removal is permanent.

### REQ-SSH-TASKDIR-RECLAMATION-007: Run the profile cleanup hook before removal

**Intent:** A profile `cleanup_script` exists to act on the checkout — collecting
artifacts, tearing down services, pushing a branch. It is worthless if it runs
after the checkout is gone, and destructive if reclamation races it.

**User story:** As a user whose cleanup script archives build output from the
remote checkout, I want it to run and finish against a populated directory before
Kandev removes it, so that my script still does its job.

#### Acceptance criteria

- **AC-SSH-TASKDIR-RECLAMATION-007.1:** When a profile defines a
  `cleanup_script` and reclamation is enabled, the system shall run the cleanup
  script against the intact task directory and shall wait for it to finish before
  evaluating any reclamation safety check.
- **AC-SSH-TASKDIR-RECLAMATION-007.2:** When the cleanup script leaves at-risk
  content in the checkout, the safety checks in
  REQ-SSH-TASKDIR-RECLAMATION-002 shall observe that content and skip
  reclamation.

## Out of scope

- **A background sweeper for pre-existing orphans.** This capability acts on
  terminal outcomes Kandev observes. Directories left by tasks that were deleted
  before it shipped, or created by an install that no longer exists, are not
  swept. A scheduled sweep is a separate capability with a different and larger
  safety problem: it must decide that a directory is an orphan without a task
  row to reason from.
- **Fetching from a remote to improve the safety verdict.** Probes are read-only
  and local to the host; a branch that was never pushed stays at risk rather than
  being pushed or fetched during cleanup.
- **Reclaiming the shared `agentctl` binary, its cache sidecar, or
  `<workdir_root>` itself.**
- **A user-facing browser of preserved directories.** Skips are reported; a
  management UI for them is not part of this capability.
- **Reclamation for executors other than SSH.** Docker and Sprites own their own
  teardown.
