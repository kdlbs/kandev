# 0045: Install-wide storage maintenance uses typed ownership providers and quarantine

**Status:** accepted (amended 2026-07-19)
**Date:** 2026-07-14
**Area:** backend, frontend, infra

## Context

Kandev task churn can leave large task directories, Go build artifacts, and Docker resources on
self-hosted machines. The existing `office/infra` garbage collector is an unconditional periodic
loop focused on worktree directories, while task archive/delete cleanup runs in a detached,
time-bounded goroutine. Neither gives operators an install-wide policy or durable retry surface,
and broad host commands such as `docker system prune -a` can delete resources Kandev does not own.

Task unarchive support added by PR #1687 also makes cleanup history significant: archived local
worktrees are disposable, but historical worktree records and branch names are needed to recover a
pushed branch later. Cleanup must distinguish filesystem payload from recovery metadata.

## Decision

Kandev will own one install-wide storage-maintenance service under `internal/system/storage`.
It replaces the periodic `office/infra` GC loop and is available whether Office mode is enabled or
not. Scheduled destructive maintenance is persisted in the install-wide `settings` store and is
disabled by default; durable cleanup requested directly by task archive/delete remains active
regardless of that preference.

Maintenance is composed from typed providers for task workspaces, the Kandev-managed Go cache,
Kandev-labeled containers, Docker build cache, and Docker images. It never executes an arbitrary
configured command. Each provider declares whether its resources are Kandev-owned or host-global.
Host-global Docker providers require a persisted dedicated-daemon acknowledgment.

All destructive providers share an activity gate with execution launch and shell/build operations.
The scheduler runs only after a quiet period, rechecks activity after acquiring the gate, and is
cancelled when a new task launch needs the gate.

Filesystem candidates are classified from a complete authoritative inventory and fail closed under
ADR 0009. Task workspaces and owned Go caches are atomically quarantined beneath
`<KANDEV_HOME_DIR>/trash` before permanent deletion. Quarantine and cleanup intent are durable, so a
backend restart can reconcile an interrupted rename or retry failed task lifecycle cleanup.

Archive/delete paths persist a cleanup job and resource snapshot before mutating the task row.
Archive jobs re-check that the task remains archived before destructive steps. Unarchive cancels a
pending archive job and restores a matching quarantined workspace when possible. Storage cleanup
does not delete historical archived-task worktree rows or branch metadata used by PR #1687's branch
recovery.

Agentctl session temporary roots are a separate lifecycle-owned resource. They remain beneath the
operating system's temporary directory under a readable, collision-resistant digest of the raw
session and instance identity so concurrent sessions have isolated sockets, compiler work files,
and command scratch space without mixing ephemeral data into `KANDEV_HOME_DIR`. After agent, shell,
VS Code, and workspace subprocesses are reaped during permanent instance teardown, agentctl deletes
only that instance's validated owned root. A later session resume creates a replacement instance
and does not depend on the prior instance's scratch. Session temp data is not quarantined because it
is explicitly ephemeral and contains no task recovery state.

Terminal instance teardown closes process admission before its ownership sweep. A failed HTTP
shutdown or process-tree reap retains a stopping instance tombstone and its allocated port so a
replacement cannot adopt unresolved runtime resources. A temporary-directory-only deletion failure
is reported but does not retain the execution port. Ordinary agent stop/start remains restartable;
the irreversible admission close belongs to instance teardown.

## Consequences

- Operators can configure and inspect cleanup from Kandev without host cron or systemd overrides.
- A systemd-managed Kandev process does not gain implicit authority to delete host resources;
  scheduling and host-global Docker actions remain explicit opt-ins.
- Detached cleanup failures become durable and retryable rather than silent disk leaks.
- Quarantine adds disk usage during the retention window, but turns filesystem classification
  mistakes and unarchive races into recoverable events.
- Execution launch and maintenance need a shared cancellation-aware activity gate.
- Docker build-cache and image cleanup remain conservative on shared daemons, even if that leaves
  reclaimable bytes for the operator.
- The existing Office GC package and startup wiring are removed or reduced to adapters over the
  System storage service to avoid two competing sweepers.
- Session teardown reclaims its temporary storage immediately and independently of scheduled
  maintenance, while path containment prevents removal of the shared temp root or sibling sessions.

## Alternatives Considered

- **Run `go clean` and `docker system prune -a` from a generic scheduled-command feature.** Rejected
  because it grants the daemon arbitrary command execution and cannot encode ownership or
  fail-closed behavior.
- **Keep the Office GC and add separate cache timers.** Rejected because independent sweepers would
  race, expose inconsistent settings, and leave non-Office installations without the same controls.
- **Delete orphan workspaces immediately after the grace period.** Rejected because uncommitted task
  files and classification errors are more valuable than the temporary space saved by skipping
  quarantine.
- **Treat every historical worktree row as a live filesystem reference.** Rejected because archive
  intentionally removes the local worktree; the row is recovery metadata and would otherwise keep
  failed-cleanup directories forever.
- **Make systemd installation enable cleanup automatically.** Rejected because process supervision
  does not imply user consent for destructive host maintenance.
- **Store session scratch data under `KANDEV_HOME_DIR`.** Rejected because it mixes transient
  process data with persistent application state and makes backups, retention, and disk accounting
  retain data that should disappear at teardown or reboot.
- **Share one temporary directory across agent sessions.** Rejected because filenames, sockets, and
  compiler work directories can collide and teardown cannot safely determine which files it owns.
