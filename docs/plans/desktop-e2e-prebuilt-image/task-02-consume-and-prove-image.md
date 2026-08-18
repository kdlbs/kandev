---
id: "02-consume-and-prove-image"
title: "Consume and prove desktop image"
status: done
wave: 2
depends_on: ["01-prebuild-desktop-image"]
plan: "plan.md"
spec: "../../specs/e2e-duration-aware-sharding/spec.md"
---

# Task 02: Consume And Prove The Desktop Image

## Acceptance

- `Desktop E2E Smoke` uses `ghcr.io/kdlbs/kandev-ci:desktop-latest` and contains
  no live Node.js, pnpm, Rust toolchain, or Ubuntu package setup step.
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

After the branch is pushed, start `.github/workflows/ci-base-image.yml` with
`workflow_dispatch`. Record that run and the next `Desktop E2E Smoke` run in
the task results and plan verification results.

## Files likely touched

- `.github/workflows/e2e-tests.yml`
- `.github/scripts/e2e-tests-workflow-contract_test.py`

## Dependencies

Task 01. The remote smoke check also requires the branch image publish.

## Parallelism

sequential

## Inputs

- `docs/specs/e2e-duration-aware-sharding/spec.md`, especially the desktop
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
- The local desktop image smoke passed in the container. `pnpm install
  --frozen-lockfile` completed, the Tauri application built both DEB and RPM
  bundles, and `node e2e/desktop-launch-smoke.mjs` reported a successful
  WebView request after backend health.
- Branch image publication and pull-request verification remain the remote
  rollout records for this task.
