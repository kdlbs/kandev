# ADR-2026-08-22-plugin-version-retention: Keep Exactly One Superseded Plugin Version

**Status:** accepted
**Date:** 2026-08-22
**Area:** backend

## Context

Every plugin install extracts a full package into `~/.kandev/plugins/<id>/<version>/`, and no
install path ever removed the version it replaced. Keeping the predecessor during the upgrade is
deliberate: `Service.Install` stops the running plugin before swapping, and the old directory is
what `rollbackFailedInstall` restarts when the new version fails. Nothing, however, removed it
afterwards.

All four install routes (marketplace/URL, auto-update poller, sideload upload, dropped tarball)
converge on that same code, and the auto-update poller installs with no user action at all, so the
growth is unattended. A version directory is roughly 20MB, or 95MB for a package shipping all five
platform binaries. On one developer machine the plugins directory had reached 4.2GB, of which
3.74GB was 58 superseded version directories, 46 of them belonging to a single auto-updating
plugin.

## Decision

A plugin converges on two extracted versions: the active one and the version it replaced. The
retained count is fixed in code, not configurable. It is a convergent state rather than an enforced
cap, because every precondition below can legitimately leave more versions on disk.

Deletion is gated on a **confirmed running process**, never on the install alone. `Install` prunes
after `activate` returns successfully and boot (`StartActivePlugins`) prunes after `runtime.Start`
succeeds, but in both cases `pruneSupersededVersions` itself re-checks `runtime.Running(id)` before
deleting anything — `activate` returns nil without starting a process when no runtime is wired, so
"activate succeeded" is not the same guarantee. A failed install, a failed spawn, a disabled plugin,
a sideload registered disabled, and a plugin in `error` therefore all keep every version they have,
as does a plugin whose deletion failed. `rollbackFailedInstall` is unchanged, and remains the one
path that deletes a version directory without a prune: it removes the newly extracted version, and
only that one, when the record cannot be persisted.

Extraction is serialized and tracked. `Install` cannot take a plugin's lifecycle lock until it has
parsed the manifest, so two overlapping installs of the same id both extract before either is
serialized; the extraction and its in-flight registration happen under one mutex, and a prune skips
any directory still marked in flight. Without that, the first install to acquire the lock would
delete the second's fresh package and strand it activating a path that no longer exists.

A directory is a deletion candidate only if it is a real directory (never a symlink), is not
`data/` or any dot-prefixed name (which covers pkgtar's `.tmp-*` staging directories), and contains
a `manifest.yaml` declaring exactly that plugin id and that directory name as its version. Listing,
manifest reads, and removal all go through a single `os.Root` handle on the plugin directory, so a
path validated by the scan is removed inside the same confined tree and the `<id>.yml` /
`<id>.config.yml` records one level up are unreachable.

Pruning is best-effort: every failure is logged and swallowed, because a plugin that installed and
started correctly must not be reported as failed because cleanup could not delete a directory.

## Consequences

- Disk use per plugin becomes bounded by two versions instead of growing with every install.
- Rollback keeps working: the version a failed upgrade restarts is always still on disk.
- Boot-time pruning lets an instance that already accumulated versions recover on its next restart,
  including plugins that are already at their latest version and would never trigger an install.
- The prune deletes durable on-disk state, so its preconditions are the safety contract and any
  change to them needs regression coverage (`service_prune_test.go`).
- Rolling back further than one version now requires reinstalling the wanted package.

## Alternatives Considered

- **Make the retained count configurable.** Rejected as speculative. The second directory exists to
  serve a specific mechanism (rollback of a failed upgrade), which is a safety invariant rather than
  an operator preference, and any larger configured value re-creates the unbounded growth this
  exists to stop. A knob would also need UI, docs, and validation for no demonstrated need.
- **Keep only the active version.** Rejected because it deletes the rollback target
  `rollbackFailedInstall` depends on, converting a failed upgrade from recoverable into an outage.
- **Prune immediately after extraction, before the new version starts.** Rejected for the same
  reason: it removes the fallback exactly when it is most likely to be needed.
- **Age-based retention (delete versions older than N days).** Rejected because it neither bounds
  disk use for a frequently-updating plugin nor guarantees a rollback target exists for a rarely
  updated one.
- **Install-time pruning only.** Rejected because the instances that motivated this fix are already
  full of superseded versions, and a plugin at its latest version, or one with auto-update off,
  would never install again and never recover.
- **A separate operator-triggered cleanup action.** Rejected as a worse default: the growth is
  unattended, so the recovery should be too, and an operator who has not noticed 3.74GB of stale
  packages will not press a button about them.
