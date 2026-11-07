# ADR-2026-11-05-sdk-release-evidence: Retain evidence on the SDK release

**Status:** accepted
**Date:** 2026-11-05
**Area:** infra, workflow

## Context

The `v0.99.0` backfill published its host release successfully. SDK preparation
then attempted to attach its tested archive to that immutable release. GitHub
rejected the upload with HTTP 422. The npm SDK job never ran. Application npm,
Homebrew, and Scoop publication succeeded independently.

SDK records permanently identify their original signed host commit. A later
host release cannot restamp an unchanged SDK or replace the original evidence.
Moving SDK preparation before future host publication alone cannot recover an
originating host release that is already immutable.

## Decision

Retain new SDK evidence on a dedicated GitHub Release for `pluginsdk/v<S>`.
Create it as a draft at the original source commit. Upload and verify the
archive, signed receipt, and detached signature before creating the signed SDK
tag. Then publish the SDK release with `make_latest=false`. The signing job
owns this lifecycle. The OIDC publishing job remains read-only on GitHub.

The receipt's signed host/source mapping and schema do not change. Reuse valid
legacy host-retained evidence through a read-only compatibility path. Conflicts
between evidence locations fail closed. Never overwrite published SDK evidence,
move a tag, delete a host release, or disable release immutability for recovery.

When host publication already completed, verify and reuse its immutable assets
instead of uploading another build. Required inventory and integrity failures
remain publication failures. SDK releases never become the application's
latest release or replace its updater feed.

## Consequences

The independently versioned SDK owns its evidence lifecycle. Existing immutable
host releases can recover missing SDK publication without changing signed
source. Public receipt and bootstrap URLs can refer to the SDK release.

Release tooling must discover authenticated drafts, reconcile uncertain writes,
finalize the SDK release before npm publication, and support legacy evidence.
An absent npm package still requires the explicit first-package bootstrap when
initial OIDC publication is unavailable. This decision does not add a CI npm
token fallback.

## Alternatives considered

- Attach SDK assets after host publication: incompatible with immutable hosts.
- Keep future host releases in draft until SDK assets are attached: avoids the
  future ordering error but cannot repair `v0.99.0` without another evidence home.
- Disable immutability or delete and recreate the host release: breaks existing
  distribution guarantees and is excluded from recovery.
- Retain only GitHub Actions artifacts: retention can expire, leaving published
  SDKs without their authenticated original archive.
- Restamp the SDK on a new host release: changes its established source mapping.
  It cannot repair the SDK record in the signed `v0.99.0` tag.

## Related artifacts

- [SDK requirements](../specs/plugins/requirements/sdk-distribution.md).
- [SDK design](../specs/plugins/system-design/sdk-distribution.md).
- [Host artifact design](../specs/release/system-design/compact-runtime-distribution.md).
- [Repair plan](../plans/immutable-release-sdk-recovery/plan.md).
- [GitHub CLI draft upload and immutability behavior](https://cli.github.com/manual/gh_release_create).
- [First-hand draft tag API observation](https://github.com/orgs/community/discussions/24690).
