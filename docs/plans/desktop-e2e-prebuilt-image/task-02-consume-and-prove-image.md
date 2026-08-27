---
id: "02-consume-and-prove-image"
title: "Consume and prove desktop image"
status: done
wave: 2
depends_on: ["01-prebuild-desktop-image"]
plan: "plan.md"
spec: "../../specs/platform/requirements/e2e-duration-aware-sharding.md"
---

# Task 02: Consume And Prove The Desktop Image

## Acceptance

- `Desktop E2E Smoke` uses `ghcr.io/kdlbs/kandev-ci:desktop-latest` and contains
  no live Node.js, pnpm, Rust toolchain, or Ubuntu package setup step.
- The container uses `--ipc=host`, and E2E change detection includes the image
  Dockerfile and publisher workflow.
- The job retains the pnpm and Rust caches, workspace install, and real desktop
  smoke command.
- The branch image publish and the image-based desktop smoke job succeed, with
  no Ubuntu mirror or Rust toolchain download in the smoke-job log.

## Verification

```bash
python3 .github/scripts/e2e-tests-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
python3 .github/scripts/lint-action-pinning.py
docker run --rm --ipc=host --volume "$PWD:/work" --workdir /work/apps kandev-ci:desktop-local bash -lc 'pnpm install --frozen-lockfile && pnpm --filter @kandev/desktop e2e'
```

The branch image publisher run `32191451396` succeeded. The PR E2E run
`32188852911` attempt 2 succeeded after rerunning its failed jobs; its
`Desktop E2E Smoke` job `95887857830` passed container initialization,
dependency installation, DEB/RPM bundling, and the WebView smoke.

## Files likely touched

- `.github/workflows/e2e-tests.yml`
- `.github/scripts/e2e-tests-workflow-contract_test.py`

## Dependencies

Task 01. The remote smoke check also requires the branch image publish.

## Parallelism

sequential

## Inputs

- `docs/specs/platform/requirements/e2e-duration-aware-sharding.md`, especially the desktop
  smoke bootstrap requirements.
- `docs/plans/desktop-e2e-prebuilt-image/plan.md`, Desktop smoke workflow,
  Rollout, and Risks sections.
- The pnpm and safe-directory patterns in the existing container jobs.

## Output contract

Report the RED and GREEN contract results, local image smoke result, GHCR
publish run, Actions desktop smoke result, changed files, blockers, risks, and
the synchronized task and plan status.

## Results

- The desktop workflow contract passed (`4 tests`) after the image consumer was
  added.
- Review remediation added the host IPC setting and image paths to change
  detection; the contract test first failed on the missing IPC mapping and
  then passed after both changes.
- The local desktop image smoke passed in the container. `pnpm install
  --frozen-lockfile` completed, the Tauri application built both DEB and RPM
  bundles, and `node e2e/desktop-launch-smoke.mjs` reported a successful
  WebView request after backend health.
- Remote image publication and pull-request verification are complete. The
  smoke log records `pnpm install --frozen-lockfile`, successful DEB/RPM
  bundles, and `Desktop smoke passed: WebView requested / after backend health`;
  it contains no live apt or Rust toolchain setup.
- Changed files, blockers, risks, and synchronized plan status are recorded in
  this task and the parent plan.
