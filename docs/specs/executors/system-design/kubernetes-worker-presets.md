---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-K8S-PRESETS-001
  - REQ-EXECUTORS-K8S-PRESETS-002
---

# Kubernetes Worker Presets System Design

## Purpose and boundaries

The executor system owns the image-to-bootstrap compatibility contract.
The [requirements](../requirements/kubernetes-worker-presets.md) add tested
examples to the existing raw PodTemplate flow. They require no executor API change.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| REQ-EXECUTORS-K8S-PRESETS-001 | Image recipes and profile examples |
| REQ-EXECUTORS-K8S-PRESETS-002 | Verification and maintenance |

## Image recipes and profile examples

New source lives under `k8s/worker-images/` and `k8s/presets/`.
One Dockerfile exposes `minimal`, `node-pnpm`, and `python` targets.
It derives from a published Kandev base image by immutable multi-platform digest.
The current root `Dockerfile` already supplies Node, npm, Python, venv, Git,
and the POSIX bootstrap tools. The Python target reuses those installations.
Only the Node/pnpm target adds pnpm, using the version in `apps/package.json`.

`k8s/worker-images/pins.env` records the chosen base digest and pnpm version.
Implementation resolves a published digest and records tool versions from built
images. Planning does not invent a digest or promise byte-identical rebuilds.
The recipe rejects absent pins and moving base tags. It ends with `USER 1000`.
No automatic publishing or root Dockerfile modification is required.

Each preset has one strict `v1/PodTemplate` example and a companion description
of profile fields. A template is an input to Kandev, not a directly runnable Pod.
Templates name `kandev-agent`, use `runAsNonRoot`, UID 1000, `fsGroup: 1000`,
disabled token automount, dropped capabilities, and disabled privilege escalation.
The templates do not set Kandev-owned command, mounts, `HOME`, ports, or restart policy.

Initial resource examples request 250m CPU and 512Mi memory with a 2Gi memory limit.
These values are starting settings, not measured workload requirements.
The default workspace is a 10Gi managed PVC with `ReadWriteOnce`.
The instructions explain the existing-claim and empty-directory alternatives.
No security claim depends only on `fsGroup` support across all storage drivers.

Examples keep npm globals and caches in writable session paths.
The existing image's `/data` defaults require explicit examination because
Kandev replaces `HOME` with `/run/kandev/home` and mounts the workspace at `/workspace`.
The Node example sets `NPM_CONFIG_PREFIX` and the corresponding `PATH` to a
writable directory under `/workspace`. These settings must survive the documented bootstrap.
Python uses a workspace venv rather than writes to the system interpreter.

Templates use an explicit image substitution marker until the operator builds
and publishes to their registry. The validation script accepts that marker only
in source examples. Resolved inputs must contain an immutable image reference.
Kind tests substitute a locally built image tag and `imagePullPolicy: Never`.
That fixture-only substitution is not the documented production policy.

## Runtime integration

`ParsePodTemplate`, `ComposePod`, admission validation, and the existing streaming
test remain the runtime gates. `KubernetesExecutor.bootstrapPod` continues to
upload the backend-selected helper and credentials through exec.
Image recipes do not replace the helper handshake or execute the server entrypoint.
Profile imports remain manual through the current create/edit flow on both viewports.

## Verification and maintenance

`scripts/test-kubernetes-worker-images.sh` is a proposed bounded build/smoke entry point.
`--check` verifies pins, targets, and source templates without a Docker daemon.
`--smoke --platform linux/amd64` builds the three targets and runs tool/version,
writable-path, local Git, npm-global, pnpm offline-project, and Python-venv tests.
It records image IDs and tool versions. All smoke data uses disposable volumes.

Backend `worker_presets_test.go` loads the actual example templates and passes
them through production parsing/composition. It asserts reserved-field compatibility.
No duplicate YAML parser or Kubernetes policy implementation is introduced.

The new `tests/kubernetes/kubernetes-worker-presets.spec.ts` uses the existing
Kind fixture, restricted executor identity, backend, and mock agent.
It drives Test Kubernetes and a real task/terminal for each preset.
It verifies a local Git operation and a small language workload through the terminal.
One parameterized stop/resume/cleanup scenario proves storage behavior for every target.
Tests load the actual built targets, not the fixture's generic Ubuntu image.

Initial required lifecycle evidence is Linux/amd64 on the existing Kind version.
An arm64 build/smoke result does not establish an arm64 Kubernetes lifecycle result.
Public guidance lists untested combinations explicitly. Existing profile support
for arm64 remains available independently of these examples.

The container E2E workflow includes the new spec through its existing Kubernetes
shard detection. Image build cost is paid once per fixture worker with unique tags.
Cleanup tracks only images, containers, volumes, and cluster names created by that fixture.
There is no daemon-wide prune or shared-cache mutation.

## Failure, security, and persistence

A recipe failure produces no profile or cluster mutation.
Failed test launches retain existing identity-checked cleanup behavior.
Samples contain no credentials, Git remotes, host mounts, or Docker sockets.
The operator remains responsible for registry access, storage ownership, and cluster admission.
No database changes or new runtime configuration keys are required.

## Related decisions

- [Kubernetes resource ownership](../../../decisions/2026-08-24-kubernetes-executor-resource-ownership.md)
- [Current Kubernetes foundation](../../kubernetes-executor/spec.md)

## Delivery

See the [worker presets plan](../../../plans/kubernetes-worker-presets/plan.md).
