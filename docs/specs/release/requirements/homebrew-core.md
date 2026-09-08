---
status: active
system: release
created: 2026-05-14
owners:
  - tbd
---
# Homebrew Core Submission Requirements

## Overview

Kandev is currently installable via `brew install kdlbs/kandev/kandev` from the `kdlbs/homebrew-kandev` tap. The tap formula downloads pre-built release tarballs, which works for end users but is rejected by `homebrew/homebrew-core` policy. Landing in homebrew-core means:

## Requirements

### REQ-RELEASE-HOMEBREW-CORE-001: Homebrew Core Submission

**Intent:** Kandev is currently installable via `brew install kdlbs/kandev/kandev` from the `kdlbs/homebrew-kandev` tap. The tap formula downloads pre-built release tarballs, which works for end users but is rejected by `homebrew/homebrew-core` policy. Landing in homebrew-core means:

#### Acceptance criteria

- **AC-RELEASE-HOMEBREW-CORE-001.1:** Add a package-manager build contract that produces the native runtime bundle from an already-installed source checkout. It builds the Vite SPA, syncs it into the Go embed input, compiles the host `kandev` and `agentctl` binaries, and cross-compiles the four Linux/macOS remote `agentctl` helpers.
- **AC-RELEASE-HOMEBREW-CORE-001.2:** Keep package-manager dependency installation outside that contract. Homebrew declares Go, Node 24, and pnpm 10 as build dependencies. Git is part of Homebrew's standard environment and is not declared as a formula dependency.
- **AC-RELEASE-HOMEBREW-CORE-001.3:** Compile only the host `kandev` binary with cgo and the `fts5` tag. `mattn/go-sqlite3` supplies the SQLite amalgamation, so the formula does not declare an external SQLite dependency unless build or linkage evidence requires it.
- **AC-RELEASE-HOMEBREW-CORE-001.4:** Merge and publish the source-build contract in a Stable Kandev release before authoring the Core formula. Core must consume the immutable GitHub tag archive for that release, never a branch or unreleased commit.
- **AC-RELEASE-HOMEBREW-CORE-001.5:** Install the bundle under `libexec/bin` and expose one `bin/kandev` wrapper produced by `write_env_script`. The wrapper sets `KANDEV_BUNDLE_DIR=<libexec>` and `KANDEV_VERSION=<version>`.
- **AC-RELEASE-HOMEBREW-CORE-001.6:** Use stable numeric tags for `livecheck`; prerelease and Nightly npm versions are not Homebrew channels.
- **AC-RELEASE-HOMEBREW-CORE-001.7:** Keep `kdlbs/homebrew-kandev` as the upstream binary fast path alongside Homebrew Core's source-built bottles. The generated tap formula must set the Stable SemVer explicitly because platform archive names end in architecture tokens such as `x64`, and it must smoke-test its version, readiness endpoint, and embedded SPA. The shared release-bundle validator runs before those binary archives are published.
- **AC-RELEASE-HOMEBREW-CORE-001.8:** Preserve all four remote `agentctl` helpers in custom-tap installations. The tap must use an exact-path Homebrew mismatched-binary audit allowlist rather than pruning helpers, so Docker and SSH targets can differ from the Homebrew host. Decision: [ADR-2026-08-05-homebrew-remote-helper-audit](../../../decisions/2026-08-05-homebrew-remote-helper-audit.md).

## Migrated source detail

## Why

Kandev is currently installable via `brew install kdlbs/kandev/kandev` from the `kdlbs/homebrew-kandev` tap. The tap formula downloads pre-built release tarballs, which works for end users but is rejected by `homebrew/homebrew-core` policy. Landing in homebrew-core means:

- `brew install kandev` (no tap required) — lower friction discovery.
- Bottles are built and signed by Homebrew's CI, eliminating per-platform GH-release tarballs as the install path.
- Automated version bumps via `brew bump-formula-pr` once `livecheck` is wired up.

## What

