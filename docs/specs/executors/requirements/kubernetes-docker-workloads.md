---
status: draft
system: executors
created: 2026-09-13
owners:
  - kandev
---

# Kubernetes Docker Workloads Requirements

## Overview

Administrators need task Pods whose agents can build source, build container
images and run Docker/Compose tests against the task checkout. The executor
system owns the Pod-template, workspace and companion-container boundaries.
This capability extends the [Kubernetes foundation](../../kubernetes-executor/spec.md)
without changing existing profiles or baseline worker presets.

## Requirements

### REQ-EXECUTORS-K8S-DOCKER-001: Explicit companion workspace access

**Intent:** A selected companion can consume the same task checkout while
Kandev retains ownership of workspace storage and private runtime material.

#### Acceptance criteria

- **AC-EXECUTORS-K8S-DOCKER-001.1:** An administrator shall be able to explicitly
  grant an ordinary non-main container access to the task workspace at the same
  path as the main container, with either read-only or read-write access.
- **AC-EXECUTORS-K8S-DOCKER-001.2:** Profiles without such a grant shall retain
  their current mount behavior. The template shall not redefine workspace
  storage, redirect a grant, or grant companion access to Kandev runtime/auth
  volumes; init and ephemeral containers shall remain ineligible.
- **AC-EXECUTORS-K8S-DOCKER-001.3:** Admission that adds, removes, duplicates,
  transfers or changes a workspace grant shall fail validation before agent
  bootstrap. Unrelated compatible admission defaults shall remain accepted.
- **AC-EXECUTORS-K8S-DOCKER-001.4:** A replacement Pod shall use the recorded
  workload's grants and storage identity. Editing a profile shall not change a
  retained session's replacement workload or authorize deletion of foreign data.

### REQ-EXECUTORS-K8S-DOCKER-002: Explicit full-toolchain Docker configuration

**Intent:** Operators can prepare a full worker without mistaking installed
client commands for functioning container execution.

#### Acceptance criteria

- **AC-EXECUTORS-K8S-DOCKER-002.1:** A documented opt-in worker recipe shall
  identify immutable image inputs, Linux architecture, installed language and
  browser tools, runtime user, writable caches and Docker client/daemon versions.
  It shall contain no task credentials or private repository contents.
- **AC-EXECUTORS-K8S-DOCKER-002.2:** The example shall give each task Pod its own
  Docker daemon and local client connection, with no shared host Docker socket,
  public Docker endpoint or implicit cluster credential in the workload.
- **AC-EXECUTORS-K8S-DOCKER-002.3:** Daemon privileges, resources and storage
  shall be explicit operator configuration. The agent shall run non-root;
  configuration shall not silently enable privileged workloads in existing
  profiles or claim compatibility with restricted admission.
- **AC-EXECUTORS-K8S-DOCKER-002.4:** Docker-dependent preparation shall wait for
  daemon readiness with a finite timeout and report failure if it cannot become
  ready, rather than reporting a Docker-capable environment based on CLI presence.

### REQ-EXECUTORS-K8S-DOCKER-003: Verified execution and lifecycle

**Intent:** A Docker-capable task remains inspectable, resumable and reclaimable
through the existing Kandev lifecycle.

#### Acceptance criteria

- **AC-EXECUTORS-K8S-DOCKER-003.1:** Verification shall exercise actual source
  tests, image build/run and a Compose service that reads and writes the same
  task-workspace files seen by the agent. CLI version checks alone shall not
  establish compatibility.
- **AC-EXECUTORS-K8S-DOCKER-003.2:** Ordinary Stop/Resume shall preserve the Pod
  and managed workspace; documentation shall disclose retained companion
  compute. Replacement shall recover workspace results while permitting loss
  of explicitly disposable Docker caches and test containers.
- **AC-EXECUTORS-K8S-DOCKER-003.3:** Terminal cleanup shall remove exactly the
  owned Pod and managed workspace, leave existing claims untouched, and leave
  no nested compute belonging to the removed Pod. Agent results and diagnostics
  shall be checked through Kandev before destructive cleanup.
- **AC-EXECUTORS-K8S-DOCKER-003.4:** Published evidence shall distinguish tested
  architecture/runtime versions from untested combinations and verify resource
  accounting for the agent, daemon and nested test containers. One task with a
  companion shall remain one Kubernetes Pod for workload inventory purposes.

## Out of scope

- New control-plane deployments, operators, CRDs, Helm charts or release channels.
- A Docker security sandbox guarantee, automatic permission grants or a new UI.
- Cross-node migration of local PVCs, automatic suspend-to-zero or a capacity queue.
- Support for arbitrary host-path mounts, shared Docker daemons or all Docker plugins.
- Changing the security contract of the existing minimal, Node/pnpm or Python presets.

## Delivery

See the [system design](../system-design/kubernetes-docker-workloads.md) and
[implementation plan](../../../plans/kubernetes-docker-workloads/plan.md).
