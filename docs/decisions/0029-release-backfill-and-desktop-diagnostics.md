# 0029: Release Backfill and Desktop Diagnostics

**Status:** accepted
**Date:** 2026-07-01
**Area:** infra, workflow

## Context

The release workflow can create and push a version tag before all platform artifacts have been built and published. If a later artifact job fails, a normal rerun from `workflow_dispatch` cannot safely resume the release because the tag already exists and the prepare job is intentionally non-idempotent for version bumping and tag creation.

The first desktop macOS failure mode we observed happened after Rust compilation completed, while Tauri was bundling the DMG. The GitHub Actions log ended at `bundle_dmg.sh` with little nested stderr. A Linux CI container image cannot remove this class of failure because DMG packaging must run on native macOS runners.

## Decision

Add a maintainer-only release backfill mode to `.github/workflows/release.yml`. The `backfill_tag` input accepts the current highest existing `vX.Y.Z` release tag, verifies that the tag exists, and verifies that all release-critical manifests at that tag match the version. It then uses the existing tag as the build and publish ref. In this mode the workflow skips version bumping, changelog generation, release PR creation, and tag creation, while still running the build, GitHub release, npm publish, and Homebrew update jobs.

Add macOS desktop packaging guardrails to the Tauri build job. macOS DMG builds now run with a bounded timeout and a retry, and failed macOS desktop builds upload diagnostics that include disk state, `hdiutil info`, related packaging processes, bundle directory listings, and Tauri's generated `bundle_dmg.sh` when present.

Keep desktop installer builds on native OS runners. Linux container images remain useful for Linux CI and e2e dependency stability, but they are not the mechanism for macOS DMG recovery.

## Consequences

Release recovery becomes explicit and repeatable for tag-only or partially published current releases. Publishing scripts can keep their existing idempotent behavior, so rerunning a backfill is safe when some assets or packages already exist. Older tags are rejected because this workflow still updates mutable channels such as Docker tags, npm dist-tags, and the Homebrew formula.

The release workflow has more branching logic, and maintainers must choose between normal release, desktop validation, dry run, and tag backfill modes intentionally. The workflow contract is covered by a small CI test so later YAML changes do not silently remove the recovery path or macOS diagnostics.

macOS package failures should now leave actionable artifacts instead of only the final Tauri command line. The retry may add time to a failing macOS job, but the step-level timeout caps total time.

## Alternatives Considered

1. **Rerun the normal release workflow.** Rejected because the existing tag makes prepare fail before artifact rebuild and publish jobs can run.
2. **Create a separate duplicate backfill workflow.** Rejected because it would duplicate the large release build/publish graph and increase drift risk.
3. **Use a custom Linux Docker image for release packaging.** Rejected for macOS DMG recovery because native DMG packaging requires macOS runners. This can still help Linux jobs that fail while downloading build dependencies, but it does not address the observed release failure.
