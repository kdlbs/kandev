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
5. Check GitHub/GHCR access, npm trusted publishing, the release-tag GPG key, the Homebrew deploy key, and any configured desktop signing/notarization secrets.
6. Update public docs for behavior that is about to ship.

Normal releases require a dedicated release-tag signing identity and a GitHub Environment named `release`. Restrict that environment to deployments from `main`, but do not configure required reviewers: any maintainer authorized to dispatch the workflow and access the environment may start a normal release. Store the ASCII-armored private key as the environment secret `RELEASE_GPG_PRIVATE_KEY`, its optional passphrase as the environment secret `RELEASE_GPG_PASSPHRASE`, and the exact full fingerprint as the required environment variable `RELEASE_GPG_FINGERPRINT`. Before importing the private key, the workflow rejects a missing, multi-key, or secret-material public-key attachment and requires its sole primary fingerprint to match `RELEASE_GPG_FINGERPRINT`; it then requires the imported private key to match both values. Do not define this signing material as repository-wide configuration.

The `prepare` job selects its environment by mode: normal releases use `release`, while dry runs, desktop validation, and backfills use `release-validation`. Leave `release-validation` as an empty default environment with no secrets, variables, deployment restrictions, or protection rules so validation and repair modes remain usable without access to the signing identity.

GitHub and the repository are the source of truth for the release public key. The current key is committed at `.github/release-signing-key.asc`; its full fingerprint is `FFB03BCD68F5BCBBD2D1767A84EAB9CE3B8EF52F`. Before rotating the key, update that repository file and the `release` environment secret and fingerprint variable together. Retain prior public keys and revocation information in repository history so historical signatures remain auditable. Key generation, private-key backup, rotation, and revocation are maintainer responsibilities; never place private key material in the repository.

Repository administrators must also enforce a GitHub tag ruleset targeting `v*`. Enable **Restrict updates** and **Restrict deletions**, leave **Restrict creations** off, and keep the bypass list empty. Do not relax this ruleset to perform a backfill; backfill must reuse the existing tag. Enable immutable GitHub Releases when the repository supports that setting, but do not treat release immutability as a substitute for tag protection before the release is published.

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
2. **Merge and tag.** Open a release branch and PR, squash-merge it, then import the protected signing key. The workflow requires the imported key's full fingerprint to exactly match `RELEASE_GPG_FINGERPRINT`, adopts that key's name and email as the tagger identity, and locally verifies the GPG-signed `vX.Y.Z` tag before pushing it.
3. **Build web and runtimes.** Build the SPA and five runtime targets: Linux x64/arm64, macOS x64/arm64, and Windows x64. Each archive contains `kandev`, the host `agentctl`, and required remote agentctl helpers; the workflow produces an adjacent checksum for each archive.
4. **Build desktop.** Embed the matching runtime and package the same five platform/architecture targets into macOS, Linux, and Windows installer formats.
5. **Build containers.** Publish amd64/arm64 base manifests, enforce the universal-image size gate, then publish multi-architecture universal images.
6. **Publish GitHub Release.** Attach runtime archives, checksums, desktop artifacts, notes, and the updater feed when eligible.
7. **Publish npm.** Use OIDC trusted publishing for five `@kdlbs/runtime-*` packages first, then the `kandev` launcher. Existing versions are skipped; the main package is not published after a runtime-package failure.
8. **Update Homebrew.** Push the formula update to `kdlbs/homebrew-kandev` using release checksums and the deploy key.

GHCR images are built before the GitHub Release. npm and Homebrew start only after the GitHub Release and may run in parallel. A late failure can therefore leave some channels complete and others missing.

Base image tags include `X.Y.Z`, `vX.Y.Z`, `sha-*`, and `latest`. Universal tags include `X.Y.Z-universal`, `vX.Y.Z-universal`, and the floating `universal`. The weekly universal rebuild updates only floating/dated weekly tags, never a version-specific release tag.

## Signing and updater behavior

npm uses GitHub OIDC trusted publishers; there is no `NPM_TOKEN` release path. The main `kandev` package and all five `@kdlbs/runtime-*` packages publish provenance attestations. Homebrew requires its repository deploy key.

Release-tag signing applies to future normal releases only. Backfills reuse the existing tag, and historical unsigned tags are not recreated or moved.

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

Obtain the release public key from `.github/release-signing-key.asc` in the GitHub repository and confirm that its fingerprint is `FFB03BCD68F5BCBBD2D1767A84EAB9CE3B8EF52F` before importing it into your GPG keyring and verifying a tag:

```bash
gpg --import .github/release-signing-key.asc
git fetch --tags origin
git tag -v vX.Y.Z
```

Compare the displayed fingerprint with the repository's release-key fingerprint. To check npm provenance, inspect the registry attestations for the launcher and every runtime package, then verify installed dependency signatures and attestations with npm:

```bash
VERSION=X.Y.Z
for package in kandev \
  @kdlbs/runtime-linux-x64 @kdlbs/runtime-linux-arm64 \
  @kdlbs/runtime-darwin-x64 @kdlbs/runtime-darwin-arm64 \
  @kdlbs/runtime-win32-x64; do
  npm view "$package@$VERSION" dist.attestations
done
npm audit signatures
```

## Repair a partial release

First identify exactly which immutable and mutable channels succeeded. Preserve workflow logs, checksums, package versions, image digests, and signing output.

If both tag push attempts fail after the release PR merges, the workflow logs the exact release merge commit. First confirm that `refs/tags/vX.Y.Z` is absent from the remote. Then, from an audited recovery context authorized by the tag ruleset and using the configured release signing identity, recreate and verify the tag at that exact commit:

```bash
git fetch origin main
git checkout --detach <release-merge-commit>
git tag -s vX.Y.Z -m "release: X.Y.Z"
git tag -v vX.Y.Z
git push origin vX.Y.Z
```

Before pushing, confirm that `git tag -v` reports the full `RELEASE_GPG_FINGERPRINT` recorded with `.github/release-signing-key.asc`. An arbitrary clone cannot recover by pushing alone because the locally created tag existed only on the failed ephemeral runner.

Use `backfill_tag` only for the latest release when shipped source is correct and the failure is missing artifacts or a recoverable publication step. Backfill checks out application source from the tag, validates all version manifests, and uses the current workflow's control-plane helpers to rebuild or reconcile GitHub Release, GHCR, npm, desktop/updater, and Homebrew channels. Existing npm versions are not overwritten.

Publish a new patch instead when code is defective, an immutable npm package or version-specific image is wrong, manifests disagree, or repair would require changing tagged source. Never delete/reuse a published tag or move an npm version as a routine fix.

For implementation detail, inspect `.github/workflows/release.yml`, `.github/workflows/universal-rebuild.yml`, `scripts/release/`, `apps/cli/README_internal.md`, and desktop packaging scripts. Those files are automation source; this page is the contributor operating contract.
