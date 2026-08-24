# ADR-2026-08-24-opt-in-ssh-task-directory-reclamation: Opt-in reclamation of remote SSH task directories

**Status:** accepted
**Date:** 2026-08-24
**Area:** backend, frontend, operations

## Context

An SSH executor materializes each task at `<workdir_root>/tasks/<task-dir-name>/` on the remote
host. Until now nothing ever removed that directory: `SSHExecutor.StopInstance` deliberately left
it in place on every stop reason, and no other code path deleted it. That was documented as
intended in the SSH executor specification and in the public feature-status entry, so operators who
adopted the executor did so under an explicit keep-forever promise. The cost is unbounded growth on
a machine Kandev does not own; a measured host held six directories totalling 1.2 GB, five of them
belonging to tasks that no longer existed.

Two questions had to be settled before removal could ship. First, whether reclamation should be
default-on. Second, where the removal belongs, given that Kandev already has a durable
task-resource cleanup job and also has a per-session stop path that runs the profile `cleanup_script`
over the same connection.

## Decision

**Reclamation is opt-in per executor profile and defaults off.** The profile carries
`ssh_reclaim_task_dir`, projected into the launch-time `executors_running` metadata and read from
that snapshot at cleanup time rather than re-read from the profile row. An upgrade therefore
changes no existing host's behavior, and a toggle flipped on after a launch does not retroactively
arm deletion of a directory created under the old promise. The settings surface names the host and
the resolved `<workdir_root>/tasks/` path, and states that removal is permanent and that
unarchiving re-clones.

**The removal is a phase of the durable task-resource cleanup job, not part of `StopInstance`.**
The phase runs after the stop phase and after `performTaskCleanup`, only for a job whose trigger is
archive, delete, or cascade delete. It refuses to begin on an already-cancelled context, resolves
ownership before touching anything, probes the checkout for uncommitted changes, unpushed commits,
and stashes, and treats a failed remote removal as a real error that feeds the job's existing
backoff, `last_error`, and `failed` handling. A safety skip is not an error: the job completes
`succeeded` with the reason recorded.

`SSHExecutor.StopInstance` keeps its contract of never removing the task directory, which is what
makes preservation on an ordinary stop and on backend shutdown structural rather than conditional.
The built-in reclamation phase is separate from the profile `cleanup_script` hook: the hook may run
during terminal stop, while reclamation runs later in the durable task-resource cleanup job.

## Consequences

An operator who wants the disk back turns reclamation on per profile and gets it on the next
terminal archive or delete. An operator who does nothing keeps every directory, exactly as before,
and the public documentation says so. Directories belonging to tasks that reached a terminal outcome
before this shipped are never swept, because the phase reads the cleanup job's snapshot rather than
listing the remote filesystem.

Because ownership resolution fails closed, a task whose environment is owned by another task (the
`inherit_parent` subtask case), whose environment still has an active borrower, or whose remote
directory is claimed by another task's `executors_running` row, records `shared` and keeps the
directory. Any error in that resolution also resolves to not-owned. The conservative direction is a
directory that outlives its task, which is the pre-existing behavior, never a lost workspace.

## Alternatives Considered

Default-on reclamation with an opt-out was rejected: it silently breaks a documented promise on
upgrade, and the first evidence an operator would get is a deleted checkout.

Removal inside `SSHExecutor.StopInstance` was rejected for three reasons. The stop path is
per-session while the directory is per-task, so a multi-session task would attempt removal while
sibling sessions still held it. `stopTaskRuntimeTargets` swallows errors for terminal targets, so a
failed remote delete would be reported as success. And the stop path has no view of the task graph,
so it cannot answer the ownership question that decides whether removal is safe.

A background sweeper that lists `<workdir_root>/tasks/` and deletes directories with no matching
task row was rejected: it acts on a remote filesystem listing rather than on recorded task
ownership, so a directory Kandev did not create, or one created by a task in another Kandev
install sharing the host, is indistinguishable from an orphan.

Treating a clean `git status --porcelain --untracked-files=all --ignored` as sufficient evidence of safety was rejected. It reports
nothing about commits that exist only locally, and nothing about stashes. The probe checks all
three.
