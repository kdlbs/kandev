---
spec: docs/specs/platform/requirements/e2e-duration-aware-sharding.md
created: 2026-08-18
status: complete
---

# Implementation Plan: Prebuilt Desktop E2E Image

## Overview

Merge-queue run `32170420828` confirmed that `apt-get update` can stall until
the 45-minute desktop job limit expires. The repair moves stable system and
Rust toolchain dependencies into a dedicated GHCR image. The desktop smoke job
then starts from that image and retains only source-specific installation,
build, and test work.

## CI image

### Desktop stage

Extend `.github/docker/ci-base/Dockerfile` with a `desktop` stage based on the
existing `runtime` stage. Install these items while the image is built:

- Rust `1.97.1` with the minimal profile,
- `build-essential` and `rpm` for the Tauri bundle,
- `pkg-config`,
- `libglib2.0-dev`,
- `libwebkit2gtk-4.1-dev`,
- `libgtk-3-dev`,
- `libayatana-appindicator3-dev`,
- `librsvg2-dev`,
- `patchelf`, and
- `xvfb`.

Set `RUST_VERSION=1.97.1`, `CARGO_HOME=/root/.cargo`, and the Rust path in the
image. Add image-build smoke commands for `rustc`, `cargo`,
`pkg-config webkit2gtk-4.1`, `patchelf`, and `xvfb-run`. Do not add Go or the
backend build tools to this stage.

### Image publisher

Extend `.github/workflows/ci-base-image.yml` with one Buildx invocation for the
`desktop` target. Publish `desktop-sha-<content-hash>` and `desktop-latest` with
the same rules as the existing images. Use a `desktop` cache scope for new
layers and the `runtime` scope as a lower-layer source. Add the desktop tag to
the workflow summary.

## Desktop smoke workflow

Update the `desktop-e2e` job in `.github/workflows/e2e-tests.yml` to use
`ghcr.io/kdlbs/kandev-ci:desktop-latest` as its job container. Add the safe Git
directory step and the pnpm-store cache pattern from the existing container
jobs. Use `--ipc=host` so WebKit receives the same shared-memory configuration
as the validated local smoke command. Include the image Dockerfile and
publisher workflow in the job's in-workflow change patterns.

Remove the pnpm setup, Node.js setup, Rust setup, and system-package steps.
Keep the Rust build cache, workspace install, and desktop smoke command. Reduce
the timeout only if a measured image-based run supports a smaller limit.

## Tests

- **What:** the desktop image contains the required packages and toolchain, and
  the publisher emits both desktop tags.
  **File:** `.github/scripts/e2e-tests-workflow-contract_test.py`
  **How:** add textual contract assertions for the Docker stage, package list,
  smoke commands, Buildx target, cache scopes, tags, and summary.
- **What:** the desktop smoke job consumes the desktop image without live
  system-package or Rust toolchain installation.
  **File:** `.github/scripts/e2e-tests-workflow-contract_test.py`
  **How:** isolate the `desktop-e2e` job and assert its image, caches, retained
  smoke command, and forbidden setup commands.
- **What:** the contract test remains part of a required, unfiltered check.
  **File:** `.github/workflows/lint-action-pinning.yml`
  **How:** add the contract test to the existing workflow-contract test steps.
- **What:** the desktop job keeps the validated IPC setting and runs when its
  image inputs change.
  **File:** `.github/scripts/e2e-tests-workflow-contract_test.py`
  **How:** assert `options: --ipc=host` and the image paths in the E2E change
  detection block.

The regression test must fail against the current workflow because the desktop
image and its publisher do not exist. It must also report the current live
`apt-get` and `rustup` setup as contract errors.

## Rollout

The new `desktop-latest` tag does not exist before this change. After the
implementation branch is pushed, start `ci-base-image.yml` with
`workflow_dispatch` on that branch. Wait for a successful image publish before
the pull request E2E workflow consumes the tag. Use the workflow publisher for
this bootstrap. Do not create or retag the image manually.

The first pull request E2E run must show that `Desktop E2E Smoke` enters the
container and runs the desktop command. Its log must contain no Ubuntu mirror
or Rust toolchain download step.

## Verification Results

Local contract, action-pinning, Dockerfile, and desktop smoke checks pass. The
branch image publisher run `32191451396` completed successfully, including the
`Build and push desktop` job `95886471809`. PR E2E run `32188852911` attempt 2
completed successfully; `Desktop E2E Smoke` job `95887857830` entered the
published container, built DEB/RPM bundles, and reported a successful WebView
smoke. Its log contains source dependency installation but no live apt or Rust
toolchain setup.

## Implementation Waves And Parallel Candidates

Execute sequentially in the primary session:

### Wave 1

- [x] [task-01-prebuild-desktop-image](task-01-prebuild-desktop-image.md)

### Wave 2

- [x] [task-02-consume-and-prove-image](task-02-consume-and-prove-image.md)

## Risks

- GitHub job containers can expose display or WebKit behavior that differs
  from the hosted runner. The image-based desktop smoke run is the acceptance
  test for this boundary.
- A new GHCR tag creates a bootstrap dependency. The documented branch
  dispatch resolves it before the consumer check runs.
- pnpm and Cargo can still contact their registries on a cold cache. This
  repair removes the unbounded operating-system and toolchain setup path only.

## Open Questions

None.