- Add a package-manager build contract that produces the native runtime bundle from an already-installed source checkout. It builds the Vite SPA, syncs it into the Go embed input, compiles the host `kandev` and `agentctl` binaries, and cross-compiles the four Linux/macOS remote `agentctl` helpers.
- Keep package-manager dependency installation outside that contract. Homebrew declares Go, Node 24, and pnpm 10 as build dependencies. Git is part of Homebrew's standard environment and is not declared as a formula dependency.
- Compile only the host `kandev` binary with cgo and the `fts5` tag. `mattn/go-sqlite3` supplies the SQLite amalgamation, so the formula does not declare an external SQLite dependency unless build or linkage evidence requires it.
- Merge and publish the source-build contract in a Stable Kandev release before authoring the Core formula. Core must consume the immutable GitHub tag archive for that release, never a branch or unreleased commit.
- Install the bundle under `libexec/bin` and expose one `bin/kandev` wrapper produced by `write_env_script`. The wrapper sets `KANDEV_BUNDLE_DIR=<libexec>` and `KANDEV_VERSION=<version>`.
- Use stable numeric tags for `livecheck`; prerelease and Nightly npm versions are not Homebrew channels.
- Keep `kdlbs/homebrew-kandev` as the upstream binary fast path alongside Homebrew Core's source-built bottles. The generated tap formula must set the Stable SemVer explicitly because platform archive names end in architecture tokens such as `x64`, and it must smoke-test its version, readiness endpoint, and embedded SPA. The shared release-bundle validator runs before those binary archives are published.
- Preserve all four remote `agentctl` helpers in custom-tap installations. The tap must use an exact-path Homebrew mismatched-binary audit allowlist rather than pruning helpers, so Docker and SSH targets can differ from the Homebrew host. Decision: [ADR-2026-08-05-homebrew-remote-helper-audit](../../../decisions/2026-08-05-homebrew-remote-helper-audit.md).

The runtime bundle contains exactly:

- host `kandev`;
- host `agentctl`;
- `agentctl-linux-amd64`;
- `agentctl-linux-arm64`;
- `agentctl-darwin-arm64`;
- `agentctl-darwin-amd64`.

The Darwin arm64 helper must carry a Mach-O code signature so Apple Silicon can execute it.

## Scenarios

- **GIVEN** the source-build contract is merged, **WHEN** a Stable release is published, **THEN** its immutable tag archive contains the contract used by the Homebrew formula.
- **GIVEN** the homebrew-core PR is merged, **WHEN** a user runs `brew install kandev`, **THEN** Homebrew builds the Vite assets and native runtime bundle from source, installs it under `Cellar/kandev/X.Y.Z/{bin,libexec}`, and `kandev --version` prints `X.Y.Z` without Node being present at runtime.
- **GIVEN** the installed formula starts with an isolated home directory and loopback port, **WHEN** `brew test kandev` polls `GET /health`, **THEN** readiness returns `{"status":"ok"}` and `/` serves the embedded page containing `<title>Kandev</title>`.
- **GIVEN** a new kandev release `vX.Y.Z` is tagged, **WHEN** Homebrew's auto-bump worker runs, **THEN** `livecheck` resolves the new tag from GitHub Releases and a bump PR is opened against the formula.
- **GIVEN** a maintainer reviews the PR, **WHEN** they run `brew install --build-from-source kandev` locally, **THEN** the build completes without network or sandbox failures and `brew test kandev` passes.
- **GIVEN** a Stable release updates `kdlbs/homebrew-kandev`, **WHEN** its platform archive is built and the tap formula is tested, **THEN** the archive contains the complete executable runtime and the installed launcher serves both `/health` and the embedded Kandev page.
- **GIVEN** a Stable release updates `kdlbs/homebrew-kandev`, **WHEN** Homebrew evaluates a platform archive URL ending in `x64` or `arm64`, **THEN** the generated formula's explicit version keeps the Cellar path and `version` value at `X.Y.Z` rather than the architecture suffix.
- **GIVEN** a tap archive contains remote helpers for CPU architectures other than the Homebrew host, **WHEN** Homebrew audits the installed formula on macOS or Linux, **THEN** only the four declared remote-helper paths are exempted and the complete runtime remains installed.

## Out of scope

- Migrating users from `kdlbs/homebrew-kandev` to homebrew-core (both can coexist; users opt in by switching tap reference).
- Linuxbrew bottle parity beyond what homebrew-core's CI provides by default.
- Vendoring JS dependencies via `resource` blocks — falls back here only if maintainers reject network-during-install.
- Changing the custom tap's direct-push publication or deploy-key authentication model.
- Advertising untapped `brew install kandev` before the Core formula merges.
- Notability lobbying — submission goes in as-is; maintainers decide.
