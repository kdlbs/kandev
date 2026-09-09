---
id: "02-kind-presets"
title: "Verify presets through Kind"
status: done
wave: 2
depends_on:
  - "01-worker-recipes"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-K8S-PRESETS-001
  - REQ-EXECUTORS-K8S-PRESETS-002
acceptance_criteria:
  - AC-EXECUTORS-K8S-PRESETS-001.4
  - AC-EXECUTORS-K8S-PRESETS-002.1
  - AC-EXECUTORS-K8S-PRESETS-002.2
  - AC-EXECUTORS-K8S-PRESETS-002.3
system_design:
  - ../../specs/executors/system-design/kubernetes-worker-presets.md
---

# Task 02: Verify Presets Through Kind

## Summary

Exercise each recipe as a real Kubernetes task environment with the current executor.
Publish instructions that name the combinations this test actually verifies.

## In scope

- Build/load the Task 01 targets through an isolated image helper in the Kind fixture.
- Drive Test Kubernetes, launch, terminal commands, a local Git operation, and per-language smoke work through the existing UI.
- Verify Stop/Resume marker retention and exact owned Pod/PVC cleanup for every target.
- Integrate the spec with existing Kubernetes container shards and attach tool/image evidence.
- Update `docs/public/k8s.md` with a focused how-to section and links to source recipes.

## Out of scope

- A new cluster harness, new release channels, provider-paid E2E, or modifications to developer clusters.
- Expanding claimed architecture support beyond actual test evidence.

## Acceptance

- All three actual image targets complete the UI/terminal workload under the restricted executor identity.
- Stop/Resume preserves each marker and terminal cleanup removes only recorded owned resources.
- Public instructions and CI evidence agree on image inputs, architecture, and tested Kubernetes version.

## Verification

From `apps`, once before the first package command in a fresh worktree:

```bash
rtk pnpm install --frozen-lockfile
```

From `apps/web`, sequentially with the managed runner:

```bash
rtk env KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --project containers tests/kubernetes/kubernetes-worker-presets.spec.ts
rtk pnpm e2e:run --project chromium tests/settings/kubernetes-executor.spec.ts
rtk pnpm e2e:run --project mobile-chrome tests/settings/mobile-kubernetes-executor.spec.ts
```

From the repository root:

```bash
rtk node --test scripts/validate-public-docs.test.mjs
rtk node scripts/validate-public-docs.mjs
rtk git diff --check
```

The new Kind spec must fail against absent/incompatible preset wiring before implementation.
The runner builds current backend, helpers, and web assets. Verify test discovery
and use the existing worker budget. No overlapping container runs are allowed.

## Files likely touched

- `apps/web/e2e/tests/kubernetes/kubernetes-worker-presets.spec.ts` (new)
- `apps/web/e2e/fixtures/kubernetes-worker-images.ts` (new)
- `apps/web/e2e/fixtures/kubernetes-test-base.ts`
- `apps/web/e2e/fixtures/kubernetes-tools.ts`
- `.github/workflows/e2e-tests.yml` only if existing Kubernetes shard wiring needs the new fixture inputs
- `docs/public/k8s.md`, `k8s/worker-images/README.md`

## Dependencies

Task `01-worker-recipes`. Host Docker, the existing pinned Kind/kubectl tools,
registry access, and sufficient disk for the three targets are required.

## Risks

- A generic fixture image can hide broken recipe permissions. The test must load each actual target.
- Global npm installation needs a network-free fixture package, not a paid agent installation.
- The published page must not imply that direct `kubectl apply` of a PodTemplate starts a Kandev session.

## Parallelism

`sequential`

## Inputs

- [Preset requirements](../../specs/executors/requirements/kubernetes-worker-presets.md)
- [Preset design](../../specs/executors/system-design/kubernetes-worker-presets.md)
- `apps/web/e2e/tests/kubernetes/kubernetes-executor.spec.ts`
- `apps/web/e2e/tests/settings/mobile-kubernetes-executor.spec.ts`
- `apps/web/e2e/README.md`

## Results

Passed:

- `rtk env KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --no-build --project containers tests/kubernetes/kubernetes-worker-presets.spec.ts`
  (1 test passed in 3.7 minutes; all three actual recipe targets launched,
  completed terminal work, preserved Stop/Resume state, and cleaned exact
  owned Pod/PVC resources)
- `rtk pnpm e2e:run --host --no-build --project chromium tests/settings/kubernetes-executor.spec.ts`
  (4 tests passed)
- `rtk pnpm e2e:run --host --no-build --project mobile-chrome tests/settings/mobile-kubernetes-executor.spec.ts`
  (3 tests passed)
- `rtk node --test scripts/validate-public-docs.test.mjs`
- `rtk node scripts/validate-public-docs.mjs`
- `rtk git diff --check`
