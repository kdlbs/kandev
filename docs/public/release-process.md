---
title: "Release Process"
description: "Understand Kandev's automated version, changelog, runtime bundle, desktop, npm, GitHub, and Homebrew release flow."
---

# Release Process

Kandev uses one semantic version for the CLI, native runtime bundles, desktop artifacts, npm packages, Git tag, GitHub release, and Homebrew formula. Releases are driven by the **Release** GitHub Actions workflow, not by manually publishing individual channels.

## Before a release

1. Confirm required checks pass on `main`.
2. Review user-visible changes and update `docs/public/**` plus changelog inputs.
3. Verify platform-sensitive launcher, agentctl, and desktop changes on their target systems.
4. Confirm publishing credentials, signing/notarization secrets, npm access, and Homebrew tap access are healthy.
5. Avoid starting a release while another version or release PR is active.

## Run the workflow

In GitHub Actions, open **Release**, choose **Run workflow**, select `patch`, `minor`, or `major`, and optionally enable dry-run validation.

The workflow validates the requested bump, computes the next version, updates the package version and changelog, opens and merges the release PR, creates the `vX.Y.Z` tag, then builds and publishes the release channels.

Do not create a competing tag or edit generated release files while the workflow is running.

## Artifacts and channels

The release produces platform runtime bundles containing the unified `kandev` binary and agentctl helpers. npm installs a small launcher plus one platform-selected `@kdlbs/runtime-*` optional package. Homebrew consumes GitHub release tarballs. The Tauri desktop build embeds the matching runtime before packaging and signing.

Supported runtime targets and artifact shapes are maintained in `apps/cli/README_internal.md` and the release scripts. When adding a platform, update build matrices, npm optional dependencies, archive names, launcher mapping, tests, checksums, desktop resources, and Homebrew handling together.

## Signing and publishing

Desktop signing/notarization and package publishing depend on repository secrets. Logs and artifacts must not expose signing material. A failure after the tag exists is a release-repair incident: inspect which channels published before rerunning or changing tags.

Never overwrite an already published semantic version. Fix the pipeline and publish a new patch when immutable package registries or release assets require it.

## Verify the release

After publication:

- check the GitHub release, notes, checksums, and expected platform archives;
- install with `npx kandev@latest` in a clean directory;
- verify a Homebrew install or upgrade;
- launch the desktop artifact on signed target platforms;
- run `kandev --version` and a minimal first task;
- confirm the Updates screen sees the new release;
- verify public docs describe the shipped behavior rather than unreleased `main` where it matters.

## Rollback and repair

Published artifacts are immutable evidence. If a release is broken:

1. stop or disable the failing publication stage if it is still running;
2. record which channels and architectures are affected;
3. preserve logs and release metadata;
4. fix on `main` with focused tests;
5. publish a new patch release;
6. document upgrade or mitigation steps for affected users.

For the exact workflow implementation, inspect `.github/workflows/release.yml`, `scripts/release/`, `apps/cli/README_internal.md`, and the desktop signing documentation in the repository. These internal files are more detailed but are not the user-facing product contract.
