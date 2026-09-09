---
id: "01-worker-recipes"
title: "Add prepared worker recipes"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-K8S-PRESETS-001
  - REQ-EXECUTORS-K8S-PRESETS-002
acceptance_criteria:
  - AC-EXECUTORS-K8S-PRESETS-001.1
  - AC-EXECUTORS-K8S-PRESETS-001.2
  - AC-EXECUTORS-K8S-PRESETS-001.3
  - AC-EXECUTORS-K8S-PRESETS-002.4
system_design:
  - ../../specs/executors/system-design/kubernetes-worker-presets.md
---

# Task 01: Add Prepared Worker Recipes

## Summary

Provide reusable image targets and copyable workload inputs for three common profiles.
The existing executor remains the owner of bootstrap, auth, and workspace mounts.

## In scope

- Resolve and record an immutable published multi-platform Kandev base digest.
- Add minimal, Node/pnpm, and Python targets with writable runtime/global-install paths.
- Add source templates, profile-field examples, and the bounded build/smoke script described in the design.
- Add production-parser tests that load those actual files.
- Document build, local smoke, registry substitution, tool inventories, and storage requirements in `k8s/worker-images/README.md`.

## Out of scope

- Kind integration, image publication, new UI controls, or changes to release workflows.
- Real provider credentials, repository content in images, or host mounts.

## Acceptance

- Source inputs pass production composition and preserve all reserved runtime fields.
- Each image target passes non-root file, Git, and documented toolchain smoke checks with explicit pins.
- Smoke cleanup affects only recorded disposable test objects and reports exact image/tool versions.

## Verification

From the repository root:

```bash
rtk bash scripts/test-kubernetes-worker-images.sh --check
rtk bash scripts/test-kubernetes-worker-images.sh --smoke --platform linux/amd64
rtk git diff --check
```

From `apps/backend`:

```bash
rtk go test ./internal/agent/kubernetes -run 'TestKubernetesWorkerPresets' -count=1
```

The script and named tests are new outputs of this work order.
Write failure assertions first. A missing Docker daemon blocks smoke evidence,
not source validation. Do not mark this work order done without the required smoke result.

## Files likely touched

- `k8s/worker-images/Dockerfile`, `pins.env`, and `README.md` (new)
- `k8s/presets/minimal.yaml`, `node-pnpm.yaml`, and `python.yaml` (new)
- `scripts/test-kubernetes-worker-images.sh` (new)
- `apps/backend/internal/agent/kubernetes/worker_presets_test.go` (new)

## Dependencies

None. Required environment: Docker with Linux/amd64 image execution and registry access.

## Risks

- Image-level `/data` defaults and runtime `HOME` differ.
- Build-time downloads must use pins. Tests must not rely on a developer's npm or Python caches.
- No arm64 lifecycle claim follows from an amd64 result.

## Parallelism

`sequential`

## Inputs

- [Preset requirements](../../specs/executors/requirements/kubernetes-worker-presets.md)
- [Preset design](../../specs/executors/system-design/kubernetes-worker-presets.md)
- `Dockerfile`, `Dockerfile.universal`, `docs/images.md`
- `apps/backend/internal/agent/kubernetes/compose_test.go`
- `docs/decisions/2026-08-24-kubernetes-executor-resource-ownership.md`

## Results

Passed:

- `rtk bash scripts/test-kubernetes-worker-images.sh --check`
- `rtk bash scripts/test-kubernetes-worker-images.sh --smoke --platform linux/amd64`
  (minimal `sha256:bf9586f8c189a3186886f518b0d12665a5e65a71c85a26b5bb67fd4788565379`,
  node-pnpm `sha256:d3a36164c42c9dd90f2d46bc6d37fad59d8f28490566e9815a76cc2692227a92`,
  Python `sha256:bf9586f8c189a3186886f518b0d12665a5e65a71c85a26b5bb67fd4788565379`
  reuses the immutable minimal layer and passed the Python cache/venv smoke)
- `rtk go test ./internal/agent/kubernetes -run 'TestKubernetesWorkerPresets' -count=1`
  (8 tests)
- `rtk git diff --check`
