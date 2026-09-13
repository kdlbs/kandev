---
id: "02-full-worker-image"
title: "Full worker image and daemon recipe"
status: blocked
wave: 2
depends_on:
  - "01-workspace-grants"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-K8S-DOCKER-002
  - REQ-EXECUTORS-K8S-DOCKER-003
acceptance_criteria:
  - AC-EXECUTORS-K8S-DOCKER-002.1
  - AC-EXECUTORS-K8S-DOCKER-002.2
  - AC-EXECUTORS-K8S-DOCKER-002.3
  - AC-EXECUTORS-K8S-DOCKER-002.4
  - AC-EXECUTORS-K8S-DOCKER-003.1
system_design:
  - ../../specs/executors/system-design/kubernetes-docker-workloads.md
---

# Task 02: Full worker image and daemon recipe

## Summary

Produce a pinned full worker and an opt-in per-Pod Docker template, retaining
the current bootstrap contract. Provide a reproducible build and smoke entry
point that task 03 can execute against real Kubernetes.

## In scope

- New `k8s/worker-images/full/` recipe, pins, README, build and smoke scripts.
  Preserve the existing minimal/node-pnpm/python recipes.
- Confirm a bounded build host before building. Inspect actual headroom and
  builder resource controls; do not start unbounded builds or introduce new
  host daemons, registry services or publication by assumption.
- Resolve immutable amd64 universal/DinD image references and added tool inputs.
  Match repository Go, pnpm, golangci-lint v2 and lockfile Playwright browser/OS
  dependencies. Include Rust, Python/venv, compilers, Docker CLI/Buildx/Compose.
- Non-root writable tool/cache paths; no credential layers. Baked browser/tools
  live outside `/workspace` and the runtime HOME. Create caches after clone.
- Profile example with main plus privileged `docker-engine`, the explicit
  workspace grant, Pod-local Unix socket, Docker data `emptyDir`, explicit
  resources and daemon MTU. Bypass inherited TCP-listener entrypoint defaults.
- Finite daemon readiness before repository setup, preserving default clone,
  origin validation, selected agent installation and task-branch postlude.

## Out of scope

Applying the template, installing cluster policy, production service updates,
real credentials, image publication or claiming full Docker-executor parity.

## Acceptance

1. A reproducible image builds from recorded immutable inputs and passes small
   Go, Node/pnpm, Python and headless Chromium tests as UID/GID 1000.
2. The documented template passes Kandev validation after task 01 and includes
   explicit daemon privilege, limits, workspace/socket/data mounts and startup
   handling without implicitly changing any existing profile.
3. The smoke includes actual Docker build/run/Compose and filesystem assertions,
   finite timeouts and targeted teardown. Task 03 must run those tests; tool
   version output alone does not complete container-execution acceptance.

## Verification

The following scripts are outputs of this work order. Their interfaces must
fail if inputs/pins are missing. Run from the repository root on the selected
bounded build host; `--check` validates inputs without building.

```bash
bash -n k8s/worker-images/full/build.sh k8s/worker-images/full/smoke.sh
bash k8s/worker-images/full/build.sh --check
bash k8s/worker-images/full/build.sh --build --verify
(cd apps/backend && GOMAXPROCS=2 go test -p 1 ./internal/agent/kubernetes -count=1)
git diff --check -- k8s docs/public/k8s.md docs/plans/kubernetes-docker-workloads
```

`--verify` must exercise actual source/browser tools in a constrained non-root
container and record image ID/digest, versions, platform and build limits.
It does not silently launch privileged Docker on a production cluster. Task 03 loads
the exact produced image and exercises `smoke.sh --all` inside the task Pod.

## Files likely touched

- `k8s/worker-images/full/{Dockerfile,pins.env,README.md,build.sh,smoke.sh}` (new)
- `k8s/worker-images/full/{pod-template.yaml,prepare.sh}` (new)
- `docs/public/k8s.md` (opt-in recipe link and explicit runtime constraints)
- A focused recipe contract test alongside existing Kubernetes preset tests.

## Dependencies

Task 01. No worker import or production Pod can use the new grant until the
operator deliberately upgrades the backend to the verified implementation.

## Risks

Image size, browser native dependencies, Rust paths under an overridden HOME,
non-root socket permissions, cache creation before clone and daemon entrypoint
defaults can each break an otherwise valid template. A bounded build may need
a different host; resolve that before consuming significant resources.

## Parallelism

`sequential`

## Inputs

- The system design's Worker and daemon configuration section.
- [System design](../../specs/executors/system-design/kubernetes-docker-workloads.md), Worker and daemon configuration.
- `Dockerfile.universal`, `mise.toml`, `apps/pnpm-lock.yaml`,
  `k8s/worker-images/README.md`, `k8s/presets/node-pnpm.yaml`.

## Results

Implementation authored; runtime acceptance remains blocked. Do not treat this
work order as accepted or promote the pull request to ready.

The immutable recipe, explicit daemon template, finite preparation, renderer and
source/browser/Docker smoke scripts are present. `TestFullWorkerTemplate` and
`TestFullWorkerPreparationContract` passed. The filesystem preparation test
`TestFullWorkerPreparationClonesBeforeCachesAndRetainsWorkspace` passed, proving
clone/cache ordering and retained checkout reuse. Shell syntax, input validation
and immutable image rendering passed. The renderer regression first failed
because the renderer was absent; its Go rerun remains pending.

A bounded `build.sh --build --verify` attempt did not complete. No image ID,
source/browser image verification or supported full runtime matrix is claimed.
The disposable build resources were removed. Complete the image build and
verification on a suitable isolated test host before running task 03.
