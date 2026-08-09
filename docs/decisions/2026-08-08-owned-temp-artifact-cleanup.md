# ADR-2026-08-08-owned-temp-artifact-cleanup: Clean only registered Kandev temporary artifacts

**Status:** accepted
**Date:** 2026-08-08
**Area:** backend, frontend, infra, security

## Context

Kandev host-local agents intentionally inherit the service's `TMPDIR`, `TMP`, and `TEMP`, so the
operating-system temporary directory can contain both Kandev scratch and unrelated user, tool, CI,
preview, or other-installation data. A name such as `kandev-*`, an old modification time, or a live
path prefix does not prove that a directory belongs to the current Kandev installation.

The Storage page should still give operators a safe way to reclaim abandoned temporary directories
created by Kandev service components. This requires a narrower ownership boundary than a general
`/tmp` sweep.

## Decision

Add a typed `temporary_artifacts` storage provider with these rules:

- The provider acts only on artifact records created by the current Kandev storage registry. Each
  record stores the exact absolute path, artifact kind, opaque artifact ID, lifecycle timestamps,
  owner lease/heartbeat state, and a marker filename inside the artifact root. The marker and record
  must match before the path is considered.
- The registry accepts only compiled-in service artifact kinds and creates top-level directories
  beneath the effective service temporary root. It rejects symlinked roots, nested paths, and
  arbitrary caller-supplied prefixes. The first producers are long-lived backend service roots such
  as Improve Kandev bundles and the host-utility parent; standalone CLI, preview, CI, and test
  harness roots are not implicitly registered.
- An active lease or recent heartbeat protects an artifact. A closed or abandoned artifact becomes
  eligible only after a fixed 24-hour stale interval. Unreadable records, missing or mismatched
  markers, path replacement, uncertain liveness, and filesystem errors are skipped fail-closed.
- The provider is manual-only in its first release. `POST /storage/run` with the explicit
  `resources: ["temporary_artifacts"]` selection invokes it; scheduled maintenance and an
  unscoped full manual run do not delete these artifacts. No new persisted policy toggle is added.
- Eligible roots are moved by same-filesystem rename into the existing Kandev quarantine before
  permanent deletion. Cross-device moves have no copy/delete fallback. Existing quarantine
  retention, restore, and permanent-delete safety rules apply.
- Legacy `/tmp/kandev-agent/*`, generic `tmp.*` paths, dependency and package caches, Go/Node/
  Playwright caches, PR/preview folders, E2E/dev-isolated roots, and artifacts belonging to another
  Kandev installation remain outside the provider.

This supplements ADR 0045: inherited shared temporary data remains unowned by default, while a
durable registry record plus matching marker creates a deliberately narrow, typed exception for
specific service-created roots.

## Consequences

- Operators can reclaim known abandoned service scratch from the Storage page without granting
  Kandev authority to recursively delete shared `/tmp` contents.
- The first release may report zero eligible artifacts even when `/tmp` is large; unregistered
  folders require their owning producer or host temporary-file policy to clean them.
- Producers must keep registry leases and markers correct, and new long-lived service temp roots
  need an explicit registration decision before they become reclaimable.
- Quarantine can temporarily move bytes from `/tmp` into Kandev's trash, but preserves restore and
  fail-closed behavior rather than deleting a possibly misclassified root immediately.
- The fixed stale interval is intentionally conservative. Scheduled cleanup or a configurable
  temp policy requires a later product and operational decision.

## Alternatives Considered

- **Delete `/tmp/kandev-*` by name and age.** Rejected because names do not identify the current
  installation and the same temporary root is shared with unrelated work.
- **Scan every file below `TMPDIR` and infer ownership from mtime or process names.** Rejected
  because it is a general-purpose sweeper, is vulnerable to path replacement, and cannot prove
  task or installation ownership.
- **Move all agent scratch beneath `KANDEV_HOME_DIR`.** Rejected by ADR 0045 because it mixes
  transient process data with persistent application state and changes cache-sharing behavior.
- **Use only an on-disk marker without a durable registry record.** Rejected because a stale or
  copied marker is not enough to establish lifecycle state after restart; the provider needs an
  exact path and lease record owned by the current installation.
- **Enable the provider for scheduled and unscoped manual runs immediately.** Rejected because
  shared-temp cleanup needs an explicit operator action and more operational evidence first.
