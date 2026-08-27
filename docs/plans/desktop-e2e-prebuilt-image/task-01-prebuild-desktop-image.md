---
id: "01-prebuild-desktop-image"
title: "Prebuild desktop CI dependencies"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/e2e-duration-aware-sharding.md"
---

# Task 01: Prebuild Desktop CI Dependencies

## Acceptance

- A dedicated `desktop` image stage contains Rust `1.97.1`, the compiler and
  rpm bundle tools, and all Linux packages that the current desktop smoke job
  installs.
- The CI image workflow publishes content-addressed and `desktop-latest` tags
  through the existing GHCR path.
- A workflow contract test fails if the image stage, dependencies, smoke
  commands, publisher, or required test invocation disappears.

## Verification

```bash
python3 .github/scripts/e2e-tests-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
python3 .github/scripts/lint-action-pinning.py
docker build --target desktop --tag kandev-ci:desktop-local --file .github/docker/ci-base/Dockerfile .
```

## Files likely touched

- `.github/docker/ci-base/Dockerfile`
- `.github/workflows/ci-base-image.yml`
- `.github/scripts/e2e-tests-workflow-contract_test.py`
- `.github/workflows/lint-action-pinning.yml`

## Dependencies

None.

## Parallelism

sequential

## Inputs

- `docs/specs/platform/requirements/e2e-duration-aware-sharding.md`, especially the CI lifecycle
  and reliability contract.
- `docs/plans/desktop-e2e-prebuilt-image/plan.md`, CI image and Tests sections.
- The `runtime` and `build` patterns in the current CI base Dockerfile and
  publisher workflow.

## Output contract

Report the RED contract result, image contents, exact command results, changed
files, blockers, risks, and the synchronized task and plan status.

## Results

- RED: `python3 .github/scripts/e2e-tests-workflow-contract_test.py` failed with
  the expected missing desktop stage, publisher, consumer, and lint invocation.
- GREEN: the same contract test passed (`4 tests`).
- `python3 .github/scripts/lint-action-pinning_test.py` passed (`9 tests`).
- `python3 .github/scripts/lint-action-pinning.py` passed for all 18 workflows.
- `docker build --target desktop --tag kandev-ci:desktop-local --file
  .github/docker/ci-base/Dockerfile .` passed. The image smoke checks reported
  Rust 1.97.1, WebKitGTK, patchelf, and Xvfb.
- Changed files: the desktop Dockerfile, CI image publisher, E2E workflow,
  action-pinning workflow, contract test, and the associated plan/spec/task
  records.
- Blockers: none after the authorized branch image dispatch.
- Risks: cold pnpm/Cargo project dependency downloads still depend on external
  registries; the unbounded OS and Rust bootstrap path is prebuilt.
- Synchronized status: Task 01 is done and Wave 1 is complete in the plan.
