---
created: 2026-09-09
status: implemented
requirements:
  - REQ-EXECUTORS-K8S-PRESETS-001
  - REQ-EXECUTORS-K8S-PRESETS-002
system_design:
  - ../../specs/executors/system-design/kubernetes-worker-presets.md
legacy_specs:
  - ../../specs/kubernetes-executor/spec.md
---

# Implementation Plan: Kubernetes Worker Presets

## Overview

Deliver three tested starting configurations through existing images and executor profiles.
First add image recipes, profile examples, and local compatibility tests.
Then verify them through the real Kind lifecycle and publish the tested scope.

This package and [startup timing](../kubernetes-startup-timing/plan.md) are the
first delivery group. [Retained compute visibility](../kubernetes-retained-compute/plan.md)
follows. There is no hard code dependency between these packages.
Execution remains sequential in the primary session unless the user authorizes delegation.

## Scope

### In scope

- Minimal, Node/pnpm, and Python build targets with explicit pins.
- Copyable PodTemplate examples and companion profile settings.
- Local image smoke tests, real Kind lifecycle tests, and operator documentation.

### Out of scope

- New UI picker, profile defaults, registry publication, release-channel changes, or Helm packaging.
- Runtime redesign, warm pools, cache controllers, or guaranteed startup reductions.

## Technical approach

Use `k8s/worker-images/Dockerfile` and `pins.env` for the prepared targets.
Use `k8s/presets/` for templates and `scripts/test-kubernetes-worker-images.sh`
for bounded validation. Existing root images supply Node, Python, Git, and bootstrap tools.
The package reuses those installations rather than maintaining another OS/toolchain base.

`internal/agent/kubernetes/worker_presets_test.go` reads the actual examples through
`ParsePodTemplate` and `ComposePod`. The existing create/profile APIs stay unchanged.
Kind coverage adds `tests/kubernetes/kubernetes-worker-presets.spec.ts` and a fixture
helper for building/loading uniquely tagged images. The managed runner owns processes and teardown.

## Tests

| Acceptance | Evidence |
| --- | --- |
| PRESETS-001.1, PRESETS-001.2 | Script `--check`, image-version output, `TestKubernetesWorkerPresetsCompose` |
| PRESETS-001.3, PRESETS-002.4 | `--smoke`, non-root write/global-install checks, `TestKubernetesWorkerPresetsPreserveRuntimeOwnership` |
| PRESETS-001.4, PRESETS-002.1 | Real configuration test and terminal workload per target |
| PRESETS-002.2, PRESETS-002.3 | Kind retention/cleanup flow and recorded platform/image evidence |

IDs in this table use the `AC-EXECUTORS-K8S-` prefix.
New tests must fail before the corresponding implementation, per TDD.

## E2E tests

`tests/kubernetes/kubernetes-worker-presets.spec.ts`, project `containers`, verifies
configuration testing, task launch, terminal work, and Stop/Resume for each actual image target.
It uses existing Kind pins and the mock agent, with no paid provider credentials.
Existing desktop/mobile configuration specs verify that manual profile entry remains usable.
No new UI composition requires another mobile specification.

## Work orders

- [x] [Task 01: Add prepared worker recipes](task-01-worker-recipes.md)
- [x] [Task 02: Verify presets through Kind](task-02-kind-presets.md)

## Verification results

Implemented. The work orders record the exact source, image, UI, and Kind
verification commands. The real worker-preset flow passed for all three targets
on Linux/amd64 through the host Kind fixture.

## Risks

- The published base includes server tooling, so minimal means minimal additions rather than a small dedicated worker image.
- Base-image npm paths can conflict with runtime `HOME` and writable volumes.
- CSI `fsGroup` behavior varies. The tested Kind configuration does not certify every cluster.
- Image downloads can increase container-shard time. Build once per fixture and retain existing runner limits.
- Immutable base pins need deliberate refreshes. No moving digest is resolved during task launch.
