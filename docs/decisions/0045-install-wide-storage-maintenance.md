# 0045: Install-wide storage maintenance uses typed ownership providers and quarantine

**Status:** accepted (amended 2026-07-22)
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

Host-local agent instances inherit the Kandev service's `TMPDIR`, `TMP`, and `TEMP` environment
unchanged. Agentctl does not replace those values with a per-instance directory and does not claim
ownership of files written to the shared operating-system temporary location. An operator-level
temporary-directory override remains authoritative for the whole service; with no override, normal
platform defaults apply.

This boundary is intentionally separate from persistent tool-cache ownership. Go's default
`GOCACHE`, for example, is already a shared user cache and is not derived from `TMPDIR`; Kandev only
injects its managed cache path when that existing Storage option is explicitly enabled. Tools that
derive reusable caches from the system temp directory may now share them across agents instead of
duplicating them beneath per-instance roots.

Terminal instance teardown still closes process admission and reaps the owned process tree, but it
does not sweep shared temp contents. Kandev-specific sockets, locks, or scratch paths that require
isolation must carry an explicit collision-resistant name at their own boundary. A demonstrated
tool-specific collision may justify a narrow override for that tool, not a replacement of the
temporary environment inherited by every child process.

Directories under `/tmp/kandev-agent/*` created by older versions are legacy host data. New agent
instances no longer add to that root. Storage maintenance does not delete legacy entries by name or
age because the existing directories lack durable ownership evidence; host temporary-file policy or
a deliberate stopped-service cleanup remains the authority for that historical footprint.

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
- Node, Playwright, and similar temp-derived caches may be reused across agent instances instead of
  being duplicated under a task-scoped temp root.
- Transient compiler work, test databases, plugin fixtures, sockets, and other scratch still exist;
  they move to the inherited shared temp location and are governed by tool cleanup plus host policy.
- Kandev loses per-task attribution and exact recursive deletion for arbitrary temp contents. This
  is an accepted tradeoff because operators prefer cache sharing and normal host temp lifecycle.
- Existing `/tmp/kandev-agent` usage is not reclaimed automatically by this change.

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
- **Give every agent instance its own `TMPDIR`, `TMP`, and `TEMP`.** Rejected after operational
  evidence showed duplicated Node/Playwright caches, explicitly redirected Go caches, and abandoned
  E2E/compiler scratch consuming many GiB beneath long-lived agent roots. It improves attribution
  and defensive isolation, but prevents useful cache sharing and makes interrupted task roots a
  concentrated storage leak.
- **Keep per-instance temp roots but mount shared subdirectories for selected caches.** Rejected for
  now because cache discovery is tool-specific and creates a growing allowlist. Persistent caches
  such as `GOCACHE` retain their dedicated policy; observed collisions can receive narrow fixes.
- **Delete old `/tmp/kandev-agent/*` entries by name or age.** Rejected because neither proves task
  archival, installation ownership, process liveness, or protection from path replacement.
