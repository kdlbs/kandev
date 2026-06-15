---
status: draft
created: 2026-05-14
owner: tbd
---

# Homebrew Core Submission

## Why

Kandev is currently installable via `brew install kdlbs/kandev/kandev` from the `kdlbs/homebrew-kandev` tap. The tap formula downloads pre-built release tarballs, which works for end users but is rejected by `homebrew/homebrew-core` policy. Landing in homebrew-core means:

- `brew install kandev` (no tap required) — lower friction discovery.
- Bottles are built and signed by Homebrew's CI, eliminating per-platform GH-release tarballs as the install path.
- Automated version bumps via `brew bump-formula-pr` once `livecheck` is wired up.

## What

- Author a homebrew-core-compliant `Formula/kandev.rb` that **builds entirely from source** — Go backend (cgo + sqlite via `mattn/go-sqlite3`), Vite web bundle, and the TypeScript CLI esbuild bundle.
- Source comes from the GitHub-generated tag archive (`https://github.com/kdlbs/kandev/archive/refs/tags/vX.Y.Z.tar.gz`); per-release sha256 is captured at submission/bump time.
- Build deps: `go => :build`, `pnpm => :build`. Runtime dep: `node` (CLI launcher). `uses_from_macos "sqlite"` (cgo sqlite3 link) ships with macOS and is only installed via Homebrew on Linux.
- Install layout: `libexec/{bin,web,cli}` plus a single `bin/kandev` wrapper produced by `write_env_script`, setting `KANDEV_BUNDLE_DIR=<libexec>` and `KANDEV_VERSION=<version>`. This is the contract `apps/cli/src/runtime.ts` already honors.
- `livecheck do; url :stable; regex(/^v?(\d+(?:\.\d+)+)$/i); end` — Git strategy (the default for `url :stable`) checks tags directly and avoids the GitHub API rate limit.
- Test block: spawns `libexec/bin/kandev` with an isolated `KANDEV_HOME_DIR` and random `KANDEV_SERVER_PORT`, polls `/api/v1/system/health` until the response contains `"healthy"` (60s timeout), then shuts the backend down. Also asserts the formula `version` appears in `bin/kandev --version`.
- Tap (`kdlbs/homebrew-kandev`) and its update script (`scripts/release/update-homebrew-tap.sh`) stay untouched — it remains the binary-install fast path; the homebrew-core formula is a parallel, source-built distribution.

## Scenarios

- **GIVEN** the homebrew-core PR is merged, **WHEN** a macOS user runs `brew install kandev`, **THEN** Homebrew downloads the source tarball, compiles the Go binaries, builds the Next.js + CLI bundles, installs them under `Cellar/kandev/X.Y.Z/{bin,libexec}`, and `kandev --help` prints "kandev launcher".
- **GIVEN** a new kandev release `vX.Y.Z` is tagged, **WHEN** Homebrew's auto-bump worker runs, **THEN** `livecheck` resolves the new tag from GitHub Releases and a bump PR is opened against the formula.
- **GIVEN** a maintainer reviews the PR, **WHEN** they run `brew install --build-from-source kandev` locally, **THEN** the build completes without network or sandbox failures and `brew test kandev` passes.

## Out of scope

- Migrating users from `kdlbs/homebrew-kandev` to homebrew-core (both can coexist; users opt in by switching tap reference).
- Linuxbrew bottle parity beyond what homebrew-core's CI provides by default.
- Vendoring JS dependencies via `resource` blocks — falls back here only if maintainers reject network-during-install.
- Changes to `.github/workflows/release.yml` or `scripts/release/update-homebrew-tap.sh`.
- Notability lobbying — submission goes in as-is; maintainers decide.
