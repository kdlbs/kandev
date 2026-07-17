---
title: "Release Process"
description: "Run and verify Kandev's version, runtime, desktop, container, npm, GitHub, updater, and Homebrew release automation."
---

# Release Process

Kandev uses one semantic version across the Git tag, native runtime bundles, desktop app, npm packages, GitHub release, container images, and Homebrew formula. Publish through the manual **Release** GitHub Actions workflow; do not update channels independently.

## Choose the workflow mode

The workflow has four mutually exclusive operating modes:

| Mode | Inputs | Result |
|---|---|---|
| Normal release | `bump=patch`, `minor`, or `major` | Creates and merges a release PR, tags its merge, builds, and publishes |
| Dry run | `dry_run=true` | Computes the next version and exercises CLI package/lock plus changelog generation in the runner; no PR, tag, artifact build, or publication |
| Desktop validation | `desktop_validation_only=true` | Builds web, five runtime bundles, and five desktop targets from the selected commit; no PR, tag, GitHub release, GHCR, npm, or Homebrew publication |
| Backfill | `backfill_tag=vX.Y.Z` | Rebuilds and repairs channels for the latest existing release tag without creating a version or tag |

`dry_run` and desktop validation are not release candidates. Backfill cannot be combined with either and accepts only the latest exact SemVer tag after version manifests are checked for agreement.

## Before dispatch

1. Open the **Release** workflow and explicitly select the `main` ref. Normal mode creates its release branch from the selected ref.
2. Confirm required checks are green on `main` and no release or release PR is active.
3. Confirm merged PR titles/commits use the conventional categories consumed by `cliff.toml`. The workflow generates `CHANGELOG.md` and release notes.
4. Verify platform-sensitive launcher, agentctl, container, and desktop changes on affected targets.
5. Check GitHub/GHCR access, npm trusted publishing, the Homebrew deploy key, and any configured desktop signing/notarization secrets.
6. Update public docs for behavior that is about to ship.

For release automation changes, run:

```bash
python3 .github/scripts/release-workflow-contract_test.py
bash scripts/release-desktop.test.sh
make test-cli
```

Use dry run to validate version/changelog preparation. Use desktop validation to validate packaging from the current commit.

## Normal release flow

Normal mode performs these stages:

1. **Prepare version.** Compute the next version from packages and tags. Update the CLI package/lock, desktop package and Tauri/Cargo manifests, and `CHANGELOG.md`.
2. **Merge and tag.** Open a release branch and PR, squash-merge it, then create `vX.Y.Z` at the merge commit.
3. **Build web and runtimes.** Build the SPA and five runtime targets: Linux x64/arm64, macOS x64/arm64, and Windows x64. Each archive contains `kandev`, the host `agentctl`, and required remote agentctl helpers; the workflow produces an adjacent checksum for each archive.
4. **Build desktop.** Embed the matching runtime and package the same five platform/architecture targets into macOS, Linux, and Windows installer formats.
5. **Build containers.** Publish amd64/arm64 base manifests, enforce the universal-image size gate, then publish multi-architecture universal images.
6. **Publish GitHub Release.** Attach runtime archives, checksums, desktop artifacts, notes, and the updater feed when eligible.
7. **Publish npm.** Use OIDC trusted publishing for five `@kdlbs/runtime-*` packages first, then the `kandev` launcher. Existing versions are skipped; the main package is not published after a runtime-package failure.
8. **Update Homebrew.** Push the formula update to `kdlbs/homebrew-kandev` using release checksums and the deploy key.

GHCR images are built before the GitHub Release. npm and Homebrew start only after the GitHub Release and may run in parallel. A late failure can therefore leave some channels complete and others missing.

Base image tags include `X.Y.Z`, `vX.Y.Z`, `sha-*`, and `latest`. Universal tags include `X.Y.Z-universal`, `vX.Y.Z-universal`, and the floating `universal`. The weekly universal rebuild updates only floating/dated weekly tags, never a version-specific release tag.

## Signing and updater behavior

npm uses GitHub OIDC trusted publishers; there is no `NPM_TOKEN` release path. Homebrew requires its repository deploy key.

Desktop OS signing and notarization are conditional on a complete secret set. Without them, the workflow can publish unsigned installers and adds a warning to release notes. Tauri updater signatures are stricter: the workflow publishes updater artifacts and `latest.json` only when the required signed set is complete. Do not claim in-app update availability from the presence of installers alone.

Never print signing material, tokens, certificate contents, or generated updater private data in logs.

## Verify every channel

After publication, verify:

- the Git tag points at the generated release merge;
- GitHub notes, five runtime archives, checksums, expected desktop installers, and conditional `latest.json`;
- all five runtime npm packages plus `kandev`, including a clean `npx kandev@latest`;
- Homebrew install/upgrade and `kandev --version`;
- GHCR base and universal images on amd64 and arm64, including their immutable version tags;
- desktop launch on affected platforms and signed/notarized status where configured;
- backend health, a minimal task, agentctl startup, and Updates-screen behavior;
- public docs describe the released behavior rather than unreleased `main` where version differences matter.

Record artifact URLs/digests and the workflow run. Do not treat a successful tag or one working installer as a complete release.

## Repair a partial release

First identify exactly which immutable and mutable channels succeeded. Preserve workflow logs, checksums, package versions, image digests, and signing output.

Use `backfill_tag` only for the latest release when shipped source is correct and the failure is missing artifacts or a recoverable publication step. Backfill checks out application source from the tag, validates all version manifests, and uses the current workflow's control-plane helpers to rebuild or reconcile GitHub Release, GHCR, npm, desktop/updater, and Homebrew channels. Existing npm versions are not overwritten.

Publish a new patch instead when code is defective, an immutable npm package or version-specific image is wrong, manifests disagree, or repair would require changing tagged source. Never delete/reuse a published tag or move an npm version as a routine fix.

For implementation detail, inspect `.github/workflows/release.yml`, `.github/workflows/universal-rebuild.yml`, `scripts/release/`, `apps/cli/README_internal.md`, and desktop packaging scripts. Those files are automation source; this page is the contributor operating contract.
